package services

import (
	"context"
	"crypto/rand"
	"encoding/base32"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/pquerna/otp/totp"
	"go.uber.org/zap"
	"golang.org/x/crypto/bcrypt"

	"erp-auth-service/internal/cache"
	"erp-auth-service/internal/config"
	"erp-auth-service/internal/domain/entities"
	"erp-auth-service/internal/domain/interfaces"
	"erp-auth-service/internal/events"
	"erp-auth-service/internal/models"
)

// AuthService handles authentication operations with concurrency optimization
type AuthService struct {
	config         *config.Config
	logger         *zap.Logger
	userRepo       interfaces.UserRepository
	orgRepo        interfaces.OrganizationRepository
	roleRepo       interfaces.RoleRepository
	tokenService   TokenServiceInterface
	eventPublisher EventPublisherInterface
	cacheManager   cache.CacheManager
	rateLimiter    *RateLimiter
	
	// Specialized worker pools for CPU-intensive operations
	workerPoolManager  *SpecializedWorkerPoolManager
	
	// Metrics and monitoring
	metrics *AuthMetrics
	
	// Concurrency control
	mu sync.RWMutex
}

// AuthMetrics tracks authentication service performance
type AuthMetrics struct {
	AuthenticationAttempts int64
	SuccessfulLogins      int64
	FailedLogins          int64
	RegistrationAttempts  int64
	SuccessfulRegistrations int64
	PasswordChanges       int64
	TwoFactorEnabled      int64
	BruteForceBlocked     int64
	RateLimitBlocked      int64
	mu                    sync.RWMutex
}

// AuthRequest represents an authentication request with security context
type AuthRequest struct {
	Email             string
	Password          string
	OrganizationDomain string
	TwoFactorCode     string
	SecurityContext   *SecurityContext
	RememberMe        bool
}

// AuthResponse represents an authentication response
type AuthResponse struct {
	Success      bool                `json:"success"`
	TokenPair    *entities.TokenPair `json:"tokens,omitempty"`
	User         *entities.User      `json:"user,omitempty"`
	RequiresTwoFA bool               `json:"requires_2fa"`
	Error        string             `json:"error,omitempty"`
	Warnings     []string           `json:"warnings,omitempty"`
}

// RegistrationRequest represents a user registration request
type RegistrationRequest struct {
	Email            string
	Password         string
	FirstName        string
	LastName         string
	OrganizationName string
	OrganizationDomain string
	SecurityContext  *SecurityContext
}

// RegistrationResponse represents a registration response
type RegistrationResponse struct {
	Success   bool                `json:"success"`
	TokenPair *entities.TokenPair `json:"tokens,omitempty"`
	User      *entities.User      `json:"user,omitempty"`
	Error     string             `json:"error,omitempty"`
}

// PasswordChangeRequest represents a password change request
type PasswordChangeRequest struct {
	UserID          uuid.UUID
	CurrentPassword string
	NewPassword     string
	SecurityContext *SecurityContext
}

// TwoFactorSetupRequest represents a 2FA setup request
type TwoFactorSetupRequest struct {
	UserID uuid.UUID
	Enable bool
}

// TwoFactorSetupResponse represents a 2FA setup response
type TwoFactorSetupResponse struct {
	Success     bool     `json:"success"`
	Secret      string   `json:"secret,omitempty"`
	QRCodeURL   string   `json:"qr_code_url,omitempty"`
	BackupCodes []string `json:"backup_codes,omitempty"`
	Error       string   `json:"error,omitempty"`
}

// TokenServiceInterface defines the interface for token operations
type TokenServiceInterface interface {
	GenerateTokenPair(ctx context.Context, userID, organizationID uuid.UUID, email string, securityCtx *SecurityContext) (*entities.TokenPair, error)
	RevokeAllUserTokens(ctx context.Context, userID uuid.UUID, tokenType entities.TokenType) error
}

// EventPublisherInterface defines the interface for event publishing
type EventPublisherInterface interface {
	PublishUserLoggedIn(ctx context.Context, data events.UserLoggedInData, userID, orgID uuid.UUID, ipAddress, userAgent string) error
	PublishUserRegistered(ctx context.Context, data events.UserRegisteredData, userID, orgID uuid.UUID, ipAddress, userAgent string) error
	PublishOrganizationCreated(ctx context.Context, data events.OrganizationCreatedData, userID, orgID uuid.UUID, ipAddress, userAgent string) error
	PublishPasswordChanged(ctx context.Context, data events.PasswordChangedData, userID, orgID uuid.UUID, ipAddress, userAgent string) error
}

// NewAuthService creates a new authentication service with optimizations
func NewAuthService(
	config *config.Config,
	logger *zap.Logger,
	userRepo interfaces.UserRepository,
	orgRepo interfaces.OrganizationRepository,
	roleRepo interfaces.RoleRepository,
	tokenService TokenServiceInterface,
	eventPublisher EventPublisherInterface,
	cacheManager cache.CacheManager,
) *AuthService {
	service := &AuthService{
		config:         config,
		logger:         logger.With(zap.String("service", "auth")),
		userRepo:       userRepo,
		orgRepo:        orgRepo,
		roleRepo:       roleRepo,
		tokenService:   tokenService,
		eventPublisher: eventPublisher,
		cacheManager:   cacheManager,
		metrics:        &AuthMetrics{},
	}
	
	// Initialize rate limiter
	service.rateLimiter = NewRateLimiter(config, logger, cacheManager)
	
	// Initialize specialized worker pool manager with cache adapter
	cacheAdapter := NewCacheAdapter(cacheManager)
	service.workerPoolManager = NewSpecializedWorkerPoolManager(
		logger,
		cacheAdapter,
		nil, // Permission service will be set later if needed
		eventPublisher,
	)
	
	// Start specialized worker pools
	service.workerPoolManager.StartSpecializedPools()
	
	return service
}

// Authenticate performs user authentication with security features
func (s *AuthService) Authenticate(ctx context.Context, req *AuthRequest) (*AuthResponse, error) {
	s.updateMetrics(func(m *AuthMetrics) {
		m.AuthenticationAttempts++
	})
	
	// Validate input
	if err := s.validateAuthRequest(req); err != nil {
		return &AuthResponse{
			Success: false,
			Error:   fmt.Sprintf("invalid request: %v", err),
		}, nil
	}
	
	// Check rate limiting
	if blocked, err := s.rateLimiter.IsBlocked(ctx, req.Email, req.SecurityContext.IPAddress); err != nil {
		s.logger.Error("Rate limiter check failed", zap.Error(err))
	} else if blocked {
		s.updateMetrics(func(m *AuthMetrics) {
			m.RateLimitBlocked++
		})
		return &AuthResponse{
			Success: false,
			Error:   "too many authentication attempts, please try again later",
		}, nil
	}
	
	// Get user by email
	user, err := s.getUserByEmail(ctx, req.Email)
	if err != nil {
		s.logger.Debug("User not found", zap.String("email", req.Email))
		s.recordFailedAttempt(ctx, req.Email, req.SecurityContext.IPAddress)
		s.updateMetrics(func(m *AuthMetrics) {
			m.FailedLogins++
		})
		return &AuthResponse{
			Success: false,
			Error:   "invalid credentials",
		}, nil
	}
	
	// Check if user account is locked
	if user.IsLocked() {
		s.updateMetrics(func(m *AuthMetrics) {
			m.BruteForceBlocked++
		})
		return &AuthResponse{
			Success: false,
			Error:   "account is temporarily locked due to multiple failed login attempts",
		}, nil
	}
	
	// Verify password using specialized password worker pool
	if !s.verifyPassword(req.Password, user.PasswordHash) {
		s.handleFailedLogin(ctx, user, req.SecurityContext.IPAddress)
		s.updateMetrics(func(m *AuthMetrics) {
			m.FailedLogins++
		})
		return &AuthResponse{
			Success: false,
			Error:   "invalid credentials",
		}, nil
	}
	
	// Check if 2FA is required
	if user.TwoFactorEnabled {
		if req.TwoFactorCode == "" {
			return &AuthResponse{
				Success:       false,
				RequiresTwoFA: true,
				Error:        "two-factor authentication code required",
			}, nil
		}
		
		// Verify 2FA code
		if !s.verifyTwoFactorCode(user.TwoFactorSecret, req.TwoFactorCode, user.BackupCodes) {
			s.handleFailedLogin(ctx, user, req.SecurityContext.IPAddress)
			s.updateMetrics(func(m *AuthMetrics) {
				m.FailedLogins++
			})
			return &AuthResponse{
				Success: false,
				Error:   "invalid two-factor authentication code",
			}, nil
		}
	}
	
	// Authentication successful - reset failed attempts
	if err := s.userRepo.ResetFailedLoginAttempts(ctx, user.ID); err != nil {
		s.logger.Warn("Failed to reset failed login attempts", zap.Error(err))
	}
	
	// Update last login
	if err := s.userRepo.UpdateLastLogin(ctx, user.ID, time.Now()); err != nil {
		s.logger.Warn("Failed to update last login", zap.Error(err))
	}
	
	// Generate token pair
	tokenPair, err := s.tokenService.GenerateTokenPair(
		ctx,
		user.ID,
		user.OrganizationID,
		user.Email,
		req.SecurityContext,
	)
	if err != nil {
		s.logger.Error("Failed to generate token pair", zap.Error(err))
		return &AuthResponse{
			Success: false,
			Error:   "failed to generate authentication tokens",
		}, nil
	}
	
	// Convert model to entity
	userEntity := s.modelToEntity(user)
	
	// Publish login event
	s.publishLoginEvent(ctx, user, req.SecurityContext)
	
	// Log authentication event asynchronously for audit purposes
	s.workerPoolManager.GetAuditPool().LogAuthenticationEvent(
		user.ID,
		user.OrganizationID,
		"user_login",
		"login",
		req.SecurityContext.IPAddress,
		req.SecurityContext.UserAgent,
		map[string]interface{}{
			"email":        user.Email,
			"login_method": "password",
			"two_factor":   user.TwoFactorEnabled,
		},
	)
	
	s.updateMetrics(func(m *AuthMetrics) {
		m.SuccessfulLogins++
	})
	
	return &AuthResponse{
		Success:   true,
		TokenPair: tokenPair,
		User:      userEntity,
	}, nil
}

// Register creates a new user account with organization
func (s *AuthService) Register(ctx context.Context, req *RegistrationRequest) (*RegistrationResponse, error) {
	s.updateMetrics(func(m *AuthMetrics) {
		m.RegistrationAttempts++
	})
	
	// Validate input
	if err := s.validateRegistrationRequest(req); err != nil {
		return &RegistrationResponse{
			Success: false,
			Error:   fmt.Sprintf("invalid request: %v", err),
		}, nil
	}
	
	// Check if organization domain already exists
	if exists, err := s.organizationDomainExists(ctx, req.OrganizationDomain); err != nil {
		s.logger.Error("Failed to check organization domain", zap.Error(err))
		return &RegistrationResponse{
			Success: false,
			Error:   "internal server error",
		}, nil
	} else if exists {
		return &RegistrationResponse{
			Success: false,
			Error:   "organization domain already exists",
		}, nil
	}
	
	// Check if user email already exists
	if exists, err := s.userEmailExists(ctx, req.Email); err != nil {
		s.logger.Error("Failed to check user email", zap.Error(err))
		return &RegistrationResponse{
			Success: false,
			Error:   "internal server error",
		}, nil
	} else if exists {
		return &RegistrationResponse{
			Success: false,
			Error:   "user email already exists",
		}, nil
	}
	
	// Hash password using specialized password worker pool
	hashedPassword, err := s.workerPoolManager.GetPasswordPool().HashPasswordSync(
		ctx, 
		req.Password, 
		uuid.New(), // Temporary user ID for registration
		10*time.Second,
	)
	if err != nil {
		s.logger.Error("Failed to hash password", zap.Error(err))
		return &RegistrationResponse{
			Success: false,
			Error:   "failed to process password",
		}, nil
	}
	
	// Create organization and user in transaction
	user, org, err := s.createUserWithOrganization(ctx, req, hashedPassword)
	if err != nil {
		s.logger.Error("Failed to create user with organization", zap.Error(err))
		return &RegistrationResponse{
			Success: false,
			Error:   "failed to create account",
		}, nil
	}
	
	// Generate token pair
	tokenPair, err := s.tokenService.GenerateTokenPair(
		ctx,
		user.ID,
		user.OrganizationID,
		user.Email,
		req.SecurityContext,
	)
	if err != nil {
		s.logger.Error("Failed to generate token pair for new user", zap.Error(err))
		return &RegistrationResponse{
			Success: false,
			Error:   "failed to generate authentication tokens",
		}, nil
	}
	
	// Convert model to entity
	userEntity := s.modelToEntity(user)
	
	// Publish registration events
	s.publishRegistrationEvents(ctx, user, org, req.SecurityContext)
	
	s.updateMetrics(func(m *AuthMetrics) {
		m.SuccessfulRegistrations++
	})
	
	return &RegistrationResponse{
		Success:   true,
		TokenPair: tokenPair,
		User:      userEntity,
	}, nil
}

// ChangePassword changes a user's password with token revocation
func (s *AuthService) ChangePassword(ctx context.Context, req *PasswordChangeRequest) error {
	// Get user
	user, err := s.userRepo.GetByID(ctx, req.UserID)
	if err != nil {
		return fmt.Errorf("user not found: %w", err)
	}
	
	// Verify current password
	if !s.verifyPassword(req.CurrentPassword, user.PasswordHash) {
		return fmt.Errorf("current password is incorrect")
	}
	
	// Hash new password using specialized password worker pool
	hashedPassword, err := s.workerPoolManager.GetPasswordPool().HashPasswordSync(
		ctx,
		req.NewPassword,
		req.UserID,
		10*time.Second,
	)
	if err != nil {
		return fmt.Errorf("failed to hash new password: %w", err)
	}
	
	// Update password
	user.PasswordHash = hashedPassword
	user.PasswordResetAt = &time.Time{}
	now := time.Now()
	*user.PasswordResetAt = now
	
	if err := s.userRepo.Update(ctx, user); err != nil {
		return fmt.Errorf("failed to update password: %w", err)
	}
	
	// Revoke all existing tokens for security
	if err := s.tokenService.RevokeAllUserTokens(ctx, req.UserID, entities.AccessToken); err != nil {
		s.logger.Warn("Failed to revoke access tokens after password change", zap.Error(err))
	}
	
	if err := s.tokenService.RevokeAllUserTokens(ctx, req.UserID, entities.RefreshToken); err != nil {
		s.logger.Warn("Failed to revoke refresh tokens after password change", zap.Error(err))
	}
	
	// Publish password change event
	s.publishPasswordChangeEvent(ctx, user, req.SecurityContext)
	
	s.updateMetrics(func(m *AuthMetrics) {
		m.PasswordChanges++
	})
	
	return nil
}

// SetupTwoFactor enables or disables two-factor authentication
func (s *AuthService) SetupTwoFactor(ctx context.Context, req *TwoFactorSetupRequest) (*TwoFactorSetupResponse, error) {
	user, err := s.userRepo.GetByID(ctx, req.UserID)
	if err != nil {
		return &TwoFactorSetupResponse{
			Success: false,
			Error:   "user not found",
		}, nil
	}
	
	if req.Enable {
		// Generate TOTP secret
		key, err := totp.Generate(totp.GenerateOpts{
			Issuer:      "ERP Suite",
			AccountName: user.Email,
			SecretSize:  32,
		})
		if err != nil {
			return &TwoFactorSetupResponse{
				Success: false,
				Error:   "failed to generate 2FA secret",
			}, nil
		}
		
		// Generate backup codes
		backupCodes := s.generateBackupCodes(10)
		
		// Update user
		user.TwoFactorEnabled = true
		user.TwoFactorSecret = key.Secret()
		user.BackupCodes = backupCodes
		
		if err := s.userRepo.Update(ctx, user); err != nil {
			return &TwoFactorSetupResponse{
				Success: false,
				Error:   "failed to enable 2FA",
			}, nil
		}
		
		s.updateMetrics(func(m *AuthMetrics) {
			m.TwoFactorEnabled++
		})
		
		return &TwoFactorSetupResponse{
			Success:     true,
			Secret:      key.Secret(),
			QRCodeURL:   key.URL(),
			BackupCodes: backupCodes,
		}, nil
	} else {
		// Disable 2FA
		user.TwoFactorEnabled = false
		user.TwoFactorSecret = ""
		user.BackupCodes = nil
		
		if err := s.userRepo.Update(ctx, user); err != nil {
			return &TwoFactorSetupResponse{
				Success: false,
				Error:   "failed to disable 2FA",
			}, nil
		}
		
		return &TwoFactorSetupResponse{
			Success: true,
		}, nil
	}
}

// Helper methods

func (s *AuthService) validateAuthRequest(req *AuthRequest) error {
	if req.Email == "" {
		return fmt.Errorf("email is required")
	}
	if req.Password == "" {
		return fmt.Errorf("password is required")
	}
	if req.SecurityContext == nil {
		return fmt.Errorf("security context is required")
	}
	return nil
}

func (s *AuthService) validateRegistrationRequest(req *RegistrationRequest) error {
	if req.Email == "" {
		return fmt.Errorf("email is required")
	}
	if req.Password == "" {
		return fmt.Errorf("password is required")
	}
	if len(req.Password) < 8 {
		return fmt.Errorf("password must be at least 8 characters")
	}
	if req.FirstName == "" {
		return fmt.Errorf("first name is required")
	}
	if req.LastName == "" {
		return fmt.Errorf("last name is required")
	}
	if req.OrganizationName == "" {
		return fmt.Errorf("organization name is required")
	}
	if req.OrganizationDomain == "" {
		return fmt.Errorf("organization domain is required")
	}
	return nil
}

func (s *AuthService) getUserByEmail(ctx context.Context, email string) (*models.User, error) {
	// Try cache first
	cacheKey := fmt.Sprintf("user:email:%s", email)
	if _, err := s.cacheManager.Get(ctx, cacheKey); err == nil {
		// Deserialize cached user (implementation depends on cache format)
		s.logger.Debug("User found in cache", zap.String("email", email))
		// For now, fall through to database query
	}
	
	// Get from database
	user, err := s.userRepo.GetByEmailWithOrganization(ctx, email)
	if err != nil {
		return nil, err
	}
	
	// Cache user for future requests
	// Implementation depends on cache serialization format
	
	return user, nil
}

func (s *AuthService) verifyPassword(password, hash string) bool {
	err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
	return err == nil
}

func (s *AuthService) hashPassword(password string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	return string(hash), nil
}

func (s *AuthService) verifyTwoFactorCode(secret, code string, backupCodes []string) bool {
	// Try TOTP code first
	if totp.Validate(code, secret) {
		return true
	}
	
	// Try backup codes
	for i, backupCode := range backupCodes {
		if backupCode == code {
			// Remove used backup code
			backupCodes = append(backupCodes[:i], backupCodes[i+1:]...)
			return true
		}
	}
	
	return false
}

func (s *AuthService) generateBackupCodes(count int) []string {
	codes := make([]string, count)
	for i := 0; i < count; i++ {
		bytes := make([]byte, 5)
		rand.Read(bytes)
		codes[i] = strings.ToUpper(base32.StdEncoding.EncodeToString(bytes))
	}
	return codes
}

func (s *AuthService) handleFailedLogin(ctx context.Context, user *models.User, ipAddress string) {
	// Increment failed login attempts
	if err := s.userRepo.IncrementFailedLoginAttempts(ctx, user.ID); err != nil {
		s.logger.Error("Failed to increment failed login attempts", zap.Error(err))
		return
	}
	
	// Check if user should be locked
	maxAttempts := 5 // Configure this
	if user.FailedLoginAttempts+1 >= maxAttempts {
		lockDuration := 15 * time.Minute // Configure this
		lockUntil := time.Now().Add(lockDuration)
		
		if err := s.userRepo.LockUser(ctx, user.ID, lockUntil); err != nil {
			s.logger.Error("Failed to lock user", zap.Error(err))
		}
	}
	
	// Record failed attempt for rate limiting
	s.recordFailedAttempt(ctx, user.Email, ipAddress)
}

func (s *AuthService) recordFailedAttempt(ctx context.Context, email, ipAddress string) {
	if err := s.rateLimiter.RecordAttempt(ctx, email, ipAddress, false); err != nil {
		s.logger.Error("Failed to record failed attempt", zap.Error(err))
	}
}

func (s *AuthService) organizationDomainExists(ctx context.Context, domain string) (bool, error) {
	_, err := s.orgRepo.GetByDomain(ctx, domain)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func (s *AuthService) userEmailExists(ctx context.Context, email string) (bool, error) {
	_, err := s.userRepo.GetByEmail(ctx, email)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func (s *AuthService) createUserWithOrganization(ctx context.Context, req *RegistrationRequest, passwordHash string) (*models.User, *models.Organization, error) {
	// This should be implemented as a database transaction
	// For now, implementing basic version
	
	// Create organization
	org := &models.Organization{
		ID:     uuid.New(),
		Name:   req.OrganizationName,
		Domain: req.OrganizationDomain,
		Settings: models.Settings{
			Timezone:         "UTC",
			DateFormat:       "YYYY-MM-DD",
			Currency:         "USD",
			Language:         "en",
			TwoFactorEnabled: false,
			SessionTimeout:   3600,
		},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	
	if err := s.orgRepo.Create(ctx, org); err != nil {
		return nil, nil, fmt.Errorf("failed to create organization: %w", err)
	}
	
	// Create user
	user := &models.User{
		ID:             uuid.New(),
		OrganizationID: org.ID,
		Email:          req.Email,
		PasswordHash:   passwordHash,
		FirstName:      req.FirstName,
		LastName:       req.LastName,
		IsActive:       true,
		IsVerified:     false,
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}
	
	if err := s.userRepo.Create(ctx, user); err != nil {
		return nil, nil, fmt.Errorf("failed to create user: %w", err)
	}
	
	// Create admin role and assign to user
	// This should be implemented with proper role creation
	
	return user, org, nil
}

func (s *AuthService) modelToEntity(user *models.User) *entities.User {
	return &entities.User{
		ID:                  user.ID,
		OrganizationID:      user.OrganizationID,
		Email:               user.Email,
		FirstName:           user.FirstName,
		LastName:            user.LastName,
		IsActive:            user.IsActive,
		IsVerified:          user.IsVerified,
		LastLoginAt:         user.LastLoginAt,
		PasswordResetAt:     user.PasswordResetAt,
		TwoFactorEnabled:    user.TwoFactorEnabled,
		FailedLoginAttempts: user.FailedLoginAttempts,
		LockedUntil:         user.LockedUntil,
		CreatedAt:           user.CreatedAt,
		UpdatedAt:           user.UpdatedAt,
		FullName:            user.GetFullName(),
	}
}

func (s *AuthService) publishLoginEvent(ctx context.Context, user *models.User, securityCtx *SecurityContext) {
	if s.eventPublisher == nil {
		return
	}
	
	loginData := events.UserLoggedInData{
		UserID:         user.ID,
		Email:          user.Email,
		OrganizationID: user.OrganizationID,
		LoginMethod:    "password",
	}
	
	s.eventPublisher.PublishUserLoggedIn(
		ctx,
		loginData,
		user.ID,
		user.OrganizationID,
		securityCtx.IPAddress,
		securityCtx.UserAgent,
	)
}

func (s *AuthService) publishRegistrationEvents(ctx context.Context, user *models.User, org *models.Organization, securityCtx *SecurityContext) {
	if s.eventPublisher == nil {
		return
	}
	
	// Publish organization created event
	orgData := events.OrganizationCreatedData{
		OrganizationID: org.ID,
		Name:           org.Name,
		Domain:         org.Domain,
		CreatedBy:      user.ID,
	}
	
	s.eventPublisher.PublishOrganizationCreated(
		ctx,
		orgData,
		user.ID,
		org.ID,
		securityCtx.IPAddress,
		securityCtx.UserAgent,
	)
	
	// Publish user registered event
	userData := events.UserRegisteredData{
		UserID:         user.ID,
		Email:          user.Email,
		FirstName:      user.FirstName,
		LastName:       user.LastName,
		OrganizationID: user.OrganizationID,
	}
	
	s.eventPublisher.PublishUserRegistered(
		ctx,
		userData,
		user.ID,
		org.ID,
		securityCtx.IPAddress,
		securityCtx.UserAgent,
	)
}

func (s *AuthService) publishPasswordChangeEvent(ctx context.Context, user *models.User, securityCtx *SecurityContext) {
	if s.eventPublisher == nil {
		return
	}
	
	passwordData := events.PasswordChangedData{
		UserID:         user.ID,
		OrganizationID: user.OrganizationID,
		ChangedBy:      user.ID,
	}
	
	s.eventPublisher.PublishPasswordChanged(
		ctx,
		passwordData,
		user.ID,
		user.OrganizationID,
		securityCtx.IPAddress,
		securityCtx.UserAgent,
	)
}

func (s *AuthService) updateMetrics(updateFunc func(*AuthMetrics)) {
	s.metrics.mu.Lock()
	defer s.metrics.mu.Unlock()
	updateFunc(s.metrics)
}

// GetMetrics returns current authentication service metrics
func (s *AuthService) GetMetrics() AuthMetrics {
	s.metrics.mu.RLock()
	defer s.metrics.mu.RUnlock()
	return *s.metrics
}

// Close gracefully shuts down the authentication service
func (s *AuthService) Close() error {
	if s.workerPoolManager != nil {
		s.workerPoolManager.StopSpecializedPools()
	}
	return nil
}