package grpc

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"erp-auth-service/internal/application/services"
	"erp-auth-service/internal/cache"
	"erp-auth-service/internal/config"
	"erp-auth-service/internal/domain/entities"
	"erp-auth-service/internal/elasticsearch"
	internalErrors "erp-auth-service/internal/errors"
	"erp-auth-service/internal/events"
	"erp-auth-service/internal/grpc/interceptors"
	"erp-auth-service/internal/infrastructure/repositories"
	"erp-auth-service/internal/middleware"
	"erp-auth-service/internal/models"
	pb "erp-auth-service/proto"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/redis/go-redis/v9"
	"github.com/sony/gobreaker"
	"go.uber.org/zap"
	"golang.org/x/crypto/bcrypt"
	"google.golang.org/grpc"
	"google.golang.org/grpc/keepalive"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
	"gorm.io/gorm"
)

// EnhancedAuthGRPCServer represents the enhanced gRPC server with interceptor chain
type EnhancedAuthGRPCServer struct {
	pb.UnimplementedAuthServiceServer

	// Dependencies
	db          *gorm.DB
	redisClient *redis.Client
	config      *config.Config
	logger      *zap.Logger

	// Business services
	authService       *services.AuthService
	tokenService      *services.TokenService
	permissionService *services.PermissionService
	activityService   ActivityServiceInterface
	cacheManager      cache.CacheManager

	// Server components
	grpcServer *grpc.Server
	listener   net.Listener

	// Interceptors and middleware
	rateLimiter           *interceptors.RateLimiter
	circuitBreakerManager *interceptors.CircuitBreakerManager
	metricsInterceptor    *interceptors.MetricsInterceptor
	loggingInterceptor    *interceptors.LoggingInterceptor
	authInterceptor       *interceptors.AuthInterceptor
	recoveryInterceptor   *interceptors.RecoveryInterceptor

	// Error handling
	errorHandler *internalErrors.ErrorHandler

	// Metrics registry
	metricsRegistry *prometheus.Registry

	// Shutdown coordination
	shutdownCh chan struct{}
	wg         sync.WaitGroup
}

// ActivityServiceInterface abstracts activity logging/retrieval backend (PostgreSQL or Elasticsearch)
type ActivityServiceInterface interface {
	LogActivity(ctx context.Context, userID, organizationID uuid.UUID, action, resource string, details map[string]interface{}, ipAddress, userAgent string) error
	GetUserActivities(ctx context.Context, userID *uuid.UUID, organizationID *uuid.UUID, limit, offset int) ([]*models.UserActivity, int64, error)
	GetSecurityStats(ctx context.Context, organizationID uuid.UUID) (map[string]int64, error)
	LogLogin(ctx context.Context, userID, organizationID uuid.UUID, ipAddress, userAgent string) error
	LogLoginFailed(ctx context.Context, email, ipAddress, userAgent string, organizationID uuid.UUID) error
	LogLogout(ctx context.Context, userID, organizationID uuid.UUID, ipAddress, userAgent string) error
}

// NewAuthGRPCServer creates a new gRPC server with properly initialized dependencies
func NewAuthGRPCServer(db *gorm.DB, redisClient *redis.Client, config *config.Config) *EnhancedAuthGRPCServer {
	// Initialize logger
	logger, err := zap.NewProduction()
	if err != nil {
		// Fallback to development logger if production fails
		logger, _ = zap.NewDevelopment()
	}

	// Initialize repository factory
	repoFactory := repositories.NewRepositoryFactory(db, db) // Using same DB for read/write for now

	// Initialize cache manager with proper configuration
	cacheConfig := cache.CacheConfig{
		BigCacheConfig: cache.BigCacheConfig{
			Shards:             1024,
			LifeWindow:         10 * time.Minute,
			CleanWindow:        5 * time.Minute,
			MaxEntriesInWindow: 1000 * 10 * 60,
			MaxEntrySize:       500,
			HardMaxCacheSize:   8192,
			Verbose:            false,
		},
		RedisConfig: cache.RedisClusterConfig{
			Addrs:              []string{fmt.Sprintf("%s:%s", config.Redis.Host, config.Redis.Port)},
			Password:           config.Redis.Password,
			DB:                 config.Redis.DB,
			PoolSize:           10,
			MinIdleConns:       5,
			MaxConnAge:         time.Hour,
			PoolTimeout:        30 * time.Second,
			IdleTimeout:        5 * time.Minute,
			IdleCheckFrequency: time.Minute,
			ReadTimeout:        3 * time.Second,
			WriteTimeout:       3 * time.Second,
			DialTimeout:        5 * time.Second,
		},
		DefaultTTL:        5 * time.Minute,
		MaxRetries:        3,
		RetryDelay:        100 * time.Millisecond,
		EnableMetrics:     true,
		MetricsInterval:   30 * time.Second,
		SyncInterval:      time.Minute,
		WarmupEnabled:     true,
		WarmupBatchSize:   100,
		InvalidationDelay: time.Second,
	}

	var cacheManager cache.CacheManager
	cacheManagerImpl, err := cache.NewCacheManager(cacheConfig, logger)
	if err != nil {
		logger.Error("Failed to initialize cache manager", zap.Error(err))
		// Create a simple fallback cache manager
		cacheManager = &SimpleCacheManager{redisClient: redisClient}
	} else {
		cacheManager = cacheManagerImpl
	}

	// Initialize event publisher
	eventPublisher := &KafkaEventPublisher{
		producer: events.NewProducer(events.ProducerConfig{
			Brokers: config.Kafka.Brokers,
			Topic:   config.Kafka.Topic,
		}),
		logger: logger,
	}

	// Initialize services with proper dependencies
	tokenService := services.NewTokenService(
		config,
		logger,
		repoFactory.TokenRepository(),
		cacheManager,
		redisClient,
	)

	permissionService := services.NewPermissionService(
		repoFactory.PermissionRepository(),
		repoFactory.RoleRepository(),
		repoFactory.UserRepository(),
		cacheManager,
		logger,
		services.PermissionServiceConfig{
			CacheTTL:              5 * time.Minute,
			BulkEvaluationEnabled: true,
			CacheWarmingEnabled:   true,
			MaxCacheSize:          10000,
			EvaluationTimeout:     30 * time.Second,
		},
	)

	// Initialize activity service (Elasticsearch preferred if enabled)
	var activityService ActivityServiceInterface
	if config.Elasticsearch.Enabled {
		esClient, err := elasticsearch.NewClient(elasticsearch.Config{
			Addresses: config.Elasticsearch.Addresses,
			Username:  config.Elasticsearch.Username,
			Password:  config.Elasticsearch.Password,
			APIKey:    config.Elasticsearch.APIKey,
		}, logger)
		if err != nil {
			logger.Warn("Falling back to PostgreSQL activity service due to Elasticsearch init failure", zap.Error(err))
			activityService = services.NewActivityService(
				repositories.NewUserActivityRepository(db),
				logger,
			)
		} else {
			// Attach Redis client for real-time WebSocket publishing
			activityService = services.NewElasticsearchActivityService(esClient, logger).WithRedis(redisClient)
		}
	} else {
		activityService = services.NewActivityService(
			repositories.NewUserActivityRepository(db),
			logger,
		)
	}

	authService := services.NewAuthService(
		config,
		logger,
		repoFactory.UserRepository(),
		repoFactory.OrganizationRepository(),
		repoFactory.RoleRepository(),
		tokenService,
		eventPublisher,
		cacheManager,
	)

	return NewEnhancedAuthGRPCServer(db, redisClient, config, logger, authService, tokenService, permissionService, activityService, cacheManager)
}

// NewEnhancedAuthGRPCServer creates a new enhanced gRPC server
func NewEnhancedAuthGRPCServer(
	db *gorm.DB,
	redisClient *redis.Client,
	config *config.Config,
	logger *zap.Logger,
	authService *services.AuthService,
	tokenService *services.TokenService,
	permissionService *services.PermissionService,
	activityService ActivityServiceInterface,
	cacheManager cache.CacheManager,
) *EnhancedAuthGRPCServer {
	// Create metrics registry
	metricsRegistry := prometheus.NewRegistry()

	// Initialize interceptors
	rateLimiterConfig := interceptors.RateLimiterConfig{
		RequestsPerSecond: 1000, // 1000 RPS per client
		BurstSize:         100,
		WindowSize:        time.Minute,
		RedisKeyPrefix:    "ratelimit",
	}

	server := &EnhancedAuthGRPCServer{
		db:              db,
		redisClient:     redisClient,
		config:          config,
		logger:          logger,
		metricsRegistry: metricsRegistry,
		shutdownCh:      make(chan struct{}),

		// Business services
		authService:       authService,
		tokenService:      tokenService,
		permissionService: permissionService,
		activityService:   activityService,
		cacheManager:      cacheManager,

		// Initialize interceptors
		rateLimiter:           interceptors.NewRateLimiter(redisClient, rateLimiterConfig, logger),
		circuitBreakerManager: interceptors.NewCircuitBreakerManager(logger),
		metricsInterceptor:    interceptors.NewMetricsInterceptor(metricsRegistry),
		loggingInterceptor:    interceptors.NewLoggingInterceptor(logger),
		authInterceptor:       interceptors.NewAuthInterceptor(config.JWT.Secret, "internal-service-key", logger),
		recoveryInterceptor:   interceptors.NewRecoveryInterceptor(logger),
	}

	// Initialize comprehensive error handler
	server.errorHandler = internalErrors.NewErrorHandler(logger, internalErrors.GetDefaultErrorHandlerConfig())

	// Register circuit breakers for dependencies
	server.setupCircuitBreakers()

	return server
}

// setupCircuitBreakers configures circuit breakers for external dependencies
func (s *EnhancedAuthGRPCServer) setupCircuitBreakers() {
	// Database circuit breaker
	s.circuitBreakerManager.RegisterBreaker("database", interceptors.GetDefaultDatabaseConfig())

	// Redis circuit breaker
	s.circuitBreakerManager.RegisterBreaker("redis", interceptors.GetDefaultRedisConfig())

	// Kafka circuit breaker (if needed)
	s.circuitBreakerManager.RegisterBreaker("kafka", interceptors.GetDefaultKafkaConfig())

	// gRPC handler circuit breaker
	s.circuitBreakerManager.RegisterBreaker("grpc-handler", interceptors.CircuitBreakerConfig{
		MaxRequests: 10,
		Interval:    time.Second * 30,
		Timeout:     time.Second * 60,
		ReadyToTrip: func(counts gobreaker.Counts) bool {
			failureRatio := float64(counts.TotalFailures) / float64(counts.Requests)
			return counts.Requests >= 10 && failureRatio >= 0.3
		},
	})
}

// Start starts the enhanced gRPC server with graceful shutdown
func (s *EnhancedAuthGRPCServer) Start() error {
	// Create listener
	lis, err := net.Listen("tcp", ":"+s.config.GRPC.Port)
	if err != nil {
		return fmt.Errorf("failed to listen on port %s: %w", s.config.GRPC.Port, err)
	}
	s.listener = lis

	// Create gRPC server with enhanced configuration
	s.grpcServer = grpc.NewServer(
		// Connection settings
		grpc.KeepaliveParams(keepalive.ServerParameters{
			MaxConnectionIdle:     time.Duration(s.config.GRPC.MaxConnectionIdle) * time.Second,
			MaxConnectionAge:      time.Duration(s.config.GRPC.MaxConnectionAge) * time.Second,
			MaxConnectionAgeGrace: time.Duration(s.config.GRPC.MaxConnectionAgeGrace) * time.Second,
			Time:                  time.Duration(s.config.GRPC.Time) * time.Second,
			Timeout:               time.Duration(s.config.GRPC.Timeout) * time.Second,
		}),
		grpc.KeepaliveEnforcementPolicy(keepalive.EnforcementPolicy{
			MinTime:             time.Duration(s.config.GRPC.KeepaliveEnforcementMinTime) * time.Second,
			PermitWithoutStream: s.config.GRPC.KeepaliveEnforcementPermitWithoutStream,
		}),

		// Message size limits
		grpc.MaxRecvMsgSize(s.config.GRPC.MaxRecvMsgSize),
		grpc.MaxSendMsgSize(s.config.GRPC.MaxSendMsgSize),
		grpc.MaxConcurrentStreams(uint32(s.config.GRPC.MaxConcurrentStreams)),

		// Interceptor chain - order matters!
		grpc.ChainUnaryInterceptor(
			s.recoveryInterceptor.UnaryServerInterceptor(),   // First: catch panics
			s.loggingInterceptor.UnaryServerInterceptor(),    // Second: log requests
			s.metricsInterceptor.UnaryServerInterceptor(),    // Third: collect metrics
			s.rateLimiter.UnaryServerInterceptor(),           // Fourth: rate limiting
			s.authInterceptor.UnaryServerInterceptor(),       // Fifth: authentication
			s.circuitBreakerManager.UnaryServerInterceptor(), // Sixth: circuit breaker
		),
		grpc.ChainStreamInterceptor(
			s.recoveryInterceptor.StreamServerInterceptor(),
			s.loggingInterceptor.StreamServerInterceptor(),
			s.metricsInterceptor.StreamServerInterceptor(),
			s.rateLimiter.StreamServerInterceptor(),
			s.authInterceptor.StreamServerInterceptor(),
			s.circuitBreakerManager.StreamServerInterceptor(),
		),
	)

	// Register service
	pb.RegisterAuthServiceServer(s.grpcServer, s)

	// Start server in goroutine
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		s.logger.Info("Enhanced gRPC server starting",
			zap.String("port", s.config.GRPC.Port),
			zap.Int("max_concurrent_streams", s.config.GRPC.MaxConcurrentStreams),
			zap.Int("max_recv_msg_size", s.config.GRPC.MaxRecvMsgSize),
			zap.Int("max_send_msg_size", s.config.GRPC.MaxSendMsgSize))

		if err := s.grpcServer.Serve(lis); err != nil {
			s.logger.Error("gRPC server error", zap.Error(err))
		}
	}()

	// Setup graceful shutdown
	s.setupGracefulShutdown()

	return nil
}

// setupGracefulShutdown configures graceful shutdown handling
func (s *EnhancedAuthGRPCServer) setupGracefulShutdown() {
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		sig := <-sigCh
		s.logger.Info("Received shutdown signal", zap.String("signal", sig.String()))
		s.Shutdown()
	}()
}

// Shutdown gracefully shuts down the server
func (s *EnhancedAuthGRPCServer) Shutdown() {
	s.logger.Info("Starting graceful shutdown...")

	// Close shutdown channel to signal shutdown
	close(s.shutdownCh)

	// Create shutdown context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Graceful stop with timeout
	done := make(chan struct{})
	go func() {
		s.grpcServer.GracefulStop()
		close(done)
	}()

	select {
	case <-done:
		s.logger.Info("gRPC server stopped gracefully")
	case <-ctx.Done():
		s.logger.Warn("Graceful shutdown timeout, forcing stop")
		s.grpcServer.Stop()
	}

	// Wait for all goroutines to finish
	s.wg.Wait()
	s.logger.Info("Enhanced gRPC server shutdown complete")
}

// Wait blocks until the server shuts down
func (s *EnhancedAuthGRPCServer) Wait() {
	<-s.shutdownCh
	s.wg.Wait()
}

// GetMetricsRegistry returns the Prometheus metrics registry
func (s *EnhancedAuthGRPCServer) GetMetricsRegistry() *prometheus.Registry {
	return s.metricsRegistry
}

// Authenticate handles user authentication with email and password
func (s *EnhancedAuthGRPCServer) Authenticate(ctx context.Context, req *pb.AuthenticateRequest) (*pb.AuthenticateResponse, error) {
	// Validate input
	if req.Email == "" || req.Password == "" {
		return &pb.AuthenticateResponse{
			Success: false,
			Error:   "email and password are required",
		}, nil
	}

	// Convert gRPC request to auth service request
	authReq := &services.AuthRequest{
		Email:    req.Email,
		Password: req.Password,
		SecurityContext: &services.SecurityContext{
			IPAddress: req.SecurityContext.GetIpAddress(),
			UserAgent: req.SecurityContext.GetUserAgent(),
			SessionID: req.SecurityContext.GetSessionId(),
		},
		RememberMe: req.RememberMe,
	}

	// Call auth service
	authResp, err := s.authService.Authenticate(ctx, authReq)
	if err != nil {
		s.logger.Error("Authentication failed", zap.Error(err))

		// Log failed login attempt
		if s.activityService != nil {
			// For failed authentication, we don't have organization ID, so we'll use a default or skip
			s.activityService.LogLoginFailed(ctx, req.Email,
				req.SecurityContext.GetIpAddress(),
				req.SecurityContext.GetUserAgent(),
				uuid.Nil) // We'll need to handle this better
		}

		return &pb.AuthenticateResponse{
			Success: false,
			Error:   "authentication failed",
		}, nil
	}

	// Convert response
	response := &pb.AuthenticateResponse{
		Success: authResp.Success,
		Error:   authResp.Error,
	}

	if authResp.Success && authResp.User != nil {
		// Log successful login
		if s.activityService != nil {
			s.activityService.LogLogin(ctx,
				authResp.User.ID,
				authResp.User.OrganizationID,
				req.SecurityContext.GetIpAddress(),
				req.SecurityContext.GetUserAgent())
		}
		// Convert user
		response.User = &pb.User{
			Id:             authResp.User.ID.String(),
			OrganizationId: authResp.User.OrganizationID.String(),
			Email:          authResp.User.Email,
			FirstName:      authResp.User.FirstName,
			LastName:       authResp.User.LastName,
			IsActive:       authResp.User.IsActive,
			IsVerified:     authResp.User.IsVerified,
			CreatedAt:      timestamppb.New(authResp.User.CreatedAt),
			UpdatedAt:      timestamppb.New(authResp.User.UpdatedAt),
		}

		if authResp.User.LastLoginAt != nil {
			response.User.LastLoginAt = timestamppb.New(*authResp.User.LastLoginAt)
		}

		// Convert tokens if available
		if authResp.TokenPair != nil {
			response.Tokens = &pb.TokenPair{
				AccessToken:      authResp.TokenPair.AccessToken,
				RefreshToken:     authResp.TokenPair.RefreshToken,
				TokenType:        authResp.TokenPair.TokenType,
				ExpiresAt:        timestamppb.New(authResp.TokenPair.ExpiresAt),
				RefreshExpiresAt: timestamppb.New(authResp.TokenPair.RefreshExpiresAt),
			}
		}
	}

	return response, nil
}

// ValidateToken validates a JWT token with sub-10ms response time optimization
func (s *EnhancedAuthGRPCServer) ValidateToken(ctx context.Context, req *pb.ValidateTokenRequest) (*pb.ValidateTokenResponse, error) {
	// Start performance timer
	start := time.Now()
	defer func() {
		duration := time.Since(start)
		if s.metricsInterceptor != nil {
			s.metricsInterceptor.RecordTokenValidationDuration(duration)
		}
		if duration > 10*time.Millisecond {
			s.logger.Warn("Token validation exceeded 10ms target",
				zap.Duration("duration", duration),
				zap.String("token_prefix", req.Token[:min(len(req.Token), 20)]))
		}
	}()

	if req.Token == "" {
		return &pb.ValidateTokenResponse{
			Valid: false,
			Error: "Token is required",
		}, nil
	}

	// Use token service for optimized validation with caching
	tokenService := s.getTokenService()
	claims, err := tokenService.ValidateToken(ctx, req.Token)
	if err != nil {
		s.logger.Debug("Token validation failed", zap.Error(err))
		return &pb.ValidateTokenResponse{
			Valid: false,
			Error: "Invalid or expired token",
		}, nil
	}

	return &pb.ValidateTokenResponse{
		Valid:          true,
		UserId:         claims.UserID.String(),
		OrganizationId: claims.OrganizationID.String(),
		Email:          claims.Email,
		ExpiresAt:      timestamppb.New(claims.ExpiresAt.Time),
	}, nil
}

// RefreshToken creates new tokens with atomic operations
func (s *EnhancedAuthGRPCServer) RefreshToken(ctx context.Context, req *pb.RefreshTokenRequest) (*pb.RefreshTokenResponse, error) {
	if req.RefreshToken == "" {
		return &pb.RefreshTokenResponse{
			Error: "Refresh token is required",
		}, nil
	}

	// Use token service for atomic refresh operation
	tokenService := s.getTokenService()

	// Create security context from request metadata if available
	securityCtx := &services.SecurityContext{
		IPAddress: "unknown", // Extract from gRPC metadata if needed
		UserAgent: "grpc-client",
	}

	tokenPair, err := tokenService.RefreshToken(ctx, req.RefreshToken, securityCtx)
	if err != nil {
		s.logger.Debug("Token refresh failed", zap.Error(err))
		return &pb.RefreshTokenResponse{
			Error: "Invalid or expired refresh token",
		}, nil
	}

	return &pb.RefreshTokenResponse{
		AccessToken:  tokenPair.AccessToken,
		RefreshToken: tokenPair.RefreshToken,
		ExpiresIn:    int32(s.config.JWT.AccessExpiry),
	}, nil
}

// RevokeToken revokes a token with atomic operations
func (s *EnhancedAuthGRPCServer) RevokeToken(ctx context.Context, req *pb.RevokeTokenRequest) (*pb.RevokeTokenResponse, error) {
	if req.Token == "" {
		return &pb.RevokeTokenResponse{
			Success: false,
			Error:   "Token is required",
		}, nil
	}

	// Use token service for atomic revocation
	tokenService := s.getTokenService()
	err := tokenService.RevokeToken(ctx, req.Token)
	if err != nil {
		s.logger.Debug("Token revocation failed", zap.Error(err))
		return &pb.RevokeTokenResponse{
			Success: false,
			Error:   "Failed to revoke token",
		}, nil
	}

	return &pb.RevokeTokenResponse{
		Success: true,
	}, nil
}

// GetUser retrieves user with preloaded relationships and caching
func (s *EnhancedAuthGRPCServer) GetUser(ctx context.Context, req *pb.GetUserRequest) (*pb.GetUserResponse, error) {
	if req.UserId == "" {
		s.logger.Debug("GetUser called without user ID")
		return &pb.GetUserResponse{
			Error: "User ID is required",
		}, nil
	}

	userID, err := uuid.Parse(req.UserId)
	if err != nil {
		s.logger.Debug("GetUser called with invalid user ID", zap.String("user_id", req.UserId))
		return &pb.GetUserResponse{
			Error: "Invalid user ID format",
		}, nil
	}

	s.logger.Debug("Getting user", zap.String("user_id", userID.String()))

	// Test database connection first
	var count int64
	if err := s.db.Model(&models.User{}).Count(&count).Error; err != nil {
		s.logger.Error("Database connection test failed", zap.Error(err))
		return &pb.GetUserResponse{
			Error: "Database connection error",
		}, nil
	}
	s.logger.Debug("Database connection OK", zap.Int64("total_users", count))

	// Fetch directly from database with minimal query
	var user models.User

	// Add debug logging for the exact query
	s.logger.Info("Executing user lookup query",
		zap.String("user_id", userID.String()),
		zap.String("query", "SELECT * FROM users WHERE id = ?"))

	err = s.db.Where("id = ?", userID).First(&user).Error

	if err != nil {
		s.logger.Error("Database user lookup failed",
			zap.Error(err),
			zap.String("user_id", userID.String()),
			zap.String("error_message", err.Error()),
			zap.String("error_type", fmt.Sprintf("%T", err)))

		// Try alternative query methods to debug
		var userByString models.User
		stringErr := s.db.Where("id::text = ?", userID.String()).First(&userByString).Error
		s.logger.Debug("Alternative string query result",
			zap.Error(stringErr),
			zap.Bool("found_by_string", stringErr == nil))

		// Try to find any user with similar ID to debug
		var similarUsers []models.User
		s.db.Where("id::text LIKE ?", userID.String()[:8]+"%").Limit(5).Find(&similarUsers)
		s.logger.Debug("Similar users found", zap.Int("count", len(similarUsers)))

		// Try to get the first user to test basic connectivity
		var firstUser models.User
		firstErr := s.db.First(&firstUser).Error
		s.logger.Debug("First user query test",
			zap.Error(firstErr),
			zap.Bool("can_query_users", firstErr == nil))

		return &pb.GetUserResponse{
			Error: fmt.Sprintf("user not found: %s", userID.String()),
		}, nil
	}

	// Load organization separately if needed
	var org models.Organization
	if user.OrganizationID != uuid.Nil {
		s.db.Where("id = ?", user.OrganizationID).First(&org)
		user.Organization = org
	}

	s.logger.Info("User found in database",
		zap.String("user_id", userID.String()),
		zap.String("email", user.Email),
		zap.String("name", user.FirstName+" "+user.LastName))

	// Convert to protobuf with all relationships
	pbUser := s.convertUserToProto(&user)

	s.logger.Info("User converted to protobuf successfully",
		zap.String("user_id", userID.String()),
		zap.Bool("pb_user_nil", pbUser == nil))

	return &pb.GetUserResponse{
		User:  pbUser,
		Error: "", // Explicitly clear any error
	}, nil
}

// CheckPermission checks user permissions with intelligent caching strategies
func (s *EnhancedAuthGRPCServer) CheckPermission(ctx context.Context, req *pb.CheckPermissionRequest) (*pb.CheckPermissionResponse, error) {
	if req.UserId == "" || req.Resource == "" || req.Action == "" {
		return &pb.CheckPermissionResponse{
			HasPermission: false,
			Error:         "User ID, resource, and action are required",
		}, nil
	}

	userID, err := uuid.Parse(req.UserId)
	if err != nil {
		return &pb.CheckPermissionResponse{
			HasPermission: false,
			Error:         "Invalid user ID format",
		}, nil
	}

	// Use permission service with intelligent caching
	permissionService := s.getPermissionService()
	hasPermission, err := permissionService.CheckUserPermissionWithScope(ctx, userID, req.Resource, req.Action, "")
	if err != nil {
		s.logger.Error("Permission check failed", zap.Error(err),
			zap.String("user_id", userID.String()),
			zap.String("resource", req.Resource),
			zap.String("action", req.Action))
		return &pb.CheckPermissionResponse{
			HasPermission: false,
			Error:         "Failed to check permission",
		}, nil
	}

	return &pb.CheckPermissionResponse{
		HasPermission: hasPermission,
	}, nil
}

// Note: Authenticate method will be implemented once protobuf files are regenerated
// For now, focusing on the existing methods that work with current proto definitions

// CreateUser implements CreateUser method with transaction management
func (s *EnhancedAuthGRPCServer) CreateUser(ctx context.Context, req *pb.CreateUserRequest) (*pb.CreateUserResponse, error) {
	// Validate input
	if req.Email == "" || req.Password == "" || req.FirstName == "" || req.LastName == "" {
		return &pb.CreateUserResponse{
			Success: false,
			Error:   "Email, password, first name, and last name are required",
		}, nil
	}

	orgID, err := uuid.Parse(req.OrganizationId)
	if err != nil {
		return &pb.CreateUserResponse{
			Success: false,
			Error:   "Invalid organization ID format",
		}, nil
	}

	// Create security context
	securityCtx := &services.SecurityContext{
		IPAddress: "grpc-client", // Extract from metadata if available
		UserAgent: "grpc-client",
	}
	if req.SecurityContext != nil {
		securityCtx.IPAddress = req.SecurityContext.IpAddress
		securityCtx.UserAgent = req.SecurityContext.UserAgent
	}

	// Create registration request
	regReq := &services.RegistrationRequest{
		Email:           req.Email,
		Password:        req.Password,
		FirstName:       req.FirstName,
		LastName:        req.LastName,
		SecurityContext: securityCtx,
	}

	// For existing organization, we need a different method
	user, err := s.createUserInExistingOrganization(ctx, orgID, regReq)
	if err != nil {
		s.logger.Error("Failed to create user", zap.Error(err))
		return &pb.CreateUserResponse{
			Success: false,
			Error:   "Failed to create user: " + err.Error(),
		}, nil
	}

	// Convert to protobuf
	pbUser := s.convertUserToProto(user)

	// Invalidate relevant caches
	s.invalidateUserCaches(ctx, user.ID, user.OrganizationID)

	return &pb.CreateUserResponse{
		Success: true,
		User:    pbUser,
	}, nil
}

// UpdateUser implements UpdateUser method with cache invalidation
func (s *EnhancedAuthGRPCServer) UpdateUser(ctx context.Context, req *pb.UpdateUserRequest) (*pb.UpdateUserResponse, error) {
	// Validate input
	if req.UserId == "" {
		return &pb.UpdateUserResponse{
			Success: false,
			Error:   "User ID is required",
		}, nil
	}

	userID, err := uuid.Parse(req.UserId)
	if err != nil {
		return &pb.UpdateUserResponse{
			Success: false,
			Error:   "Invalid user ID format",
		}, nil
	}

	// Get existing user
	var user models.User
	_, err = s.circuitBreakerManager.Execute("database", func() (interface{}, error) {
		return nil, s.db.Where("id = ?", userID).First(&user).Error
	})

	if err != nil {
		return &pb.UpdateUserResponse{
			Success: false,
			Error:   "User not found",
		}, nil
	}

	// Update fields if provided
	if req.Email != "" {
		user.Email = req.Email
	}
	if req.FirstName != "" {
		user.FirstName = req.FirstName
	}
	if req.LastName != "" {
		user.LastName = req.LastName
	}
	user.IsActive = req.IsActive
	user.IsVerified = req.IsVerified
	user.UpdatedAt = time.Now()

	// Update user in database with transaction
	_, err = s.circuitBreakerManager.Execute("database", func() (interface{}, error) {
		return nil, s.db.Transaction(func(tx *gorm.DB) error {
			if err := tx.Save(&user).Error; err != nil {
				return err
			}

			// Update role assignments if provided
			if len(req.RoleIds) > 0 {
				// Remove existing roles
				if err := tx.Where("user_id = ?", userID).Delete(&models.UserRole{}).Error; err != nil {
					return err
				}

				// Add new roles
				for _, roleIDStr := range req.RoleIds {
					roleID, err := uuid.Parse(roleIDStr)
					if err != nil {
						continue // Skip invalid role IDs
					}

					userRole := models.UserRole{
						UserID: userID,
						RoleID: roleID,
					}
					if err := tx.Create(&userRole).Error; err != nil {
						return err
					}
				}
			}

			return nil
		})
	})

	if err != nil {
		s.logger.Error("Failed to update user", zap.Error(err))
		return &pb.UpdateUserResponse{
			Success: false,
			Error:   "Failed to update user",
		}, nil
	}

	// Invalidate caches
	s.invalidateUserCaches(ctx, user.ID, user.OrganizationID)

	// Publish user updated event
	s.publishUserUpdatedEvent(ctx, &user, req.SecurityContext)

	// Convert to protobuf
	pbUser := s.convertUserToProto(&user)

	return &pb.UpdateUserResponse{
		Success: true,
		User:    pbUser,
	}, nil
}

// ChangePassword implements ChangePassword method with security event publishing
func (s *EnhancedAuthGRPCServer) ChangePassword(ctx context.Context, req *pb.ChangePasswordRequest) (*pb.ChangePasswordResponse, error) {
	// Validate input
	if req.UserId == "" || req.CurrentPassword == "" || req.NewPassword == "" {
		return &pb.ChangePasswordResponse{
			Success: false,
			Error:   "User ID, current password, and new password are required",
		}, nil
	}

	userID, err := uuid.Parse(req.UserId)
	if err != nil {
		return &pb.ChangePasswordResponse{
			Success: false,
			Error:   "Invalid user ID format",
		}, nil
	}

	// Create security context
	securityCtx := &services.SecurityContext{
		IPAddress: "grpc-client",
		UserAgent: "grpc-client",
	}
	if req.SecurityContext != nil {
		securityCtx.IPAddress = req.SecurityContext.IpAddress
		securityCtx.UserAgent = req.SecurityContext.UserAgent
	}

	// Use auth service to change password
	authService := s.getAuthService()
	changeReq := &services.PasswordChangeRequest{
		UserID:          userID,
		CurrentPassword: req.CurrentPassword,
		NewPassword:     req.NewPassword,
		SecurityContext: securityCtx,
	}

	if err := authService.ChangePassword(ctx, changeReq); err != nil {
		s.logger.Error("Failed to change password", zap.Error(err))
		return &pb.ChangePasswordResponse{
			Success: false,
			Error:   "Failed to change password: " + err.Error(),
		}, nil
	}

	return &pb.ChangePasswordResponse{
		Success: true,
	}, nil
}

// CreateOrganization implements CreateOrganization method with initial role setup
func (s *EnhancedAuthGRPCServer) CreateOrganization(ctx context.Context, req *pb.CreateOrganizationRequest) (*pb.CreateOrganizationResponse, error) {
	// Validate input
	if req.Name == "" || req.Domain == "" || req.AdminEmail == "" || req.AdminPassword == "" {
		return &pb.CreateOrganizationResponse{
			Success: false,
			Error:   "Name, domain, admin email, and admin password are required",
		}, nil
	}

	// Create security context
	securityCtx := &services.SecurityContext{
		IPAddress: "grpc-client",
		UserAgent: "grpc-client",
	}
	if req.SecurityContext != nil {
		securityCtx.IPAddress = req.SecurityContext.IpAddress
		securityCtx.UserAgent = req.SecurityContext.UserAgent
	}

	// Use auth service to register organization with admin user
	authService := s.getAuthService()
	regReq := &services.RegistrationRequest{
		Email:              req.AdminEmail,
		Password:           req.AdminPassword,
		FirstName:          req.AdminFirstName,
		LastName:           req.AdminLastName,
		OrganizationName:   req.Name,
		OrganizationDomain: req.Domain,
		SecurityContext:    securityCtx,
	}

	regResp, err := authService.Register(ctx, regReq)
	if err != nil {
		s.logger.Error("Failed to create organization", zap.Error(err))
		return &pb.CreateOrganizationResponse{
			Success: false,
			Error:   "Failed to create organization: " + err.Error(),
		}, nil
	}

	if !regResp.Success {
		return &pb.CreateOrganizationResponse{
			Success: false,
			Error:   regResp.Error,
		}, nil
	}

	// Get organization details
	var org models.Organization
	_, err = s.circuitBreakerManager.Execute("database", func() (interface{}, error) {
		return nil, s.db.Where("id = ?", regResp.User.OrganizationID).First(&org).Error
	})

	if err != nil {
		s.logger.Error("Failed to get created organization", zap.Error(err))
		return &pb.CreateOrganizationResponse{
			Success: false,
			Error:   "Failed to retrieve created organization",
		}, nil
	}

	// Convert to protobuf
	pbOrg := s.convertOrganizationToProto(&org)
	pbUser := s.convertEntityUserToProto(regResp.User)
	pbTokens := s.convertEntityTokenPairToProto(regResp.TokenPair)

	return &pb.CreateOrganizationResponse{
		Success:      true,
		Organization: pbOrg,
		AdminUser:    pbUser,
		Tokens:       pbTokens,
	}, nil
}

// GetOrganization implements GetOrganization method
func (s *EnhancedAuthGRPCServer) GetOrganization(ctx context.Context, req *pb.GetOrganizationRequest) (*pb.GetOrganizationResponse, error) {
	if req.OrganizationId == "" {
		return &pb.GetOrganizationResponse{
			Error: "Organization ID is required",
		}, nil
	}

	orgID, err := uuid.Parse(req.OrganizationId)
	if err != nil {
		return &pb.GetOrganizationResponse{
			Error: "Invalid organization ID format",
		}, nil
	}

	// Try cache first
	cacheKey := fmt.Sprintf("org:id:%s", orgID.String())
	if cachedData, err := s.cacheManager.Get(ctx, cacheKey); err == nil {
		var cachedOrg pb.Organization
		if err := s.deserializeOrganization(cachedData, &cachedOrg); err == nil {
			return &pb.GetOrganizationResponse{Organization: &cachedOrg}, nil
		}
	}

	// Get from database with users preloaded
	var org models.Organization
	_, err = s.circuitBreakerManager.Execute("database", func() (interface{}, error) {
		return nil, s.db.Preload("Users").Where("id = ?", orgID).First(&org).Error
	})

	if err != nil {
		return &pb.GetOrganizationResponse{
			Error: "Organization not found",
		}, nil
	}

	// Convert to protobuf
	pbOrg := s.convertOrganizationToProto(&org)

	// Cache the result
	if serializedOrg, err := s.serializeOrganization(pbOrg); err == nil {
		s.cacheManager.Set(ctx, cacheKey, serializedOrg, time.Hour)
	}

	return &pb.GetOrganizationResponse{
		Organization: pbOrg,
	}, nil
}

// ListOrganizations implements ListOrganizations method
func (s *EnhancedAuthGRPCServer) ListOrganizations(ctx context.Context, req *pb.ListOrganizationsRequest) (*pb.ListOrganizationsResponse, error) {
	// Set defaults
	limit := req.Limit
	if limit <= 0 {
		limit = 10
	}
	offset := req.Offset
	if offset < 0 {
		offset = 0
	}

	// Get organizations from database
	var organizations []models.Organization
	query := s.db.WithContext(ctx).Model(&models.Organization{})

	// Add search filter if provided
	if req.Search != "" {
		query = query.Where("name ILIKE ? OR domain ILIKE ?", "%"+req.Search+"%", "%"+req.Search+"%")
	}

	// Add sorting
	sortBy := req.SortBy
	if sortBy == "" {
		sortBy = "created_at"
	}
	sortOrder := req.SortOrder
	if sortOrder == "" {
		sortOrder = "desc"
	}
	query = query.Order(fmt.Sprintf("%s %s", sortBy, sortOrder))

	// Get total count
	var totalCount int64
	if err := query.Count(&totalCount).Error; err != nil {
		s.logger.Error("Failed to count organizations", zap.Error(err))
		return &pb.ListOrganizationsResponse{
			Success: false,
			Error:   "Failed to count organizations",
		}, nil
	}

	// Get paginated results
	if err := query.Limit(int(limit)).Offset(int(offset)).Find(&organizations).Error; err != nil {
		s.logger.Error("Failed to list organizations", zap.Error(err))
		return &pb.ListOrganizationsResponse{
			Success: false,
			Error:   "Failed to list organizations",
		}, nil
	}

	// Convert to protobuf
	pbOrgs := make([]*pb.Organization, len(organizations))
	for i, org := range organizations {
		pbOrgs[i] = s.convertOrganizationToProto(&org)
	}

	hasNextPage := int64(offset+int32(len(organizations))) < totalCount

	return &pb.ListOrganizationsResponse{
		Success:       true,
		Organizations: pbOrgs,
		TotalCount:    int32(totalCount),
		HasNextPage:   hasNextPage,
	}, nil
}

// GetOrganizationStats implements GetOrganizationStats method
func (s *EnhancedAuthGRPCServer) GetOrganizationStats(ctx context.Context, req *pb.GetOrganizationStatsRequest) (*pb.GetOrganizationStatsResponse, error) {
	// Get organization statistics
	var totalOrgs, activeOrgs, inactiveOrgs int64
	var totalUsers int64

	// Total organizations
	if err := s.db.WithContext(ctx).Model(&models.Organization{}).Count(&totalOrgs).Error; err != nil {
		s.logger.Error("Failed to count total organizations", zap.Error(err))
		return &pb.GetOrganizationStatsResponse{
			Success: false,
			Error:   "Failed to get organization statistics",
		}, nil
	}

	// Active organizations
	if err := s.db.WithContext(ctx).Model(&models.Organization{}).Where("is_active = ?", true).Count(&activeOrgs).Error; err != nil {
		s.logger.Error("Failed to count active organizations", zap.Error(err))
		return &pb.GetOrganizationStatsResponse{
			Success: false,
			Error:   "Failed to get organization statistics",
		}, nil
	}

	inactiveOrgs = totalOrgs - activeOrgs

	// Total users across all organizations
	if err := s.db.WithContext(ctx).Model(&models.User{}).Count(&totalUsers).Error; err != nil {
		s.logger.Error("Failed to count total users", zap.Error(err))
		return &pb.GetOrganizationStatsResponse{
			Success: false,
			Error:   "Failed to get organization statistics",
		}, nil
	}

	// Calculate average users per organization
	var averageUsersPerOrg float32
	if totalOrgs > 0 {
		averageUsersPerOrg = float32(totalUsers) / float32(totalOrgs)
	}

	stats := &pb.OrganizationStats{
		TotalOrganizations:      int32(totalOrgs),
		ActiveOrganizations:     int32(activeOrgs),
		InactiveOrganizations:   int32(inactiveOrgs),
		VerifiedOrganizations:   int32(activeOrgs), // Assuming active = verified for now
		UnverifiedOrganizations: int32(inactiveOrgs),
		TotalUsers:              int32(totalUsers),
		AverageUsersPerOrg:      averageUsersPerOrg,
	}

	return &pb.GetOrganizationStatsResponse{
		Success: true,
		Stats:   stats,
	}, nil
}

// BulkCreateUsers implements bulk user creation for high-throughput scenarios
func (s *EnhancedAuthGRPCServer) BulkCreateUsers(ctx context.Context, req *pb.BulkCreateUsersRequest) (*pb.BulkCreateUsersResponse, error) {
	if req.OrganizationId == "" || len(req.Users) == 0 {
		return &pb.BulkCreateUsersResponse{
			Success: false,
			Error:   "Organization ID and users list are required",
		}, nil
	}

	orgID, err := uuid.Parse(req.OrganizationId)
	if err != nil {
		return &pb.BulkCreateUsersResponse{
			Success: false,
			Error:   "Invalid organization ID format",
		}, nil
	}

	results := make([]*pb.BulkUserResult, len(req.Users))
	createdCount := int32(0)
	failedCount := int32(0)

	// Process users in batches for better performance
	batchSize := 10
	for i := 0; i < len(req.Users); i += batchSize {
		end := i + batchSize
		if end > len(req.Users) {
			end = len(req.Users)
		}

		batch := req.Users[i:end]
		batchResults := s.processBulkUserCreation(ctx, orgID, batch, i)

		for j, result := range batchResults {
			results[i+j] = result
			if result.Success {
				createdCount++
			} else {
				failedCount++
			}
		}
	}

	return &pb.BulkCreateUsersResponse{
		Success:      createdCount > 0,
		Results:      results,
		CreatedCount: createdCount,
		FailedCount:  failedCount,
	}, nil
}

// BulkUpdateUsers implements bulk user updates for high-throughput scenarios
func (s *EnhancedAuthGRPCServer) BulkUpdateUsers(ctx context.Context, req *pb.BulkUpdateUsersRequest) (*pb.BulkUpdateUsersResponse, error) {
	if len(req.Users) == 0 {
		return &pb.BulkUpdateUsersResponse{
			Success: false,
			Error:   "Users list is required",
		}, nil
	}

	results := make([]*pb.BulkUserResult, len(req.Users))
	updatedCount := int32(0)
	failedCount := int32(0)

	// Process users in batches
	batchSize := 10
	for i := 0; i < len(req.Users); i += batchSize {
		end := i + batchSize
		if end > len(req.Users) {
			end = len(req.Users)
		}

		batch := req.Users[i:end]
		batchResults := s.processBulkUserUpdate(ctx, batch, i)

		for j, result := range batchResults {
			results[i+j] = result
			if result.Success {
				updatedCount++
			} else {
				failedCount++
			}
		}
	}

	return &pb.BulkUpdateUsersResponse{
		Success:      updatedCount > 0,
		Results:      results,
		UpdatedCount: updatedCount,
		FailedCount:  failedCount,
	}, nil
}

// BulkCheckPermissions implements bulk permission checking for high-throughput scenarios
func (s *EnhancedAuthGRPCServer) BulkCheckPermissions(ctx context.Context, req *pb.BulkCheckPermissionsRequest) (*pb.BulkCheckPermissionsResponse, error) {
	if len(req.Checks) == 0 {
		return &pb.BulkCheckPermissionsResponse{
			Success: false,
			Error:   "Permission checks list is required",
		}, nil
	}

	results := make([]*pb.PermissionResult, len(req.Checks))
	permissionService := s.getPermissionService()

	// Process permission checks concurrently for better performance
	type checkResult struct {
		index  int
		result *pb.PermissionResult
	}

	resultChan := make(chan checkResult, len(req.Checks))
	semaphore := make(chan struct{}, 20) // Limit concurrent checks

	for i, check := range req.Checks {
		go func(idx int, permCheck *pb.PermissionCheck) {
			semaphore <- struct{}{}        // Acquire semaphore
			defer func() { <-semaphore }() // Release semaphore

			result := &pb.PermissionResult{
				UserId:   permCheck.UserId,
				Resource: permCheck.Resource,
				Action:   permCheck.Action,
				Scope:    permCheck.Scope,
			}

			userID, err := uuid.Parse(permCheck.UserId)
			if err != nil {
				result.Error = "Invalid user ID format"
				resultChan <- checkResult{idx, result}
				return
			}

			hasPermission, err := permissionService.CheckUserPermissionWithScope(
				ctx, userID, permCheck.Resource, permCheck.Action, permCheck.Scope)
			if err != nil {
				result.Error = "Failed to check permission"
			} else {
				result.HasPermission = hasPermission
			}

			resultChan <- checkResult{idx, result}
		}(i, check)
	}

	// Collect results
	for i := 0; i < len(req.Checks); i++ {
		checkRes := <-resultChan
		results[checkRes.index] = checkRes.result
	}

	return &pb.BulkCheckPermissionsResponse{
		Success: true,
		Results: results,
	}, nil
}

func (s *EnhancedAuthGRPCServer) HealthCheck(ctx context.Context, req *pb.HealthCheckRequest) (*pb.HealthCheckResponse, error) {
	return &pb.HealthCheckResponse{
		Status:    "healthy",
		Service:   "auth-service",
		Timestamp: timestamppb.New(time.Now()),
	}, nil
}

func (s *EnhancedAuthGRPCServer) generateTokens(user models.User) (string, string, error) {
	// Access token claims
	accessClaims := middleware.Claims{
		UserID:         user.ID,
		OrganizationID: user.OrganizationID,
		Email:          user.Email,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Duration(s.config.JWT.AccessExpiry) * time.Second)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			Subject:   user.ID.String(),
		},
	}

	// Generate access token
	accessToken := jwt.NewWithClaims(jwt.SigningMethodHS256, accessClaims)
	accessTokenString, err := accessToken.SignedString([]byte(s.config.JWT.Secret))
	if err != nil {
		return "", "", err
	}

	// Refresh token claims
	refreshClaims := middleware.Claims{
		UserID:         user.ID,
		OrganizationID: user.OrganizationID,
		Email:          user.Email,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Duration(s.config.JWT.RefreshExpiry) * time.Second)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			Subject:   user.ID.String(),
		},
	}

	// Generate refresh token
	refreshToken := jwt.NewWithClaims(jwt.SigningMethodHS256, refreshClaims)
	refreshTokenString, err := refreshToken.SignedString([]byte(s.config.JWT.Secret))
	if err != nil {
		return "", "", err
	}

	return accessTokenString, refreshTokenString, nil
}

// getTokenService returns the token service instance
func (s *EnhancedAuthGRPCServer) getTokenService() *services.TokenService {
	return s.tokenService
}

// getPermissionService returns the permission service instance
func (s *EnhancedAuthGRPCServer) getPermissionService() *services.PermissionService {
	return s.permissionService
}

// getAuthService returns the auth service instance
func (s *EnhancedAuthGRPCServer) getAuthService() *services.AuthService {
	return s.authService
}

// min returns the minimum of two integers
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// convertUserToProto converts a models.User to pb.User with all relationships
func (s *EnhancedAuthGRPCServer) convertUserToProto(user *models.User) *pb.User {
	pbUser := &pb.User{
		Id:             user.ID.String(),
		OrganizationId: user.OrganizationID.String(),
		Email:          user.Email,
		FirstName:      user.FirstName,
		LastName:       user.LastName,
		IsActive:       user.IsActive,
		IsVerified:     user.IsVerified,
		CreatedAt:      timestamppb.New(user.CreatedAt),
		UpdatedAt:      timestamppb.New(user.UpdatedAt),
	}

	if user.LastLoginAt != nil {
		pbUser.LastLoginAt = timestamppb.New(*user.LastLoginAt)
	}

	// Add organization if loaded
	if user.Organization.ID != uuid.Nil {
		pbUser.Organization = &pb.Organization{
			Id:        user.Organization.ID.String(),
			Name:      user.Organization.Name,
			Domain:    user.Organization.Domain,
			IsActive:  user.Organization.IsActive,
			CreatedAt: timestamppb.New(user.Organization.CreatedAt),
			UpdatedAt: timestamppb.New(user.Organization.UpdatedAt),
		}
	}

	return pbUser
}

// serializeUser serializes a pb.User to bytes for caching
func (s *EnhancedAuthGRPCServer) serializeUser(user *pb.User) ([]byte, error) {
	// Use protobuf marshaling for efficient serialization
	return user.ProtoReflect().Interface().(interface{ Marshal() ([]byte, error) }).Marshal()
}

// deserializeUser deserializes bytes to a pb.User from cache
func (s *EnhancedAuthGRPCServer) deserializeUser(data []byte, user *pb.User) error {
	// Use protobuf unmarshaling for efficient deserialization
	return user.ProtoReflect().Interface().(interface{ Unmarshal([]byte) error }).Unmarshal(data)
}

// Helper methods for new gRPC operations

// createUserInExistingOrganization creates a user in an existing organization
func (s *EnhancedAuthGRPCServer) createUserInExistingOrganization(ctx context.Context, orgID uuid.UUID, req *services.RegistrationRequest) (*models.User, error) {
	// Hash password
	hashedPassword, err := s.hashPassword(req.Password)
	if err != nil {
		return nil, fmt.Errorf("failed to hash password: %w", err)
	}

	// Create user in transaction
	var user models.User
	_, err = s.circuitBreakerManager.Execute("database", func() (interface{}, error) {
		return nil, s.db.Transaction(func(tx *gorm.DB) error {
			// Check if organization exists
			var org models.Organization
			if err := tx.Where("id = ?", orgID).First(&org).Error; err != nil {
				return fmt.Errorf("organization not found: %w", err)
			}

			// Check if email already exists
			var existingUser models.User
			if err := tx.Where("email = ?", req.Email).First(&existingUser).Error; err == nil {
				return fmt.Errorf("user with email %s already exists", req.Email)
			}

			// Create user
			user = models.User{
				ID:             uuid.New(),
				OrganizationID: orgID,
				Email:          req.Email,
				PasswordHash:   hashedPassword,
				FirstName:      req.FirstName,
				LastName:       req.LastName,
				IsActive:       true,
				IsVerified:     false,
				CreatedAt:      time.Now(),
				UpdatedAt:      time.Now(),
			}

			if err := tx.Create(&user).Error; err != nil {
				return fmt.Errorf("failed to create user: %w", err)
			}

			// Assign default role (if exists)
			var defaultRole models.Role
			if err := tx.Where("organization_id = ? AND name = ?", orgID, "user").First(&defaultRole).Error; err == nil {
				userRole := models.UserRole{
					UserID: user.ID,
					RoleID: defaultRole.ID,
				}
				tx.Create(&userRole)
			}

			return nil
		})
	})

	if err != nil {
		return nil, err
	}

	// Publish user created event
	s.publishUserCreatedEvent(ctx, &user, req.SecurityContext)

	return &user, nil
}

// invalidateUserCaches invalidates all caches related to a user
func (s *EnhancedAuthGRPCServer) invalidateUserCaches(ctx context.Context, userID, orgID uuid.UUID) {
	cacheKeys := []string{
		fmt.Sprintf("user:id:%s", userID.String()),
		fmt.Sprintf("user:full:%s", userID.String()),
		fmt.Sprintf("user:permissions:%s", userID.String()),
		fmt.Sprintf("org:users:%s", orgID.String()),
	}

	for _, key := range cacheKeys {
		if err := s.cacheManager.Delete(ctx, key); err != nil {
			s.logger.Warn("Failed to invalidate cache", zap.String("key", key), zap.Error(err))
		}
	}
}

// publishUserUpdatedEvent publishes a user updated event
func (s *EnhancedAuthGRPCServer) publishUserUpdatedEvent(ctx context.Context, user *models.User, securityCtx *pb.SecurityContext) {
	// This would integrate with the event publisher
	s.logger.Info("User updated",
		zap.String("user_id", user.ID.String()),
		zap.String("email", user.Email))
}

// publishUserCreatedEvent publishes a user created event
func (s *EnhancedAuthGRPCServer) publishUserCreatedEvent(ctx context.Context, user *models.User, securityCtx *services.SecurityContext) {
	// This would integrate with the event publisher
	s.logger.Info("User created",
		zap.String("user_id", user.ID.String()),
		zap.String("email", user.Email))
}

// convertOrganizationToProto converts a models.Organization to pb.Organization
func (s *EnhancedAuthGRPCServer) convertOrganizationToProto(org *models.Organization) *pb.Organization {
	pbOrg := &pb.Organization{
		Id:        org.ID.String(),
		Name:      org.Name,
		Domain:    org.Domain,
		IsActive:  org.IsActive,
		CreatedAt: timestamppb.New(org.CreatedAt),
		UpdatedAt: timestamppb.New(org.UpdatedAt),
	}

	// Convert users if loaded
	if len(org.Users) > 0 {
		pbOrg.Users = make([]*pb.User, len(org.Users))
		for i, user := range org.Users {
			pbOrg.Users[i] = s.convertUserToProto(&user)
		}
	}

	// Set user counts
	pbOrg.UserCount = int32(len(org.Users))
	activeCount := int32(0)
	for _, user := range org.Users {
		if user.IsActive {
			activeCount++
		}
	}
	pbOrg.ActiveUserCount = activeCount

	// Convert roles if loaded
	if len(org.Roles) > 0 {
		pbOrg.Roles = make([]*pb.Role, len(org.Roles))
		for i, role := range org.Roles {
			pbOrg.Roles[i] = s.convertRoleToProto(&role)
		}
	}

	return pbOrg
}

// convertRoleToProto converts a models.Role to pb.Role
func (s *EnhancedAuthGRPCServer) convertRoleToProto(role *models.Role) *pb.Role {
	pbRole := &pb.Role{
		Id:             role.ID.String(),
		OrganizationId: role.OrganizationID.String(),
		Name:           role.Name,
		Description:    role.Description,
		IsSystem:       role.IsSystem,
		IsActive:       role.IsActive,
		CreatedAt:      timestamppb.New(role.CreatedAt),
		UpdatedAt:      timestamppb.New(role.UpdatedAt),
	}

	// Convert permissions if loaded through RolePermissions
	if len(role.RolePermissions) > 0 {
		pbRole.Permissions = make([]*pb.Permission, len(role.RolePermissions))
		for i, rolePermission := range role.RolePermissions {
			pbRole.Permissions[i] = s.convertPermissionToProto(&rolePermission.Permission)
		}
	}

	return pbRole
}

// convertPermissionToProto converts a models.Permission to pb.Permission
func (s *EnhancedAuthGRPCServer) convertPermissionToProto(perm *models.Permission) *pb.Permission {
	return &pb.Permission{
		Id:          perm.ID.String(),
		Name:        perm.Name,
		Description: perm.Description,
		Resource:    perm.Resource,
		Action:      perm.Action,
		Scope:       perm.Scope,
		IsSystem:    perm.IsSystem,
		CreatedAt:   timestamppb.New(perm.CreatedAt),
		UpdatedAt:   timestamppb.New(perm.UpdatedAt),
	}
}

// convertEntityUserToProto converts an entities.User to pb.User
func (s *EnhancedAuthGRPCServer) convertEntityUserToProto(user *entities.User) *pb.User {
	pbUser := &pb.User{
		Id:             user.ID.String(),
		OrganizationId: user.OrganizationID.String(),
		Email:          user.Email,
		FirstName:      user.FirstName,
		LastName:       user.LastName,
		IsActive:       user.IsActive,
		IsVerified:     user.IsVerified,
		CreatedAt:      timestamppb.New(user.CreatedAt),
		UpdatedAt:      timestamppb.New(user.UpdatedAt),
	}

	if user.LastLoginAt != nil {
		pbUser.LastLoginAt = timestamppb.New(*user.LastLoginAt)
	}

	return pbUser
}

// convertEntityTokenPairToProto converts an entities.TokenPair to pb.TokenPair
func (s *EnhancedAuthGRPCServer) convertEntityTokenPairToProto(tokenPair *entities.TokenPair) *pb.TokenPair {
	return &pb.TokenPair{
		AccessToken:      tokenPair.AccessToken,
		RefreshToken:     tokenPair.RefreshToken,
		ExpiresAt:        timestamppb.New(tokenPair.ExpiresAt),
		RefreshExpiresAt: timestamppb.New(tokenPair.RefreshExpiresAt),
		TokenType:        tokenPair.TokenType,
	}
}

// serializeOrganization serializes a pb.Organization to bytes for caching
func (s *EnhancedAuthGRPCServer) serializeOrganization(org *pb.Organization) ([]byte, error) {
	return proto.Marshal(org)
}

// deserializeOrganization deserializes bytes to a pb.Organization from cache
func (s *EnhancedAuthGRPCServer) deserializeOrganization(data []byte, org *pb.Organization) error {
	return proto.Unmarshal(data, org)
}

// processBulkUserCreation processes a batch of user creation requests
func (s *EnhancedAuthGRPCServer) processBulkUserCreation(ctx context.Context, orgID uuid.UUID, users []*pb.CreateUserData, startIndex int) []*pb.BulkUserResult {
	results := make([]*pb.BulkUserResult, len(users))

	for i, userData := range users {
		result := &pb.BulkUserResult{
			Email: userData.Email,
		}

		// Validate user data
		if userData.Email == "" || userData.Password == "" || userData.FirstName == "" || userData.LastName == "" {
			result.Success = false
			result.Error = "Email, password, first name, and last name are required"
			results[i] = result
			continue
		}

		// Create user
		regReq := &services.RegistrationRequest{
			Email:           userData.Email,
			Password:        userData.Password,
			FirstName:       userData.FirstName,
			LastName:        userData.LastName,
			SecurityContext: &services.SecurityContext{IPAddress: "bulk-operation", UserAgent: "grpc-bulk"},
		}

		user, err := s.createUserInExistingOrganization(ctx, orgID, regReq)
		if err != nil {
			result.Success = false
			result.Error = err.Error()
		} else {
			result.Success = true
			result.User = s.convertUserToProto(user)
		}

		results[i] = result
	}

	return results
}

// processBulkUserUpdate processes a batch of user update requests
func (s *EnhancedAuthGRPCServer) processBulkUserUpdate(ctx context.Context, users []*pb.UpdateUserData, startIndex int) []*pb.BulkUserResult {
	results := make([]*pb.BulkUserResult, len(users))

	for i, userData := range users {
		result := &pb.BulkUserResult{
			Email: userData.Email,
		}

		// Validate user data
		if userData.UserId == "" {
			result.Success = false
			result.Error = "User ID is required"
			results[i] = result
			continue
		}

		_, err := uuid.Parse(userData.UserId)
		if err != nil {
			result.Success = false
			result.Error = "Invalid user ID format"
			results[i] = result
			continue
		}

		// Update user
		updateReq := &pb.UpdateUserRequest{
			UserId:     userData.UserId,
			Email:      userData.Email,
			FirstName:  userData.FirstName,
			LastName:   userData.LastName,
			IsActive:   userData.IsActive,
			IsVerified: userData.IsVerified,
			RoleIds:    userData.RoleIds,
			SecurityContext: &pb.SecurityContext{
				IpAddress: "bulk-operation",
				UserAgent: "grpc-bulk",
			},
		}

		updateResp, err := s.UpdateUser(ctx, updateReq)
		if err != nil {
			result.Success = false
			result.Error = err.Error()
		} else if !updateResp.Success {
			result.Success = false
			result.Error = updateResp.Error
		} else {
			result.Success = true
			result.User = updateResp.User
		}

		results[i] = result
	}

	return results
}

// hashPassword hashes a password using bcrypt
func (s *EnhancedAuthGRPCServer) hashPassword(password string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	return string(hash), nil
}

// SimpleCacheManager is a fallback cache manager implementation
type SimpleCacheManager struct {
	redisClient *redis.Client
}

func (s *SimpleCacheManager) Get(ctx context.Context, key string) ([]byte, error) {
	return s.redisClient.Get(ctx, key).Bytes()
}

func (s *SimpleCacheManager) Set(ctx context.Context, key string, value []byte, ttl time.Duration) error {
	return s.redisClient.Set(ctx, key, value, ttl).Err()
}

func (s *SimpleCacheManager) Delete(ctx context.Context, key string) error {
	return s.redisClient.Del(ctx, key).Err()
}

func (s *SimpleCacheManager) Exists(ctx context.Context, key string) (bool, error) {
	result, err := s.redisClient.Exists(ctx, key).Result()
	return result > 0, err
}

func (s *SimpleCacheManager) Clear(ctx context.Context) error {
	return s.redisClient.FlushDB(ctx).Err()
}

func (s *SimpleCacheManager) Stats() cache.CacheStats {
	return cache.CacheStats{}
}

func (s *SimpleCacheManager) Close() error {
	return nil
}

func (s *SimpleCacheManager) GetFromLevel(ctx context.Context, key string, level cache.CacheLevel) ([]byte, error) {
	return s.Get(ctx, key)
}

func (s *SimpleCacheManager) SetToLevel(ctx context.Context, key string, value []byte, ttl time.Duration, level cache.CacheLevel) error {
	return s.Set(ctx, key, value, ttl)
}

func (s *SimpleCacheManager) InvalidateKey(ctx context.Context, key string) error {
	return s.Delete(ctx, key)
}

func (s *SimpleCacheManager) InvalidatePattern(ctx context.Context, pattern string) error {
	keys, err := s.redisClient.Keys(ctx, pattern).Result()
	if err != nil {
		return err
	}
	if len(keys) > 0 {
		return s.redisClient.Del(ctx, keys...).Err()
	}
	return nil
}

func (s *SimpleCacheManager) WarmCache(ctx context.Context, keys []string) error {
	return nil // No-op for simple implementation
}

func (s *SimpleCacheManager) GetCache(name string) cache.Cache {
	return s
}

func (s *SimpleCacheManager) RegisterCache(name string, cache cache.Cache) error {
	return nil // No-op for simple implementation
}

func (s *SimpleCacheManager) SetStrategy(strategy cache.CacheStrategy) {
	// No-op for simple implementation
}

func (s *SimpleCacheManager) GetStrategy() cache.CacheStrategy {
	return cache.WriteThrough
}

func (s *SimpleCacheManager) Sync(ctx context.Context) error {
	return nil // No-op for simple implementation
}

// KafkaEventPublisher implements the EventPublisherInterface for Kafka
type KafkaEventPublisher struct {
	producer *events.Producer
	logger   *zap.Logger
}

func (k *KafkaEventPublisher) PublishUserLoggedIn(ctx context.Context, data events.UserLoggedInData, userID, orgID uuid.UUID, ipAddress, userAgent string) error {
	event := events.BaseEvent{
		ID:             uuid.New(),
		Type:           events.EventTypeUserLoggedIn,
		Source:         "auth-service",
		Timestamp:      time.Now(),
		UserID:         &userID,
		OrganizationID: &orgID,
		IPAddress:      ipAddress,
		UserAgent:      userAgent,
		Data:           data,
	}
	return k.producer.PublishEvent(ctx, event)
}

func (k *KafkaEventPublisher) PublishUserRegistered(ctx context.Context, data events.UserRegisteredData, userID, orgID uuid.UUID, ipAddress, userAgent string) error {
	event := events.BaseEvent{
		ID:             uuid.New(),
		Type:           events.EventTypeUserRegistered,
		Source:         "auth-service",
		Timestamp:      time.Now(),
		UserID:         &userID,
		OrganizationID: &orgID,
		IPAddress:      ipAddress,
		UserAgent:      userAgent,
		Data:           data,
	}
	return k.producer.PublishEvent(ctx, event)
}

func (k *KafkaEventPublisher) PublishOrganizationCreated(ctx context.Context, data events.OrganizationCreatedData, userID, orgID uuid.UUID, ipAddress, userAgent string) error {
	event := events.BaseEvent{
		ID:             uuid.New(),
		Type:           events.EventTypeOrganizationCreated,
		Source:         "auth-service",
		Timestamp:      time.Now(),
		UserID:         &userID,
		OrganizationID: &orgID,
		IPAddress:      ipAddress,
		UserAgent:      userAgent,
		Data:           data,
	}
	return k.producer.PublishEvent(ctx, event)
}

func (k *KafkaEventPublisher) PublishPasswordChanged(ctx context.Context, data events.PasswordChangedData, userID, orgID uuid.UUID, ipAddress, userAgent string) error {
	event := events.BaseEvent{
		ID:             uuid.New(),
		Type:           events.EventTypePasswordChanged,
		Source:         "auth-service",
		Timestamp:      time.Now(),
		UserID:         &userID,
		OrganizationID: &orgID,
		IPAddress:      ipAddress,
		UserAgent:      userAgent,
		Data:           data,
	}
	return k.producer.PublishEvent(ctx, event)
}

// ListUsers implements ListUsers method for user management
func (s *EnhancedAuthGRPCServer) ListUsers(ctx context.Context, req *pb.ListUsersRequest) (*pb.ListUsersResponse, error) {
	// Set defaults
	limit := req.Limit
	if limit <= 0 {
		limit = 10
	}
	offset := req.Offset
	if offset < 0 {
		offset = 0
	}

	// Get users from database
	var users []models.User
	var totalCount int64

	// Build query - if OrganizationId is empty, list all users (for app admins)
	query := s.db.WithContext(ctx)

	if req.OrganizationId != "" {
		// Parse organization ID for organization-specific queries
		orgID, err := uuid.Parse(req.OrganizationId)
		if err != nil {
			return &pb.ListUsersResponse{
				Success: false,
				Error:   "Invalid organization ID format",
			}, nil
		}
		query = query.Where("organization_id = ?", orgID)
	}
	// If OrganizationId is empty, query all users (no WHERE clause for organization)

	// Add search filter if provided
	if req.Search != "" {
		searchTerm := "%" + req.Search + "%"
		query = query.Where("first_name ILIKE ? OR last_name ILIKE ? OR email ILIKE ?",
			searchTerm, searchTerm, searchTerm)
	}

	// Get total count
	if err := query.Model(&models.User{}).Count(&totalCount).Error; err != nil {
		s.logger.Error("Failed to count users", zap.Error(err))
		return &pb.ListUsersResponse{
			Success: false,
			Error:   "Failed to count users",
		}, nil
	}

	// Add sorting
	sortBy := req.SortBy
	if sortBy == "" {
		sortBy = "created_at"
	}
	sortOrder := req.SortOrder
	if sortOrder == "" {
		sortOrder = "desc"
	}
	query = query.Order(fmt.Sprintf("%s %s", sortBy, sortOrder))

	// Add pagination
	query = query.Limit(int(limit)).Offset(int(offset))

	// Execute query with preloading
	if err := query.Preload("Organization").Find(&users).Error; err != nil {
		s.logger.Error("Failed to list users", zap.Error(err))
		return &pb.ListUsersResponse{
			Success: false,
			Error:   "Failed to retrieve users",
		}, nil
	}

	// Convert to protobuf
	pbUsers := make([]*pb.User, len(users))
	for i, user := range users {
		pbUsers[i] = s.convertUserToProto(&user)
	}

	return &pb.ListUsersResponse{
		Success:     true,
		Users:       pbUsers,
		TotalCount:  int32(totalCount),
		HasNextPage: int32(offset+limit) < int32(totalCount),
	}, nil
}

// GetUserStats implements GetUserStats method for dashboard statistics
func (s *EnhancedAuthGRPCServer) GetUserStats(ctx context.Context, req *pb.GetUserStatsRequest) (*pb.GetUserStatsResponse, error) {
	// Validate input
	if req.OrganizationId == "" {
		return &pb.GetUserStatsResponse{
			Success: false,
			Error:   "Organization ID is required",
		}, nil
	}

	// Parse organization ID
	orgID, err := uuid.Parse(req.OrganizationId)
	if err != nil {
		return &pb.GetUserStatsResponse{
			Success: false,
			Error:   "Invalid organization ID format",
		}, nil
	}

	// Get user statistics
	var stats pb.UserStats

	// Total users
	var totalUsers int64
	if err := s.db.WithContext(ctx).Model(&models.User{}).Where("organization_id = ?", orgID).Count(&totalUsers).Error; err != nil {
		s.logger.Error("Failed to count total users", zap.Error(err))
		return &pb.GetUserStatsResponse{Success: false, Error: "Failed to get user statistics"}, nil
	}
	stats.TotalUsers = int32(totalUsers)

	// Active users
	var activeUsers int64
	if err := s.db.WithContext(ctx).Model(&models.User{}).Where("organization_id = ? AND is_active = ?", orgID, true).Count(&activeUsers).Error; err != nil {
		s.logger.Error("Failed to count active users", zap.Error(err))
		return &pb.GetUserStatsResponse{Success: false, Error: "Failed to get user statistics"}, nil
	}
	stats.ActiveUsers = int32(activeUsers)
	stats.InactiveUsers = int32(totalUsers - activeUsers)

	// Verified users
	var verifiedUsers int64
	if err := s.db.WithContext(ctx).Model(&models.User{}).Where("organization_id = ? AND is_verified = ?", orgID, true).Count(&verifiedUsers).Error; err != nil {
		s.logger.Error("Failed to count verified users", zap.Error(err))
		return &pb.GetUserStatsResponse{Success: false, Error: "Failed to get user statistics"}, nil
	}
	stats.VerifiedUsers = int32(verifiedUsers)
	stats.UnverifiedUsers = int32(totalUsers - verifiedUsers)

	// Recent signups (last 7 days)
	var recentSignups int64
	sevenDaysAgo := time.Now().AddDate(0, 0, -7)
	if err := s.db.WithContext(ctx).Model(&models.User{}).Where("organization_id = ? AND created_at >= ?", orgID, sevenDaysAgo).Count(&recentSignups).Error; err != nil {
		s.logger.Error("Failed to count recent signups", zap.Error(err))
		return &pb.GetUserStatsResponse{Success: false, Error: "Failed to get user statistics"}, nil
	}
	stats.RecentSignups = int32(recentSignups)

	// Recent logins (last 7 days)
	var recentLogins int64
	if err := s.db.WithContext(ctx).Model(&models.User{}).Where("organization_id = ? AND last_login_at >= ?", orgID, sevenDaysAgo).Count(&recentLogins).Error; err != nil {
		s.logger.Error("Failed to count recent logins", zap.Error(err))
		return &pb.GetUserStatsResponse{Success: false, Error: "Failed to get user statistics"}, nil
	}
	stats.RecentLogins = int32(recentLogins)

	return &pb.GetUserStatsResponse{
		Success: true,
		Stats:   &stats,
	}, nil
}

// GetSecurityStats implements GetSecurityStats method for security dashboard
func (s *EnhancedAuthGRPCServer) GetSecurityStats(ctx context.Context, req *pb.GetSecurityStatsRequest) (*pb.GetSecurityStatsResponse, error) {
	// Validate input
	if req.OrganizationId == "" {
		s.logger.Debug("GetSecurityStats called without organization ID")
		return &pb.GetSecurityStatsResponse{
			Success: false,
			Error:   "Organization ID is required",
		}, nil
	}

	// Parse organization ID
	orgID, err := uuid.Parse(req.OrganizationId)
	if err != nil {
		s.logger.Debug("GetSecurityStats called with invalid organization ID", zap.String("org_id", req.OrganizationId))
		return &pb.GetSecurityStatsResponse{
			Success: false,
			Error:   "Invalid organization ID format",
		}, nil
	}

	s.logger.Debug("Getting security stats", zap.String("org_id", orgID.String()))

	// Get security statistics via activity service (ES if enabled, otherwise DB)
	var stats pb.SecurityStats
	if s.activityService != nil {
		m, err := s.activityService.GetSecurityStats(ctx, orgID)
		if err != nil {
			s.logger.Warn("GetSecurityStats: activity service failed, using defaults", zap.Error(err))
		} else {
			stats.FailedLoginsToday = int32(m["failed_logins_today"])
			stats.SecurityAlerts = int32(m["security_alerts"])
			// Optional metrics
			if v, ok := m["two_factor_enabled"]; ok {
				stats.TwoFactorEnabled = int32(v)
			}
			if v, ok := m["password_resets_today"]; ok {
				stats.PasswordResetsToday = int32(v)
			}
		}
	}
	// Locked accounts not tracked here; keep 0 unless added elsewhere
	// Ensure non-negative defaults

	s.logger.Debug("Security stats retrieved successfully",
		zap.String("org_id", orgID.String()),
		zap.Int32("two_factor_enabled", stats.TwoFactorEnabled))

	return &pb.GetSecurityStatsResponse{
		Success: true,
		Stats:   &stats,
	}, nil
}

// GetUserActivity implements GetUserActivity method for activity logs
func (s *EnhancedAuthGRPCServer) GetUserActivity(ctx context.Context, req *pb.GetUserActivityRequest) (*pb.GetUserActivityResponse, error) {
	// Parse organization ID if provided
	var orgID *uuid.UUID
	if req.OrganizationId != "" {
		parsedOrgID, err := uuid.Parse(req.OrganizationId)
		if err != nil {
			return &pb.GetUserActivityResponse{
				Success: false,
				Error:   "Invalid organization ID format",
			}, nil
		}
		orgID = &parsedOrgID
	}
	// If OrganizationId is empty, orgID will be nil, which means "all organizations" for super admin

	// Parse user ID if provided
	var userID *uuid.UUID
	if req.UserId != "" {
		parsedUserID, err := uuid.Parse(req.UserId)
		if err != nil {
			return &pb.GetUserActivityResponse{
				Success: false,
				Error:   "Invalid user ID format",
			}, nil
		}
		userID = &parsedUserID
	}

	// Set defaults for pagination
	limit := int(req.Limit)
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	offset := int(req.Offset)
	if offset < 0 {
		offset = 0
	}

	// Get activities from activity service (ES if enabled, otherwise DB)
	activities, total, err := s.activityService.GetUserActivities(ctx, userID, orgID, limit, offset)
	if err != nil {
		s.logger.Error("Failed to get user activities", zap.Error(err))
		return &pb.GetUserActivityResponse{
			Success: false,
			Error:   "Failed to retrieve user activities",
		}, nil
	}

	// Convert to protobuf format
	pbActivities := make([]*pb.UserActivity, len(activities))
	for i, activity := range activities {
		pbActivity := &pb.UserActivity{
			Id:        activity.ID.String(),
			UserId:    activity.UserID.String(),
			Action:    activity.Action,
			Resource:  activity.Resource,
			Details:   activity.Details,
			IpAddress: activity.IPAddress,
			UserAgent: activity.UserAgent,
			CreatedAt: timestamppb.New(activity.CreatedAt),
		}

		// Add user information if available
		if activity.User.ID != uuid.Nil {
			pbActivity.User = &pb.User{
				Id:        activity.User.ID.String(),
				FirstName: activity.User.FirstName,
				LastName:  activity.User.LastName,
				Email:     activity.User.Email,
			}
		}

		pbActivities[i] = pbActivity
	}

	return &pb.GetUserActivityResponse{
		Success:    true,
		Activities: pbActivities,
		TotalCount: int32(total),
	}, nil
}

// ListRoles implements ListRoles method for role management
func (s *EnhancedAuthGRPCServer) ListRoles(ctx context.Context, req *pb.ListRolesRequest) (*pb.ListRolesResponse, error) {
	// Validate input
	if req.OrganizationId == "" {
		return &pb.ListRolesResponse{
			Success: false,
			Error:   "Organization ID is required",
		}, nil
	}

	// Parse organization ID
	orgID, err := uuid.Parse(req.OrganizationId)
	if err != nil {
		return &pb.ListRolesResponse{
			Success: false,
			Error:   "Invalid organization ID format",
		}, nil
	}

	// Get roles from database
	var roles []models.Role
	if err := s.db.WithContext(ctx).Where("organization_id = ?", orgID).Preload("RolePermissions.Permission").Find(&roles).Error; err != nil {
		s.logger.Error("Failed to list roles", zap.Error(err))
		return &pb.ListRolesResponse{
			Success: false,
			Error:   "Failed to retrieve roles",
		}, nil
	}

	// Convert to protobuf
	pbRoles := make([]*pb.Role, len(roles))
	for i, role := range roles {
		permissions := make([]*pb.Permission, len(role.RolePermissions))
		for j, rp := range role.RolePermissions {
			permissions[j] = &pb.Permission{
				Id:          rp.Permission.ID.String(),
				Name:        rp.Permission.Name,
				Resource:    rp.Permission.Resource,
				Action:      rp.Permission.Action,
				Scope:       rp.Permission.Scope,
				Description: rp.Permission.Description,
				IsSystem:    rp.Permission.IsSystem,
				CreatedAt:   timestamppb.New(rp.Permission.CreatedAt),
				UpdatedAt:   timestamppb.New(rp.Permission.UpdatedAt),
			}
		}

		pbRoles[i] = &pb.Role{
			Id:             role.ID.String(),
			OrganizationId: role.OrganizationID.String(),
			Name:           role.Name,
			Description:    role.Description,
			IsSystem:       role.IsSystem,
			IsActive:       role.IsActive,
			Permissions:    permissions,
			CreatedAt:      timestamppb.New(role.CreatedAt),
			UpdatedAt:      timestamppb.New(role.UpdatedAt),
		}
	}

	return &pb.ListRolesResponse{
		Success: true,
		Roles:   pbRoles,
	}, nil
}

// ListPermissions implements ListPermissions method for permission management
func (s *EnhancedAuthGRPCServer) ListPermissions(ctx context.Context, req *pb.ListPermissionsRequest) (*pb.ListPermissionsResponse, error) {
	// Get all permissions from database
	var permissions []models.Permission
	if err := s.db.WithContext(ctx).Find(&permissions).Error; err != nil {
		s.logger.Error("Failed to list permissions", zap.Error(err))
		return &pb.ListPermissionsResponse{
			Success: false,
			Error:   "Failed to retrieve permissions",
		}, nil
	}

	// Convert to protobuf
	pbPermissions := make([]*pb.Permission, len(permissions))
	for i, perm := range permissions {
		pbPermissions[i] = &pb.Permission{
			Id:          perm.ID.String(),
			Name:        perm.Name,
			Resource:    perm.Resource,
			Action:      perm.Action,
			Scope:       perm.Scope,
			Description: perm.Description,
			IsSystem:    perm.IsSystem,
			CreatedAt:   timestamppb.New(perm.CreatedAt),
			UpdatedAt:   timestamppb.New(perm.UpdatedAt),
		}
	}

	return &pb.ListPermissionsResponse{
		Success:     true,
		Permissions: pbPermissions,
	}, nil
}

// DeleteUser implements DeleteUser method for user management
func (s *EnhancedAuthGRPCServer) DeleteUser(ctx context.Context, req *pb.DeleteUserRequest) (*pb.DeleteUserResponse, error) {
	// Validate input
	if req.UserId == "" {
		return &pb.DeleteUserResponse{
			Success: false,
			Error:   "User ID is required",
		}, nil
	}

	// Parse user ID
	userID, err := uuid.Parse(req.UserId)
	if err != nil {
		return &pb.DeleteUserResponse{
			Success: false,
			Error:   "Invalid user ID format",
		}, nil
	}

	// Check if user exists
	var user models.User
	if err := s.db.WithContext(ctx).First(&user, "id = ?", userID).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return &pb.DeleteUserResponse{
				Success: false,
				Error:   "User not found",
			}, nil
		}
		s.logger.Error("Failed to find user", zap.Error(err))
		return &pb.DeleteUserResponse{
			Success: false,
			Error:   "Failed to find user",
		}, nil
	}

	// Delete user (soft delete)
	if err := s.db.WithContext(ctx).Delete(&user).Error; err != nil {
		s.logger.Error("Failed to delete user", zap.Error(err))
		return &pb.DeleteUserResponse{
			Success: false,
			Error:   "Failed to delete user",
		}, nil
	}

	// Invalidate caches
	s.invalidateUserCaches(ctx, userID, user.OrganizationID)

	return &pb.DeleteUserResponse{
		Success: true,
	}, nil
}

// ActivateUser implements ActivateUser method for user management
func (s *EnhancedAuthGRPCServer) ActivateUser(ctx context.Context, req *pb.ActivateUserRequest) (*pb.ActivateUserResponse, error) {
	// Validate input
	if req.UserId == "" {
		return &pb.ActivateUserResponse{
			Success: false,
			Error:   "User ID is required",
		}, nil
	}

	// Parse user ID
	userID, err := uuid.Parse(req.UserId)
	if err != nil {
		return &pb.ActivateUserResponse{
			Success: false,
			Error:   "Invalid user ID format",
		}, nil
	}

	// Update user status
	var user models.User
	if err := s.db.WithContext(ctx).First(&user, "id = ?", userID).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return &pb.ActivateUserResponse{
				Success: false,
				Error:   "User not found",
			}, nil
		}
		s.logger.Error("Failed to find user", zap.Error(err))
		return &pb.ActivateUserResponse{
			Success: false,
			Error:   "Failed to find user",
		}, nil
	}

	// Update user
	user.IsActive = true
	user.UpdatedAt = time.Now()

	if err := s.db.WithContext(ctx).Save(&user).Error; err != nil {
		s.logger.Error("Failed to activate user", zap.Error(err))
		return &pb.ActivateUserResponse{
			Success: false,
			Error:   "Failed to activate user",
		}, nil
	}

	// Invalidate caches
	s.invalidateUserCaches(ctx, userID, user.OrganizationID)

	return &pb.ActivateUserResponse{
		Success: true,
		User:    s.convertUserToProto(&user),
	}, nil
}

// DeactivateUser implements DeactivateUser method for user management
func (s *EnhancedAuthGRPCServer) DeactivateUser(ctx context.Context, req *pb.DeactivateUserRequest) (*pb.DeactivateUserResponse, error) {
	// Validate input
	if req.UserId == "" {
		return &pb.DeactivateUserResponse{
			Success: false,
			Error:   "User ID is required",
		}, nil
	}

	// Parse user ID
	userID, err := uuid.Parse(req.UserId)
	if err != nil {
		return &pb.DeactivateUserResponse{
			Success: false,
			Error:   "Invalid user ID format",
		}, nil
	}

	// Update user status
	var user models.User
	if err := s.db.WithContext(ctx).First(&user, "id = ?", userID).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return &pb.DeactivateUserResponse{
				Success: false,
				Error:   "User not found",
			}, nil
		}
		s.logger.Error("Failed to find user", zap.Error(err))
		return &pb.DeactivateUserResponse{
			Success: false,
			Error:   "Failed to find user",
		}, nil
	}

	// Update user
	user.IsActive = false
	user.UpdatedAt = time.Now()

	if err := s.db.WithContext(ctx).Save(&user).Error; err != nil {
		s.logger.Error("Failed to deactivate user", zap.Error(err))
		return &pb.DeactivateUserResponse{
			Success: false,
			Error:   "Failed to deactivate user",
		}, nil
	}

	// Invalidate caches
	s.invalidateUserCaches(ctx, userID, user.OrganizationID)

	return &pb.DeactivateUserResponse{
		Success: true,
		User:    s.convertUserToProto(&user),
	}, nil
}

// VerifyUser implements VerifyUser method for user management
func (s *EnhancedAuthGRPCServer) VerifyUser(ctx context.Context, req *pb.VerifyUserRequest) (*pb.VerifyUserResponse, error) {
	// Validate input
	if req.UserId == "" {
		return &pb.VerifyUserResponse{
			Success: false,
			Error:   "User ID is required",
		}, nil
	}

	// Parse user ID
	userID, err := uuid.Parse(req.UserId)
	if err != nil {
		return &pb.VerifyUserResponse{
			Success: false,
			Error:   "Invalid user ID format",
		}, nil
	}

	// Update user status
	var user models.User
	if err := s.db.WithContext(ctx).First(&user, "id = ?", userID).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return &pb.VerifyUserResponse{
				Success: false,
				Error:   "User not found",
			}, nil
		}
		s.logger.Error("Failed to find user", zap.Error(err))
		return &pb.VerifyUserResponse{
			Success: false,
			Error:   "Failed to find user",
		}, nil
	}

	// Update user
	user.IsVerified = true
	user.UpdatedAt = time.Now()

	if err := s.db.WithContext(ctx).Save(&user).Error; err != nil {
		s.logger.Error("Failed to verify user", zap.Error(err))
		return &pb.VerifyUserResponse{
			Success: false,
			Error:   "Failed to verify user",
		}, nil
	}

	// Invalidate caches
	s.invalidateUserCaches(ctx, userID, user.OrganizationID)

	return &pb.VerifyUserResponse{
		Success: true,
		User:    s.convertUserToProto(&user),
	}, nil
}

// CreateRole implements CreateRole method for role management
func (s *EnhancedAuthGRPCServer) CreateRole(ctx context.Context, req *pb.CreateRoleRequest) (*pb.CreateRoleResponse, error) {
	// Validate input
	if req.OrganizationId == "" || req.Name == "" {
		return &pb.CreateRoleResponse{
			Success: false,
			Error:   "Organization ID and role name are required",
		}, nil
	}

	// Parse organization ID
	orgID, err := uuid.Parse(req.OrganizationId)
	if err != nil {
		return &pb.CreateRoleResponse{
			Success: false,
			Error:   "Invalid organization ID format",
		}, nil
	}

	// Create role
	role := models.Role{
		ID:             uuid.New(),
		OrganizationID: orgID,
		Name:           req.Name,
		Description:    req.Description,
		IsSystem:       false,
		IsActive:       true,
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}

	if err := s.db.WithContext(ctx).Create(&role).Error; err != nil {
		s.logger.Error("Failed to create role", zap.Error(err))
		return &pb.CreateRoleResponse{
			Success: false,
			Error:   "Failed to create role",
		}, nil
	}

	// Convert to protobuf
	pbRole := &pb.Role{
		Id:             role.ID.String(),
		OrganizationId: role.OrganizationID.String(),
		Name:           role.Name,
		Description:    role.Description,
		IsSystem:       role.IsSystem,
		IsActive:       role.IsActive,
		Permissions:    []*pb.Permission{}, // Empty for new role
		CreatedAt:      timestamppb.New(role.CreatedAt),
		UpdatedAt:      timestamppb.New(role.UpdatedAt),
	}

	return &pb.CreateRoleResponse{
		Success: true,
		Role:    pbRole,
	}, nil
}

// UpdateRole implements UpdateRole method for role management
func (s *EnhancedAuthGRPCServer) UpdateRole(ctx context.Context, req *pb.UpdateRoleRequest) (*pb.UpdateRoleResponse, error) {
	// Validate input
	if req.RoleId == "" {
		return &pb.UpdateRoleResponse{
			Success: false,
			Error:   "Role ID is required",
		}, nil
	}

	// Parse role ID
	roleID, err := uuid.Parse(req.RoleId)
	if err != nil {
		return &pb.UpdateRoleResponse{
			Success: false,
			Error:   "Invalid role ID format",
		}, nil
	}

	// Find and update role
	var role models.Role
	if err := s.db.WithContext(ctx).First(&role, "id = ?", roleID).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return &pb.UpdateRoleResponse{
				Success: false,
				Error:   "Role not found",
			}, nil
		}
		s.logger.Error("Failed to find role", zap.Error(err))
		return &pb.UpdateRoleResponse{
			Success: false,
			Error:   "Failed to find role",
		}, nil
	}

	// Update fields
	if req.Name != "" {
		role.Name = req.Name
	}
	if req.Description != "" {
		role.Description = req.Description
	}
	role.UpdatedAt = time.Now()

	if err := s.db.WithContext(ctx).Save(&role).Error; err != nil {
		s.logger.Error("Failed to update role", zap.Error(err))
		return &pb.UpdateRoleResponse{
			Success: false,
			Error:   "Failed to update role",
		}, nil
	}

	// Convert to protobuf
	pbRole := &pb.Role{
		Id:             role.ID.String(),
		OrganizationId: role.OrganizationID.String(),
		Name:           role.Name,
		Description:    role.Description,
		IsSystem:       role.IsSystem,
		IsActive:       role.IsActive,
		Permissions:    []*pb.Permission{}, // Would need to load permissions
		CreatedAt:      timestamppb.New(role.CreatedAt),
		UpdatedAt:      timestamppb.New(role.UpdatedAt),
	}

	return &pb.UpdateRoleResponse{
		Success: true,
		Role:    pbRole,
	}, nil
}

// DeleteRole implements DeleteRole method for role management
func (s *EnhancedAuthGRPCServer) DeleteRole(ctx context.Context, req *pb.DeleteRoleRequest) (*pb.DeleteRoleResponse, error) {
	// Validate input
	if req.RoleId == "" {
		return &pb.DeleteRoleResponse{
			Success: false,
			Error:   "Role ID is required",
		}, nil
	}

	// Parse role ID
	roleID, err := uuid.Parse(req.RoleId)
	if err != nil {
		return &pb.DeleteRoleResponse{
			Success: false,
			Error:   "Invalid role ID format",
		}, nil
	}

	// Check if role exists and is not system role
	var role models.Role
	if err := s.db.WithContext(ctx).First(&role, "id = ?", roleID).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return &pb.DeleteRoleResponse{
				Success: false,
				Error:   "Role not found",
			}, nil
		}
		s.logger.Error("Failed to find role", zap.Error(err))
		return &pb.DeleteRoleResponse{
			Success: false,
			Error:   "Failed to find role",
		}, nil
	}

	if role.IsSystem {
		return &pb.DeleteRoleResponse{
			Success: false,
			Error:   "Cannot delete system role",
		}, nil
	}

	// Delete role (soft delete)
	if err := s.db.WithContext(ctx).Delete(&role).Error; err != nil {
		s.logger.Error("Failed to delete role", zap.Error(err))
		return &pb.DeleteRoleResponse{
			Success: false,
			Error:   "Failed to delete role",
		}, nil
	}

	return &pb.DeleteRoleResponse{
		Success: true,
	}, nil
}

// AssignUserRole implements AssignUserRole method for role assignment
func (s *EnhancedAuthGRPCServer) AssignUserRole(ctx context.Context, req *pb.AssignUserRoleRequest) (*pb.AssignUserRoleResponse, error) {
	// Validate input
	if req.UserId == "" || req.RoleId == "" {
		return &pb.AssignUserRoleResponse{
			Success: false,
			Error:   "User ID and Role ID are required",
		}, nil
	}

	// Parse IDs
	userID, err := uuid.Parse(req.UserId)
	if err != nil {
		return &pb.AssignUserRoleResponse{
			Success: false,
			Error:   "Invalid user ID format",
		}, nil
	}

	roleID, err := uuid.Parse(req.RoleId)
	if err != nil {
		return &pb.AssignUserRoleResponse{
			Success: false,
			Error:   "Invalid role ID format",
		}, nil
	}

	// Check if assignment already exists
	var existingAssignment models.UserRole
	if err := s.db.WithContext(ctx).Where("user_id = ? AND role_id = ?", userID, roleID).First(&existingAssignment).Error; err == nil {
		return &pb.AssignUserRoleResponse{
			Success: false,
			Error:   "User already has this role",
		}, nil
	}

	// Create user role assignment
	userRole := models.UserRole{
		ID:        uuid.New(),
		UserID:    userID,
		RoleID:    roleID,
		CreatedAt: time.Now(),
	}

	if err := s.db.WithContext(ctx).Create(&userRole).Error; err != nil {
		s.logger.Error("Failed to assign user role", zap.Error(err))
		return &pb.AssignUserRoleResponse{
			Success: false,
			Error:   "Failed to assign role to user",
		}, nil
	}

	return &pb.AssignUserRoleResponse{
		Success: true,
	}, nil
}

// RevokeUserRole implements RevokeUserRole method for role revocation
func (s *EnhancedAuthGRPCServer) RevokeUserRole(ctx context.Context, req *pb.RevokeUserRoleRequest) (*pb.RevokeUserRoleResponse, error) {
	// Validate input
	if req.UserId == "" || req.RoleId == "" {
		return &pb.RevokeUserRoleResponse{
			Success: false,
			Error:   "User ID and Role ID are required",
		}, nil
	}

	// Parse IDs
	userID, err := uuid.Parse(req.UserId)
	if err != nil {
		return &pb.RevokeUserRoleResponse{
			Success: false,
			Error:   "Invalid user ID format",
		}, nil
	}

	roleID, err := uuid.Parse(req.RoleId)
	if err != nil {
		return &pb.RevokeUserRoleResponse{
			Success: false,
			Error:   "Invalid role ID format",
		}, nil
	}

	// Delete user role assignment
	if err := s.db.WithContext(ctx).Where("user_id = ? AND role_id = ?", userID, roleID).Delete(&models.UserRole{}).Error; err != nil {
		s.logger.Error("Failed to revoke user role", zap.Error(err))
		return &pb.RevokeUserRoleResponse{
			Success: false,
			Error:   "Failed to revoke role from user",
		}, nil
	}

	return &pb.RevokeUserRoleResponse{
		Success: true,
	}, nil
}

// AssignPermissions implements AssignPermissions method for permission assignment
func (s *EnhancedAuthGRPCServer) AssignPermissions(ctx context.Context, req *pb.AssignPermissionsRequest) (*pb.AssignPermissionsResponse, error) {
	// Validate input
	if req.RoleId == "" || len(req.PermissionIds) == 0 {
		return &pb.AssignPermissionsResponse{
			Success: false,
			Error:   "Role ID and permission IDs are required",
		}, nil
	}

	// Parse role ID
	roleID, err := uuid.Parse(req.RoleId)
	if err != nil {
		return &pb.AssignPermissionsResponse{
			Success: false,
			Error:   "Invalid role ID format",
		}, nil
	}

	// Parse permission IDs
	permissionIDs := make([]uuid.UUID, len(req.PermissionIds))
	for i, permID := range req.PermissionIds {
		parsedID, err := uuid.Parse(permID)
		if err != nil {
			return &pb.AssignPermissionsResponse{
				Success: false,
				Error:   fmt.Sprintf("Invalid permission ID format: %s", permID),
			}, nil
		}
		permissionIDs[i] = parsedID
	}

	// Remove existing permissions for this role
	if err := s.db.WithContext(ctx).Where("role_id = ?", roleID).Delete(&models.RolePermission{}).Error; err != nil {
		s.logger.Error("Failed to remove existing permissions", zap.Error(err))
		return &pb.AssignPermissionsResponse{
			Success: false,
			Error:   "Failed to update role permissions",
		}, nil
	}

	// Add new permissions
	for _, permID := range permissionIDs {
		rolePermission := models.RolePermission{
			ID:           uuid.New(),
			RoleID:       roleID,
			PermissionID: permID,
			CreatedAt:    time.Now(),
		}

		if err := s.db.WithContext(ctx).Create(&rolePermission).Error; err != nil {
			s.logger.Error("Failed to assign permission", zap.Error(err))
			return &pb.AssignPermissionsResponse{
				Success: false,
				Error:   "Failed to assign permissions to role",
			}, nil
		}
	}

	return &pb.AssignPermissionsResponse{
		Success: true,
	}, nil
}

// BulkDeleteUsers implements BulkDeleteUsers method for bulk operations
func (s *EnhancedAuthGRPCServer) BulkDeleteUsers(ctx context.Context, req *pb.BulkDeleteUsersRequest) (*pb.BulkDeleteUsersResponse, error) {
	if len(req.UserIds) == 0 {
		return &pb.BulkDeleteUsersResponse{
			Success: false,
			Error:   "User IDs list is required",
		}, nil
	}

	results := make([]*pb.BulkDeleteResult, len(req.UserIds))
	deletedCount := int32(0)
	failedCount := int32(0)

	for i, userIDStr := range req.UserIds {
		userID, err := uuid.Parse(userIDStr)
		if err != nil {
			results[i] = &pb.BulkDeleteResult{
				Success: false,
				UserId:  userIDStr,
				Error:   "Invalid user ID format",
			}
			failedCount++
			continue
		}

		// Delete user
		if err := s.db.WithContext(ctx).Delete(&models.User{}, "id = ?", userID).Error; err != nil {
			results[i] = &pb.BulkDeleteResult{
				Success: false,
				UserId:  userIDStr,
				Error:   "Failed to delete user",
			}
			failedCount++
		} else {
			results[i] = &pb.BulkDeleteResult{
				Success: true,
				UserId:  userIDStr,
			}
			deletedCount++
		}
	}

	return &pb.BulkDeleteUsersResponse{
		Success:      deletedCount > 0,
		DeletedCount: deletedCount,
		FailedCount:  failedCount,
		Results:      results,
	}, nil
}
