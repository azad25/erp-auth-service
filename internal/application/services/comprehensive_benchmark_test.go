package services

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/mock"
	"go.uber.org/zap"

	"erp-auth-service/internal/cache"
	"erp-auth-service/internal/config"
	"erp-auth-service/internal/domain/entities"
	"erp-auth-service/internal/models"
)

// BenchmarkAuthService_Authenticate benchmarks the authentication process
func BenchmarkAuthService_Authenticate(b *testing.B) {
	// Setup
	mockUserRepo := new(MockUserRepository)
	mockOrgRepo := new(MockOrganizationRepository)
	mockRoleRepo := new(MockRoleRepository)
	mockTokenSvc := new(MockTokenService)
	mockEventPub := new(MockEventPublisher)
	mockCache := NewMockCacheManager()
	
	config := &config.Config{
		JWT: config.JWTConfig{
			Secret:        "benchmark-test-secret",
			AccessExpiry:  3600,
			RefreshExpiry: 604800,
		},
	}
	
	logger := zap.NewNop()
	
	// Setup mock expectations
	mockCache.On("Get", mock.Anything, mock.AnythingOfType("string")).Return(nil, cache.ErrCacheNotFound).Maybe()
	mockCache.On("Set", mock.Anything, mock.AnythingOfType("string"), mock.Anything, mock.AnythingOfType("time.Duration")).Return(nil).Maybe()
	mockCache.On("Exists", mock.Anything, mock.AnythingOfType("string")).Return(false, nil).Maybe()
	
	authService := NewAuthService(
		config,
		logger,
		mockUserRepo,
		mockOrgRepo,
		mockRoleRepo,
		mockTokenSvc,
		mockEventPub,
		mockCache,
	)
	defer authService.Close()
	
	// Test data
	userID := uuid.New()
	orgID := uuid.New()
	email := "benchmark@example.com"
	
	user := &models.User{
		ID:             userID,
		OrganizationID: orgID,
		Email:          email,
		PasswordHash:   "$2a$10$N9qo8uLOickgx2ZMRZoMye.IjPeGvGzjYwSgjkMIUmEWqbBtxiupe", // "password123"
		FirstName:      "Benchmark",
		LastName:       "User",
		IsActive:       true,
		IsVerified:     true,
		TwoFactorEnabled: false,
		FailedLoginAttempts: 0,
		LockedUntil:    nil,
	}
	
	tokenPair := &entities.TokenPair{
		AccessToken:  "benchmark-access-token",
		RefreshToken: "benchmark-refresh-token",
		ExpiresAt:    time.Now().Add(time.Hour),
		TokenType:    "Bearer",
	}
	
	securityCtx := &SecurityContext{
		IPAddress: "192.168.1.1",
		UserAgent: "benchmark-agent",
	}
	
	req := &AuthRequest{
		Email:           email,
		Password:        "password123",
		SecurityContext: securityCtx,
	}
	
	// Mock setup
	mockUserRepo.On("GetByEmailWithOrganization", mock.Anything, email).Return(user, nil)
	mockUserRepo.On("ResetFailedLoginAttempts", mock.Anything, userID).Return(nil)
	mockUserRepo.On("UpdateLastLogin", mock.Anything, userID, mock.AnythingOfType("time.Time")).Return(nil)
	mockTokenSvc.On("GenerateTokenPair", mock.Anything, userID, orgID, email, securityCtx).Return(tokenPair, nil)
	mockEventPub.On("PublishUserLoggedIn", mock.Anything, mock.Anything, userID, orgID, securityCtx.IPAddress, securityCtx.UserAgent).Return(nil)
	
	ctx := context.Background()
	
	// Benchmark
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_, err := authService.Authenticate(ctx, req)
			if err != nil {
				b.Fatal(err)
			}
		}
	})
}

// BenchmarkAuthService_PasswordHashing benchmarks password hashing
func BenchmarkAuthService_PasswordHashing(b *testing.B) {
	authService := &AuthService{}
	password := "benchmark-password-123"
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := authService.hashPassword(password)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkAuthService_PasswordVerification benchmarks password verification
func BenchmarkAuthService_PasswordVerification(b *testing.B) {
	authService := &AuthService{}
	password := "benchmark-password-123"
	hash := "$2a$10$N9qo8uLOickgx2ZMRZoMye.IjPeGvGzjYwSgjkMIUmEWqbBtxiupe"
	
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			authService.verifyPassword(password, hash)
		}
	})
}

// BenchmarkTokenService_GenerateTokenPair benchmarks token pair generation
func BenchmarkTokenService_GenerateTokenPair(b *testing.B) {
	// Setup mini redis
	miniRedis, err := miniredis.Run()
	if err != nil {
		b.Fatal(err)
	}
	defer miniRedis.Close()
	
	redisClient := redis.NewClient(&redis.Options{
		Addr: miniRedis.Addr(),
	})
	defer redisClient.Close()
	
	mockRepo := &MockTokenRepository{}
	mockCache := NewMockCacheManager()
	
	config := &config.Config{
		JWT: config.JWTConfig{
			Secret:        "benchmark-token-secret",
			AccessExpiry:  3600,
			RefreshExpiry: 604800,
		},
	}
	
	logger := zap.NewNop()
	
	// Setup mock expectations
	mockCache.On("Set", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil).Maybe()
	mockRepo.On("Create", mock.Anything, mock.AnythingOfType("*models.Token")).Return(nil).Maybe()
	
	tokenService := NewTokenService(config, logger, mockRepo, mockCache, redisClient)
	
	// Test data
	userID := uuid.New()
	organizationID := uuid.New()
	email := "benchmark@example.com"
	securityCtx := &SecurityContext{
		IPAddress:         "192.168.1.1",
		UserAgent:         "benchmark-agent",
		DeviceFingerprint: "benchmark-fingerprint",
	}
	
	ctx := context.Background()
	
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_, err := tokenService.GenerateTokenPair(ctx, userID, organizationID, email, securityCtx)
			if err != nil {
				b.Fatal(err)
			}
		}
	})
}

// BenchmarkTokenService_ValidateToken benchmarks token validation
func BenchmarkTokenService_ValidateToken(b *testing.B) {
	// Setup mini redis
	miniRedis, err := miniredis.Run()
	if err != nil {
		b.Fatal(err)
	}
	defer miniRedis.Close()
	
	redisClient := redis.NewClient(&redis.Options{
		Addr: miniRedis.Addr(),
	})
	defer redisClient.Close()
	
	mockRepo := &MockTokenRepository{}
	mockCache := NewMockCacheManager()
	
	config := &config.Config{
		JWT: config.JWTConfig{
			Secret:        "benchmark-token-secret",
			AccessExpiry:  3600,
			RefreshExpiry: 604800,
		},
	}
	
	logger := zap.NewNop()
	
	// Setup mock expectations
	mockCache.On("Get", mock.Anything, mock.Anything).Return(nil, cache.ErrCacheNotFound).Maybe()
	mockCache.On("Set", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil).Maybe()
	mockRepo.On("Create", mock.Anything, mock.AnythingOfType("*models.Token")).Return(nil).Maybe()
	
	tokenService := NewTokenService(config, logger, mockRepo, mockCache, redisClient)
	
	// Generate a token for validation
	userID := uuid.New()
	organizationID := uuid.New()
	email := "benchmark@example.com"
	ctx := context.Background()
	
	tokenPair, err := tokenService.GenerateTokenPair(ctx, userID, organizationID, email, nil)
	if err != nil {
		b.Fatal(err)
	}
	
	// Mock token lookup for validation
	tokenHash := tokenService.hashToken(tokenPair.AccessToken)
	mockToken := &models.Token{
		ID:        uuid.New(),
		UserID:    userID,
		TokenType: models.AccessToken,
		TokenHash: tokenHash,
		IssuedAt:  time.Now(),
		ExpiresAt: time.Now().Add(time.Hour),
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	mockRepo.On("GetByTokenHash", mock.Anything, tokenHash).Return(mockToken, nil).Maybe()
	
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_, err := tokenService.ValidateToken(ctx, tokenPair.AccessToken)
			if err != nil {
				b.Fatal(err)
			}
		}
	})
}

// BenchmarkTokenService_HashToken benchmarks token hashing
func BenchmarkTokenService_HashToken(b *testing.B) {
	tokenService := &TokenService{}
	token := "benchmark.token.string.with.multiple.parts.for.testing.performance"
	
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			tokenService.hashToken(token)
		}
	})
}

// BenchmarkPermissionService_CheckUserPermission benchmarks permission checking
func BenchmarkPermissionService_CheckUserPermission(b *testing.B) {
	mockPermRepo := new(MockPermissionRepository)
	mockRoleRepo := new(MockRoleRepository)
	mockUserRepo := new(MockUserRepository)
	mockCache := NewMockCacheManager()
	
	config := PermissionServiceConfig{
		CacheTTL:              5 * time.Minute,
		BulkEvaluationEnabled: true,
		CacheWarmingEnabled:   false,
		MaxCacheSize:          1000,
		EvaluationTimeout:     10 * time.Second,
	}
	
	logger := zap.NewNop()
	
	// Setup mock expectations
	mockCache.On("Get", mock.Anything, mock.AnythingOfType("string")).Return(nil, cache.ErrCacheNotFound).Maybe()
	mockCache.On("Set", mock.Anything, mock.AnythingOfType("string"), mock.Anything, mock.AnythingOfType("time.Duration")).Return(nil).Maybe()
	
	permissionService := NewPermissionService(
		mockPermRepo,
		mockRoleRepo,
		mockUserRepo,
		mockCache,
		logger,
		config,
	)
	defer permissionService.Close()
	
	// Test data
	userID := uuid.New()
	resource := "users"
	action := "read"
	
	permissions := []models.Permission{
		{
			ID:       uuid.New(),
			Name:     "users.read",
			Resource: "users",
			Action:   "read",
		},
		{
			ID:       uuid.New(),
			Name:     "users.write",
			Resource: "users",
			Action:   "write",
		},
		{
			ID:       uuid.New(),
			Name:     "documents.read",
			Resource: "documents",
			Action:   "read",
		},
	}
	
	mockPermRepo.On("GetUserPermissions", mock.Anything, userID).Return(permissions, nil).Maybe()
	
	ctx := context.Background()
	
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_, err := permissionService.CheckUserPermission(ctx, userID, resource, action)
			if err != nil {
				b.Fatal(err)
			}
		}
	})
}

// BenchmarkPermissionService_CheckBulkPermissions benchmarks bulk permission checking
func BenchmarkPermissionService_CheckBulkPermissions(b *testing.B) {
	mockPermRepo := new(MockPermissionRepository)
	mockRoleRepo := new(MockRoleRepository)
	mockUserRepo := new(MockUserRepository)
	mockCache := NewMockCacheManager()
	
	config := PermissionServiceConfig{
		CacheTTL:              5 * time.Minute,
		BulkEvaluationEnabled: true,
		CacheWarmingEnabled:   false,
		MaxCacheSize:          1000,
		EvaluationTimeout:     10 * time.Second,
	}
	
	logger := zap.NewNop()
	
	// Setup mock expectations
	mockCache.On("Get", mock.Anything, mock.AnythingOfType("string")).Return(nil, cache.ErrCacheNotFound).Maybe()
	mockCache.On("Set", mock.Anything, mock.AnythingOfType("string"), mock.Anything, mock.AnythingOfType("time.Duration")).Return(nil).Maybe()
	
	permissionService := NewPermissionService(
		mockPermRepo,
		mockRoleRepo,
		mockUserRepo,
		mockCache,
		logger,
		config,
	)
	defer permissionService.Close()
	
	// Test data
	userID := uuid.New()
	request := &BulkPermissionRequest{
		UserID: userID,
		Permissions: []PermissionCheckRequest{
			{Resource: "users", Action: "read"},
			{Resource: "users", Action: "write"},
			{Resource: "documents", Action: "read"},
			{Resource: "documents", Action: "write"},
			{Resource: "reports", Action: "read"},
		},
	}
	
	permissions := []models.Permission{
		{
			ID:       uuid.New(),
			Name:     "users.read",
			Resource: "users",
			Action:   "read",
		},
		{
			ID:       uuid.New(),
			Name:     "documents.read",
			Resource: "documents",
			Action:   "read",
		},
	}
	
	mockPermRepo.On("GetUserPermissions", mock.Anything, userID).Return(permissions, nil).Maybe()
	
	ctx := context.Background()
	
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_, err := permissionService.CheckBulkPermissions(ctx, request)
			if err != nil {
				b.Fatal(err)
			}
		}
	})
}

// BenchmarkCacheOperations benchmarks cache operations
func BenchmarkCacheOperations(b *testing.B) {
	mockCache := NewMockCacheManager()
	
	// Setup mock expectations
	mockCache.On("Get", mock.Anything, mock.AnythingOfType("string")).Return([]byte("cached-value"), nil).Maybe()
	mockCache.On("Set", mock.Anything, mock.AnythingOfType("string"), mock.Anything, mock.AnythingOfType("time.Duration")).Return(nil).Maybe()
	mockCache.On("Delete", mock.Anything, mock.AnythingOfType("string")).Return(nil).Maybe()
	
	ctx := context.Background()
	key := "benchmark-cache-key"
	value := []byte("benchmark-cache-value")
	ttl := 5 * time.Minute
	
	b.Run("CacheGet", func(b *testing.B) {
		b.RunParallel(func(pb *testing.PB) {
			for pb.Next() {
				_, err := mockCache.Get(ctx, key)
				if err != nil && err != cache.ErrCacheNotFound {
					b.Fatal(err)
				}
			}
		})
	})
	
	b.Run("CacheSet", func(b *testing.B) {
		b.RunParallel(func(pb *testing.PB) {
			for pb.Next() {
				err := mockCache.Set(ctx, key, value, ttl)
				if err != nil {
					b.Fatal(err)
				}
			}
		})
	})
	
	b.Run("CacheDelete", func(b *testing.B) {
		b.RunParallel(func(pb *testing.PB) {
			for pb.Next() {
				err := mockCache.Delete(ctx, key)
				if err != nil {
					b.Fatal(err)
				}
			}
		})
	})
}

// BenchmarkConcurrentAuthentication benchmarks concurrent authentication requests
func BenchmarkConcurrentAuthentication(b *testing.B) {
	// Setup
	mockUserRepo := new(MockUserRepository)
	mockOrgRepo := new(MockOrganizationRepository)
	mockRoleRepo := new(MockRoleRepository)
	mockTokenSvc := new(MockTokenService)
	mockEventPub := new(MockEventPublisher)
	mockCache := NewMockCacheManager()
	
	config := &config.Config{
		JWT: config.JWTConfig{
			Secret:        "concurrent-benchmark-secret",
			AccessExpiry:  3600,
			RefreshExpiry: 604800,
		},
	}
	
	logger := zap.NewNop()
	
	// Setup mock expectations
	mockCache.On("Get", mock.Anything, mock.AnythingOfType("string")).Return(nil, cache.ErrCacheNotFound).Maybe()
	mockCache.On("Set", mock.Anything, mock.AnythingOfType("string"), mock.Anything, mock.AnythingOfType("time.Duration")).Return(nil).Maybe()
	mockCache.On("Exists", mock.Anything, mock.AnythingOfType("string")).Return(false, nil).Maybe()
	
	authService := NewAuthService(
		config,
		logger,
		mockUserRepo,
		mockOrgRepo,
		mockRoleRepo,
		mockTokenSvc,
		mockEventPub,
		mockCache,
	)
	defer authService.Close()
	
	// Test data for multiple users
	users := make([]*models.User, 10)
	requests := make([]*AuthRequest, 10)
	tokenPairs := make([]*entities.TokenPair, 10)
	
	for i := 0; i < 10; i++ {
		userID := uuid.New()
		orgID := uuid.New()
		email := fmt.Sprintf("user%d@example.com", i)
		
		users[i] = &models.User{
			ID:             userID,
			OrganizationID: orgID,
			Email:          email,
			PasswordHash:   "$2a$10$N9qo8uLOickgx2ZMRZoMye.IjPeGvGzjYwSgjkMIUmEWqbBtxiupe",
			FirstName:      fmt.Sprintf("User%d", i),
			LastName:       "Benchmark",
			IsActive:       true,
			IsVerified:     true,
			TwoFactorEnabled: false,
			FailedLoginAttempts: 0,
			LockedUntil:    nil,
		}
		
		requests[i] = &AuthRequest{
			Email:    email,
			Password: "password123",
			SecurityContext: &SecurityContext{
				IPAddress: fmt.Sprintf("192.168.1.%d", i+1),
				UserAgent: "benchmark-agent",
			},
		}
		
		tokenPairs[i] = &entities.TokenPair{
			AccessToken:  fmt.Sprintf("access-token-%d", i),
			RefreshToken: fmt.Sprintf("refresh-token-%d", i),
			ExpiresAt:    time.Now().Add(time.Hour),
			TokenType:    "Bearer",
		}
		
		// Setup mocks for each user
		mockUserRepo.On("GetByEmailWithOrganization", mock.Anything, email).Return(users[i], nil).Maybe()
		mockUserRepo.On("ResetFailedLoginAttempts", mock.Anything, userID).Return(nil).Maybe()
		mockUserRepo.On("UpdateLastLogin", mock.Anything, userID, mock.AnythingOfType("time.Time")).Return(nil).Maybe()
		mockTokenSvc.On("GenerateTokenPair", mock.Anything, userID, orgID, email, requests[i].SecurityContext).Return(tokenPairs[i], nil).Maybe()
		mockEventPub.On("PublishUserLoggedIn", mock.Anything, mock.Anything, userID, orgID, requests[i].SecurityContext.IPAddress, requests[i].SecurityContext.UserAgent).Return(nil).Maybe()
	}
	
	ctx := context.Background()
	
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			req := requests[i%len(requests)]
			_, err := authService.Authenticate(ctx, req)
			if err != nil {
				b.Fatal(err)
			}
			i++
		}
	})
}

// BenchmarkMemoryAllocation benchmarks memory allocation patterns
func BenchmarkMemoryAllocation(b *testing.B) {
	b.Run("AuthRequest", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			req := &AuthRequest{
				Email:    "test@example.com",
				Password: "password123",
				SecurityContext: &SecurityContext{
					IPAddress: "192.168.1.1",
					UserAgent: "test-agent",
				},
			}
			_ = req
		}
	})
	
	b.Run("TokenClaims", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			claims := &TokenClaims{
				UserID:         uuid.New(),
				OrganizationID: uuid.New(),
				Email:          "test@example.com",
				TokenType:      "access",
			}
			_ = claims
		}
	})
	
	b.Run("PermissionRequest", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			req := &BulkPermissionRequest{
				UserID: uuid.New(),
				Permissions: []PermissionCheckRequest{
					{Resource: "users", Action: "read"},
					{Resource: "documents", Action: "write"},
				},
			}
			_ = req
		}
	})
}

// BenchmarkStringOperations benchmarks string operations used in security functions
func BenchmarkStringOperations(b *testing.B) {
	b.Run("EmailValidation", func(b *testing.B) {
		email := "test@example.com"
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_ = len(email) > 0 && strings.Contains(email, "@")
		}
	})
	
	b.Run("PasswordLengthCheck", func(b *testing.B) {
		password := "password123"
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_ = len(password) >= 8
		}
	})
	
	b.Run("TokenFormatValidation", func(b *testing.B) {
		token := "header.payload.signature"
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			parts := strings.Split(token, ".")
			_ = len(parts) == 3
		}
	})
}

// BenchmarkUUIDOperations benchmarks UUID operations
func BenchmarkUUIDOperations(b *testing.B) {
	b.Run("UUIDGeneration", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			_ = uuid.New()
		}
	})
	
	b.Run("UUIDString", func(b *testing.B) {
		id := uuid.New()
		b.ResetTimer()
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			_ = id.String()
		}
	})
	
	b.Run("UUIDParsing", func(b *testing.B) {
		idStr := uuid.New().String()
		b.ResetTimer()
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			_, err := uuid.Parse(idStr)
			if err != nil {
				b.Fatal(err)
			}
		}
	})
}