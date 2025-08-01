package services

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"
	"go.uber.org/zap/zaptest"

	"erp-auth-service/internal/cache"
	"erp-auth-service/internal/config"
	"erp-auth-service/internal/domain/entities"
	"erp-auth-service/internal/models"
)

// AuthSecurityTestSuite defines comprehensive security tests for AuthService
type AuthSecurityTestSuite struct {
	suite.Suite
	authService    *AuthService
	mockUserRepo   *MockUserRepository
	mockOrgRepo    *MockOrganizationRepository
	mockRoleRepo   *MockRoleRepository
	mockTokenSvc   *MockTokenService
	mockEventPub   *MockEventPublisher
	mockCache      *MockCacheManager
	config         *config.Config
	ctx            context.Context
}

// SetupTest sets up the security test suite
func (suite *AuthSecurityTestSuite) SetupTest() {
	suite.ctx = context.Background()
	suite.mockUserRepo = new(MockUserRepository)
	suite.mockOrgRepo = new(MockOrganizationRepository)
	suite.mockRoleRepo = new(MockRoleRepository)
	suite.mockTokenSvc = new(MockTokenService)
	suite.mockEventPub = new(MockEventPublisher)
	suite.mockCache = new(MockCacheManager)
	
	suite.config = &config.Config{
		JWT: config.JWTConfig{
			Secret:        "test-secret",
			AccessExpiry:  3600,
			RefreshExpiry: 604800,
		},
	}
	
	logger := zaptest.NewLogger(suite.T())
	
	suite.authService = NewAuthService(
		suite.config,
		logger,
		suite.mockUserRepo,
		suite.mockOrgRepo,
		suite.mockRoleRepo,
		suite.mockTokenSvc,
		suite.mockEventPub,
		suite.mockCache,
	)
}

// TearDownTest cleans up after each test
func (suite *AuthSecurityTestSuite) TearDownTest() {
	suite.authService.Close()
}

// TestRateLimitingProtection tests rate limiting functionality
func (suite *AuthSecurityTestSuite) TestRateLimitingProtection() {
	email := "test@example.com"
	password := "password123"
	
	securityCtx := &SecurityContext{
		IPAddress: "192.168.1.1",
		UserAgent: "test-agent",
	}
	
	req := &AuthRequest{
		Email:           email,
		Password:        password,
		SecurityContext: securityCtx,
	}
	
	// Mock rate limiter blocking the request
	suite.mockCache.On("Get", mock.Anything, mock.AnythingOfType("string")).Return(nil, cache.ErrCacheNotFound)
	suite.mockCache.On("Exists", mock.Anything, mock.AnythingOfType("string")).Return(true, nil) // Simulate blocked
	// Add mock for the case where rate limiter is not blocked but user lookup fails
	suite.mockUserRepo.On("GetByEmailWithOrganization", suite.ctx, email).Return(nil, assert.AnError).Maybe()
	
	// Act
	response, err := suite.authService.Authenticate(suite.ctx, req)
	
	// Assert
	assert.NoError(suite.T(), err)
	assert.False(suite.T(), response.Success)
	assert.Contains(suite.T(), response.Error, "too many authentication attempts")
	
	// Verify expectations
	suite.mockCache.AssertExpectations(suite.T())
}

// TestBruteForceProtection tests brute force protection
func (suite *AuthSecurityTestSuite) TestBruteForceProtection() {
	userID := uuid.New()
	orgID := uuid.New()
	email := "test@example.com"
	password := "wrongpassword"
	
	// Create user with multiple failed attempts
	user := &models.User{
		ID:                  userID,
		OrganizationID:      orgID,
		Email:               email,
		PasswordHash:        "$2a$10$N9qo8uLOickgx2ZMRZoMye.IjPeGvGzjYwSgjkMIUmEWqbBtxiupe", // "password123"
		FirstName:           "Test",
		LastName:            "User",
		IsActive:            true,
		IsVerified:          true,
		TwoFactorEnabled:    false,
		FailedLoginAttempts: 4, // One more attempt will lock the account
		LockedUntil:         nil,
	}
	
	securityCtx := &SecurityContext{
		IPAddress: "192.168.1.1",
		UserAgent: "test-agent",
	}
	
	req := &AuthRequest{
		Email:           email,
		Password:        password,
		SecurityContext: securityCtx,
	}
	
	// Mock expectations
	suite.mockCache.On("Get", mock.Anything, mock.AnythingOfType("string")).Return(nil, cache.ErrCacheNotFound)
	suite.mockCache.On("Exists", mock.Anything, mock.AnythingOfType("string")).Return(false, nil)
	suite.mockCache.On("Set", mock.Anything, mock.AnythingOfType("string"), mock.Anything, mock.AnythingOfType("time.Duration")).Return(nil)
	suite.mockUserRepo.On("GetByEmailWithOrganization", suite.ctx, email).Return(user, nil)
	suite.mockUserRepo.On("IncrementFailedLoginAttempts", suite.ctx, userID).Return(nil)
	suite.mockUserRepo.On("LockUser", suite.ctx, userID, mock.AnythingOfType("time.Time")).Return(nil)
	
	// Act
	response, err := suite.authService.Authenticate(suite.ctx, req)
	
	// Assert
	assert.NoError(suite.T(), err)
	assert.False(suite.T(), response.Success)
	assert.Equal(suite.T(), "invalid credentials", response.Error)
	
	// Verify that user was locked after too many failed attempts
	suite.mockUserRepo.AssertExpectations(suite.T())
}

// TestPasswordHashingSecurity tests password hashing security
func (suite *AuthSecurityTestSuite) TestPasswordHashingSecurity() {
	password := "testpassword123"
	
	// Test password hashing
	hash1, err1 := suite.authService.hashPassword(password)
	hash2, err2 := suite.authService.hashPassword(password)
	
	// Assert
	assert.NoError(suite.T(), err1)
	assert.NoError(suite.T(), err2)
	assert.NotEmpty(suite.T(), hash1)
	assert.NotEmpty(suite.T(), hash2)
	assert.NotEqual(suite.T(), hash1, hash2) // Hashes should be different due to salt
	assert.True(suite.T(), suite.authService.verifyPassword(password, hash1))
	assert.True(suite.T(), suite.authService.verifyPassword(password, hash2))
	assert.False(suite.T(), suite.authService.verifyPassword("wrongpassword", hash1))
}

// TestTwoFactorAuthenticationSecurity tests 2FA security features
func (suite *AuthSecurityTestSuite) TestTwoFactorAuthenticationSecurity() {
	userID := uuid.New()
	
	user := &models.User{
		ID:               userID,
		Email:            "test@example.com",
		TwoFactorEnabled: false,
	}
	
	req := &TwoFactorSetupRequest{
		UserID: userID,
		Enable: true,
	}
	
	// Mock expectations
	suite.mockUserRepo.On("GetByID", suite.ctx, userID).Return(user, nil)
	suite.mockUserRepo.On("Update", suite.ctx, mock.AnythingOfType("*models.User")).Return(nil)
	
	// Act
	response, err := suite.authService.SetupTwoFactor(suite.ctx, req)
	
	// Assert
	assert.NoError(suite.T(), err)
	assert.True(suite.T(), response.Success)
	assert.NotEmpty(suite.T(), response.Secret)
	assert.NotEmpty(suite.T(), response.QRCodeURL)
	assert.Len(suite.T(), response.BackupCodes, 10)
	
	// Verify backup codes are unique
	codeMap := make(map[string]bool)
	for _, code := range response.BackupCodes {
		assert.False(suite.T(), codeMap[code], "Backup codes should be unique")
		codeMap[code] = true
		assert.NotEmpty(suite.T(), code)
	}
	
	// Verify expectations
	suite.mockUserRepo.AssertExpectations(suite.T())
}

// TestPasswordChangeTokenRevocation tests that password changes revoke tokens
func (suite *AuthSecurityTestSuite) TestPasswordChangeTokenRevocation() {
	userID := uuid.New()
	orgID := uuid.New()
	
	// Generate correct password hash for "password123"
	hashedPassword, _ := suite.authService.hashPassword("password123")
	user := &models.User{
		ID:             userID,
		OrganizationID: orgID,
		Email:          "test@example.com",
		PasswordHash:   hashedPassword,
		FirstName:      "Test",
		LastName:       "User",
		IsActive:       true,
	}
	
	req := &PasswordChangeRequest{
		UserID:          userID,
		CurrentPassword: "password123",
		NewPassword:     "newpassword123",
		SecurityContext: &SecurityContext{
			IPAddress: "192.168.1.1",
			UserAgent: "test-agent",
		},
	}
	
	// Mock expectations
	suite.mockUserRepo.On("GetByID", suite.ctx, userID).Return(user, nil)
	suite.mockUserRepo.On("Update", suite.ctx, mock.AnythingOfType("*models.User")).Return(nil)
	suite.mockTokenSvc.On("RevokeAllUserTokens", suite.ctx, userID, entities.AccessToken).Return(nil)
	suite.mockTokenSvc.On("RevokeAllUserTokens", suite.ctx, userID, entities.RefreshToken).Return(nil)
	suite.mockEventPub.On("PublishPasswordChanged", mock.Anything, mock.Anything, userID, orgID, req.SecurityContext.IPAddress, req.SecurityContext.UserAgent).Return(nil)
	
	// Act
	err := suite.authService.ChangePassword(suite.ctx, req)
	
	// Assert
	assert.NoError(suite.T(), err)
	
	// Verify that tokens were revoked
	suite.mockTokenSvc.AssertExpectations(suite.T())
	suite.mockUserRepo.AssertExpectations(suite.T())
	suite.mockEventPub.AssertExpectations(suite.T())
}

// TestSecurityEventPublishing tests that security events are published
func (suite *AuthSecurityTestSuite) TestSecurityEventPublishing() {
	userID := uuid.New()
	orgID := uuid.New()
	email := "test@example.com"
	
	// Generate correct password hash
	hashedPassword, _ := suite.authService.hashPassword("password123")
	user := &models.User{
		ID:             userID,
		OrganizationID: orgID,
		Email:          email,
		PasswordHash:   hashedPassword,
		FirstName:      "Test",
		LastName:       "User",
		IsActive:       true,
		IsVerified:     true,
		TwoFactorEnabled: false,
		FailedLoginAttempts: 0,
		LockedUntil:    nil,
	}
	
	tokenPair := &entities.TokenPair{
		AccessToken:  "access-token",
		RefreshToken: "refresh-token",
		ExpiresAt:    time.Now().Add(time.Hour),
		TokenType:    "Bearer",
	}
	
	securityCtx := &SecurityContext{
		IPAddress: "192.168.1.1",
		UserAgent: "test-agent",
	}
	
	req := &AuthRequest{
		Email:           email,
		Password:        "password123",
		SecurityContext: securityCtx,
	}
	
	// Mock expectations
	suite.mockCache.On("Get", mock.Anything, mock.AnythingOfType("string")).Return(nil, cache.ErrCacheNotFound)
	suite.mockCache.On("Exists", mock.Anything, mock.AnythingOfType("string")).Return(false, nil)
	suite.mockUserRepo.On("GetByEmailWithOrganization", suite.ctx, email).Return(user, nil)
	suite.mockUserRepo.On("ResetFailedLoginAttempts", suite.ctx, userID).Return(nil)
	suite.mockUserRepo.On("UpdateLastLogin", suite.ctx, userID, mock.AnythingOfType("time.Time")).Return(nil)
	suite.mockTokenSvc.On("GenerateTokenPair", suite.ctx, userID, orgID, email, securityCtx).Return(tokenPair, nil)
	suite.mockEventPub.On("PublishUserLoggedIn", mock.Anything, mock.Anything, userID, orgID, securityCtx.IPAddress, securityCtx.UserAgent).Return(nil)
	
	// Act
	response, err := suite.authService.Authenticate(suite.ctx, req)
	
	// Assert
	assert.NoError(suite.T(), err)
	assert.True(suite.T(), response.Success)
	
	// Verify that security event was published
	suite.mockEventPub.AssertExpectations(suite.T())
}

// TestConcurrentAuthenticationSecurity tests concurrent authentication security
func (suite *AuthSecurityTestSuite) TestConcurrentAuthenticationSecurity() {
	userID := uuid.New()
	orgID := uuid.New()
	email := "test@example.com"
	
	// Generate correct password hash
	hashedPassword, _ := suite.authService.hashPassword("password123")
	user := &models.User{
		ID:             userID,
		OrganizationID: orgID,
		Email:          email,
		PasswordHash:   hashedPassword,
		FirstName:      "Test",
		LastName:       "User",
		IsActive:       true,
		IsVerified:     true,
		TwoFactorEnabled: false,
		FailedLoginAttempts: 0,
		LockedUntil:    nil,
	}
	
	tokenPair := &entities.TokenPair{
		AccessToken:  "access-token",
		RefreshToken: "refresh-token",
		ExpiresAt:    time.Now().Add(time.Hour),
		TokenType:    "Bearer",
	}
	
	securityCtx := &SecurityContext{
		IPAddress: "192.168.1.1",
		UserAgent: "test-agent",
	}
	
	req := &AuthRequest{
		Email:           email,
		Password:        "password123",
		SecurityContext: securityCtx,
	}
	
	// Mock expectations for multiple concurrent requests
	suite.mockCache.On("Get", mock.Anything, mock.AnythingOfType("string")).Return(nil, cache.ErrCacheNotFound)
	suite.mockCache.On("Exists", mock.Anything, mock.AnythingOfType("string")).Return(false, nil)
	suite.mockUserRepo.On("GetByEmailWithOrganization", suite.ctx, email).Return(user, nil)
	suite.mockUserRepo.On("ResetFailedLoginAttempts", suite.ctx, userID).Return(nil)
	suite.mockUserRepo.On("UpdateLastLogin", suite.ctx, userID, mock.AnythingOfType("time.Time")).Return(nil)
	suite.mockTokenSvc.On("GenerateTokenPair", suite.ctx, userID, orgID, email, securityCtx).Return(tokenPair, nil)
	suite.mockEventPub.On("PublishUserLoggedIn", mock.Anything, mock.Anything, userID, orgID, securityCtx.IPAddress, securityCtx.UserAgent).Return(nil)
	
	// Test concurrent authentication requests
	const numRequests = 10
	results := make(chan *AuthResponse, numRequests)
	errors := make(chan error, numRequests)
	
	for i := 0; i < numRequests; i++ {
		go func() {
			response, err := suite.authService.Authenticate(suite.ctx, req)
			results <- response
			errors <- err
		}()
	}
	
	// Collect results
	successCount := 0
	for i := 0; i < numRequests; i++ {
		response := <-results
		err := <-errors
		
		assert.NoError(suite.T(), err)
		if response.Success {
			successCount++
		}
	}
	
	// At least some requests should succeed (depending on mock setup)
	assert.Greater(suite.T(), successCount, 0)
}

// TestInputValidationSecurity tests input validation security
func (suite *AuthSecurityTestSuite) TestInputValidationSecurity() {
	testCases := []struct {
		name        string
		request     *AuthRequest
		expectedErr string
	}{
		{
			name: "Empty email",
			request: &AuthRequest{
				Email:    "",
				Password: "password123",
				SecurityContext: &SecurityContext{
					IPAddress: "192.168.1.1",
					UserAgent: "test-agent",
				},
			},
			expectedErr: "email is required",
		},
		{
			name: "Empty password",
			request: &AuthRequest{
				Email:    "test@example.com",
				Password: "",
				SecurityContext: &SecurityContext{
					IPAddress: "192.168.1.1",
					UserAgent: "test-agent",
				},
			},
			expectedErr: "password is required",
		},
		{
			name: "Missing security context",
			request: &AuthRequest{
				Email:           "test@example.com",
				Password:        "password123",
				SecurityContext: nil,
			},
			expectedErr: "security context is required",
		},
	}
	
	for _, tc := range testCases {
		suite.T().Run(tc.name, func(t *testing.T) {
			response, err := suite.authService.Authenticate(suite.ctx, tc.request)
			
			assert.NoError(t, err)
			assert.False(t, response.Success)
			assert.Contains(t, response.Error, tc.expectedErr)
		})
	}
}

// TestRegistrationInputValidation tests registration input validation
func (suite *AuthSecurityTestSuite) TestRegistrationInputValidation() {
	testCases := []struct {
		name        string
		request     *RegistrationRequest
		expectedErr string
	}{
		{
			name: "Empty email",
			request: &RegistrationRequest{
				Email:              "",
				Password:           "password123",
				FirstName:          "Test",
				LastName:           "User",
				OrganizationName:   "Test Org",
				OrganizationDomain: "testorg.com",
				SecurityContext: &SecurityContext{
					IPAddress: "192.168.1.1",
					UserAgent: "test-agent",
				},
			},
			expectedErr: "email is required",
		},
		{
			name: "Short password",
			request: &RegistrationRequest{
				Email:              "test@example.com",
				Password:           "123",
				FirstName:          "Test",
				LastName:           "User",
				OrganizationName:   "Test Org",
				OrganizationDomain: "testorg.com",
				SecurityContext: &SecurityContext{
					IPAddress: "192.168.1.1",
					UserAgent: "test-agent",
				},
			},
			expectedErr: "password must be at least 8 characters",
		},
		{
			name: "Empty first name",
			request: &RegistrationRequest{
				Email:              "test@example.com",
				Password:           "password123",
				FirstName:          "",
				LastName:           "User",
				OrganizationName:   "Test Org",
				OrganizationDomain: "testorg.com",
				SecurityContext: &SecurityContext{
					IPAddress: "192.168.1.1",
					UserAgent: "test-agent",
				},
			},
			expectedErr: "first name is required",
		},
	}
	
	for _, tc := range testCases {
		suite.T().Run(tc.name, func(t *testing.T) {
			response, err := suite.authService.Register(suite.ctx, tc.request)
			
			assert.NoError(t, err)
			assert.False(t, response.Success)
			assert.Contains(t, response.Error, tc.expectedErr)
		})
	}
}

// TestMetricsTracking tests that security metrics are properly tracked
func (suite *AuthSecurityTestSuite) TestMetricsTracking() {
	// Get initial metrics
	initialMetrics := suite.authService.GetMetrics()
	
	// Perform some operations that should update metrics
	email := "test@example.com"
	securityCtx := &SecurityContext{
		IPAddress: "192.168.1.1",
		UserAgent: "test-agent",
	}
	
	req := &AuthRequest{
		Email:           email,
		Password:        "wrongpassword",
		SecurityContext: securityCtx,
	}
	
	// Mock for failed authentication
	suite.mockCache.On("Get", mock.Anything, mock.AnythingOfType("string")).Return(nil, cache.ErrCacheNotFound)
	suite.mockCache.On("Exists", mock.Anything, mock.AnythingOfType("string")).Return(false, nil)
	suite.mockCache.On("Set", mock.Anything, mock.AnythingOfType("string"), mock.Anything, mock.AnythingOfType("time.Duration")).Return(nil)
	suite.mockUserRepo.On("GetByEmailWithOrganization", suite.ctx, email).Return(nil, assert.AnError)
	
	// Perform failed authentication
	_, _ = suite.authService.Authenticate(suite.ctx, req)
	
	// Get updated metrics
	updatedMetrics := suite.authService.GetMetrics()
	
	// Assert metrics were updated
	assert.Greater(suite.T(), updatedMetrics.AuthenticationAttempts, initialMetrics.AuthenticationAttempts)
	assert.Greater(suite.T(), updatedMetrics.FailedLogins, initialMetrics.FailedLogins)
}

// TestAuthService_SecurityFeatures runs the security test suite
func TestAuthService_SecurityFeatures(t *testing.T) {
	suite.Run(t, new(AuthSecurityTestSuite))
}