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

// ComprehensiveAuthServiceTestSuite provides comprehensive test coverage for AuthService
type ComprehensiveAuthServiceTestSuite struct {
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

// SetupTest sets up the test suite with proper mock expectations
func (suite *ComprehensiveAuthServiceTestSuite) SetupTest() {
	suite.ctx = context.Background()
	suite.mockUserRepo = new(MockUserRepository)
	suite.mockOrgRepo = new(MockOrganizationRepository)
	suite.mockRoleRepo = new(MockRoleRepository)
	suite.mockTokenSvc = new(MockTokenService)
	suite.mockEventPub = new(MockEventPublisher)
	suite.mockCache = NewMockCacheManager()
	
	suite.config = &config.Config{
		JWT: config.JWTConfig{
			Secret:        "test-secret-key-for-comprehensive-testing",
			AccessExpiry:  3600,
			RefreshExpiry: 604800,
		},
	}
	
	logger := zaptest.NewLogger(suite.T())
	
	// Setup default mock expectations for cache operations
	suite.mockCache.On("Get", mock.Anything, mock.AnythingOfType("string")).Return(nil, cache.ErrCacheNotFound).Maybe()
	suite.mockCache.On("Set", mock.Anything, mock.AnythingOfType("string"), mock.Anything, mock.AnythingOfType("time.Duration")).Return(nil).Maybe()
	suite.mockCache.On("Delete", mock.Anything, mock.AnythingOfType("string")).Return(nil).Maybe()
	suite.mockCache.On("Exists", mock.Anything, mock.AnythingOfType("string")).Return(false, nil).Maybe()
	
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
func (suite *ComprehensiveAuthServiceTestSuite) TearDownTest() {
	if suite.authService != nil {
		suite.authService.Close()
	}
}

// TestAuthenticate_Success_WithValidCredentials tests successful authentication with valid credentials
func (suite *ComprehensiveAuthServiceTestSuite) TestAuthenticate_Success_WithValidCredentials() {
	// Arrange
	userID := uuid.New()
	orgID := uuid.New()
	email := "test@example.com"
	password := "password123"
	
	user := &models.User{
		ID:             userID,
		OrganizationID: orgID,
		Email:          email,
		PasswordHash:   "$2a$10$N9qo8uLOickgx2ZMRZoMye.IjPeGvGzjYwSgjkMIUmEWqbBtxiupe", // "password123"
		FirstName:      "Test",
		LastName:       "User",
		IsActive:       true,
		IsVerified:     true,
		TwoFactorEnabled: false,
		FailedLoginAttempts: 0,
		LockedUntil:    nil,
	}
	
	tokenPair := &entities.TokenPair{
		AccessToken:  "access-token-123",
		RefreshToken: "refresh-token-456",
		ExpiresAt:    time.Now().Add(time.Hour),
		TokenType:    "Bearer",
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
	suite.mockUserRepo.On("GetByEmailWithOrganization", suite.ctx, email).Return(user, nil)
	suite.mockUserRepo.On("ResetFailedLoginAttempts", suite.ctx, userID).Return(nil)
	suite.mockUserRepo.On("UpdateLastLogin", suite.ctx, userID, mock.AnythingOfType("time.Time")).Return(nil)
	suite.mockTokenSvc.On("GenerateTokenPair", suite.ctx, userID, orgID, email, securityCtx).Return(tokenPair, nil)
	suite.mockEventPub.On("PublishUserLoggedIn", mock.Anything, mock.Anything, userID, orgID, securityCtx.IPAddress, securityCtx.UserAgent).Return(nil)
	
	// Act
	response, err := suite.authService.Authenticate(suite.ctx, req)
	
	// Assert
	suite.NoError(err)
	suite.True(response.Success)
	suite.Equal(tokenPair, response.TokenPair)
	suite.Equal(userID, response.User.ID)
	suite.Equal(email, response.User.Email)
	suite.False(response.RequiresTwoFA)
	suite.Empty(response.Error)
	
	// Verify all expectations
	suite.mockUserRepo.AssertExpectations(suite.T())
	suite.mockTokenSvc.AssertExpectations(suite.T())
	suite.mockEventPub.AssertExpectations(suite.T())
}

// TestAuthenticate_Failure_InvalidCredentials tests authentication failure with invalid credentials
func (suite *ComprehensiveAuthServiceTestSuite) TestAuthenticate_Failure_InvalidCredentials() {
	// Arrange
	email := "nonexistent@example.com"
	password := "wrongpassword"
	
	securityCtx := &SecurityContext{
		IPAddress: "192.168.1.1",
		UserAgent: "test-agent",
	}
	
	req := &AuthRequest{
		Email:           email,
		Password:        password,
		SecurityContext: securityCtx,
	}
	
	// Mock expectations - user not found
	suite.mockUserRepo.On("GetByEmailWithOrganization", suite.ctx, email).Return(nil, assert.AnError)
	
	// Act
	response, err := suite.authService.Authenticate(suite.ctx, req)
	
	// Assert
	suite.NoError(err)
	suite.False(response.Success)
	suite.Equal("invalid credentials", response.Error)
	suite.Nil(response.TokenPair)
	suite.Nil(response.User)
	suite.False(response.RequiresTwoFA)
	
	// Verify expectations
	suite.mockUserRepo.AssertExpectations(suite.T())
}

// TestAuthenticate_Failure_WrongPassword tests authentication failure with wrong password
func (suite *ComprehensiveAuthServiceTestSuite) TestAuthenticate_Failure_WrongPassword() {
	// Arrange
	userID := uuid.New()
	orgID := uuid.New()
	email := "test@example.com"
	password := "wrongpassword"
	
	user := &models.User{
		ID:             userID,
		OrganizationID: orgID,
		Email:          email,
		PasswordHash:   "$2a$10$N9qo8uLOickgx2ZMRZoMye.IjPeGvGzjYwSgjkMIUmEWqbBtxiupe", // "password123"
		FirstName:      "Test",
		LastName:       "User",
		IsActive:       true,
		IsVerified:     true,
		TwoFactorEnabled: false,
		FailedLoginAttempts: 0,
		LockedUntil:    nil,
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
	suite.mockUserRepo.On("GetByEmailWithOrganization", suite.ctx, email).Return(user, nil)
	suite.mockUserRepo.On("IncrementFailedLoginAttempts", suite.ctx, userID).Return(nil)
	
	// Act
	response, err := suite.authService.Authenticate(suite.ctx, req)
	
	// Assert
	suite.NoError(err)
	suite.False(response.Success)
	suite.Equal("invalid credentials", response.Error)
	suite.Nil(response.TokenPair)
	suite.Nil(response.User)
	suite.False(response.RequiresTwoFA)
	
	// Verify expectations
	suite.mockUserRepo.AssertExpectations(suite.T())
}

// TestAuthenticate_AccountLocked tests authentication with locked account
func (suite *ComprehensiveAuthServiceTestSuite) TestAuthenticate_AccountLocked() {
	// Arrange
	userID := uuid.New()
	orgID := uuid.New()
	email := "locked@example.com"
	password := "password123"
	lockUntil := time.Now().Add(15 * time.Minute)
	
	user := &models.User{
		ID:             userID,
		OrganizationID: orgID,
		Email:          email,
		PasswordHash:   "$2a$10$N9qo8uLOickgx2ZMRZoMye.IjPeGvGzjYwSgjkMIUmEWqbBtxiupe", // "password123"
		FirstName:      "Test",
		LastName:       "User",
		IsActive:       true,
		IsVerified:     true,
		TwoFactorEnabled: false,
		FailedLoginAttempts: 5,
		LockedUntil:    &lockUntil,
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
	suite.mockUserRepo.On("GetByEmailWithOrganization", suite.ctx, email).Return(user, nil)
	
	// Act
	response, err := suite.authService.Authenticate(suite.ctx, req)
	
	// Assert
	suite.NoError(err)
	suite.False(response.Success)
	suite.Contains(response.Error, "account is temporarily locked")
	suite.Nil(response.TokenPair)
	suite.Nil(response.User)
	suite.False(response.RequiresTwoFA)
	
	// Verify expectations
	suite.mockUserRepo.AssertExpectations(suite.T())
}

// TestAuthenticate_TwoFactorRequired tests authentication requiring 2FA
func (suite *ComprehensiveAuthServiceTestSuite) TestAuthenticate_TwoFactorRequired() {
	// Arrange
	userID := uuid.New()
	orgID := uuid.New()
	email := "2fa@example.com"
	password := "password123"
	
	user := &models.User{
		ID:             userID,
		OrganizationID: orgID,
		Email:          email,
		PasswordHash:   "$2a$10$N9qo8uLOickgx2ZMRZoMye.IjPeGvGzjYwSgjkMIUmEWqbBtxiupe", // "password123"
		FirstName:      "Test",
		LastName:       "User",
		IsActive:       true,
		IsVerified:     true,
		TwoFactorEnabled: true,
		TwoFactorSecret: "JBSWY3DPEHPK3PXP",
		FailedLoginAttempts: 0,
		LockedUntil:    nil,
	}
	
	securityCtx := &SecurityContext{
		IPAddress: "192.168.1.1",
		UserAgent: "test-agent",
	}
	
	req := &AuthRequest{
		Email:           email,
		Password:        password,
		SecurityContext: securityCtx,
		// No 2FA code provided
	}
	
	// Mock expectations
	suite.mockUserRepo.On("GetByEmailWithOrganization", suite.ctx, email).Return(user, nil)
	
	// Act
	response, err := suite.authService.Authenticate(suite.ctx, req)
	
	// Assert
	suite.NoError(err)
	suite.False(response.Success)
	suite.True(response.RequiresTwoFA)
	suite.Equal("two-factor authentication code required", response.Error)
	suite.Nil(response.TokenPair)
	suite.Nil(response.User)
	
	// Verify expectations
	suite.mockUserRepo.AssertExpectations(suite.T())
}

// TestRegister_Success tests successful user registration
func (suite *ComprehensiveAuthServiceTestSuite) TestRegister_Success() {
	// Arrange
	req := &RegistrationRequest{
		Email:              "newuser@example.com",
		Password:           "password123",
		FirstName:          "New",
		LastName:           "User",
		OrganizationName:   "Test Organization",
		OrganizationDomain: "testorg.com",
		SecurityContext: &SecurityContext{
			IPAddress: "192.168.1.1",
			UserAgent: "test-agent",
		},
	}
	
	tokenPair := &entities.TokenPair{
		AccessToken:  "access-token-123",
		RefreshToken: "refresh-token-456",
		ExpiresAt:    time.Now().Add(time.Hour),
		TokenType:    "Bearer",
	}
	
	// Mock expectations - organization and user don't exist
	suite.mockOrgRepo.On("GetByDomain", suite.ctx, req.OrganizationDomain).Return(nil, assert.AnError)
	suite.mockUserRepo.On("GetByEmail", suite.ctx, req.Email).Return(nil, assert.AnError)
	suite.mockOrgRepo.On("Create", suite.ctx, mock.AnythingOfType("*models.Organization")).Return(nil)
	suite.mockUserRepo.On("Create", suite.ctx, mock.AnythingOfType("*models.User")).Return(nil)
	suite.mockTokenSvc.On("GenerateTokenPair", suite.ctx, mock.AnythingOfType("uuid.UUID"), mock.AnythingOfType("uuid.UUID"), req.Email, req.SecurityContext).Return(tokenPair, nil)
	suite.mockEventPub.On("PublishOrganizationCreated", mock.Anything, mock.Anything, mock.AnythingOfType("uuid.UUID"), mock.AnythingOfType("uuid.UUID"), req.SecurityContext.IPAddress, req.SecurityContext.UserAgent).Return(nil)
	suite.mockEventPub.On("PublishUserRegistered", mock.Anything, mock.Anything, mock.AnythingOfType("uuid.UUID"), mock.AnythingOfType("uuid.UUID"), req.SecurityContext.IPAddress, req.SecurityContext.UserAgent).Return(nil)
	
	// Act
	response, err := suite.authService.Register(suite.ctx, req)
	
	// Assert
	suite.NoError(err)
	suite.True(response.Success)
	suite.Equal(tokenPair, response.TokenPair)
	suite.NotNil(response.User)
	suite.Equal(req.Email, response.User.Email)
	suite.Equal(req.FirstName, response.User.FirstName)
	suite.Equal(req.LastName, response.User.LastName)
	suite.Empty(response.Error)
	
	// Verify expectations
	suite.mockOrgRepo.AssertExpectations(suite.T())
	suite.mockUserRepo.AssertExpectations(suite.T())
	suite.mockTokenSvc.AssertExpectations(suite.T())
	suite.mockEventPub.AssertExpectations(suite.T())
}

// TestRegister_Failure_DuplicateEmail tests registration failure with duplicate email
func (suite *ComprehensiveAuthServiceTestSuite) TestRegister_Failure_DuplicateEmail() {
	// Arrange
	req := &RegistrationRequest{
		Email:              "existing@example.com",
		Password:           "password123",
		FirstName:          "New",
		LastName:           "User",
		OrganizationName:   "Test Organization",
		OrganizationDomain: "testorg.com",
		SecurityContext: &SecurityContext{
			IPAddress: "192.168.1.1",
			UserAgent: "test-agent",
		},
	}
	
	existingUser := &models.User{
		ID:    uuid.New(),
		Email: req.Email,
	}
	
	// Mock expectations - organization doesn't exist but user does
	suite.mockOrgRepo.On("GetByDomain", suite.ctx, req.OrganizationDomain).Return(nil, assert.AnError)
	suite.mockUserRepo.On("GetByEmail", suite.ctx, req.Email).Return(existingUser, nil)
	
	// Act
	response, err := suite.authService.Register(suite.ctx, req)
	
	// Assert
	suite.NoError(err)
	suite.False(response.Success)
	suite.Equal("user email already exists", response.Error)
	suite.Nil(response.TokenPair)
	suite.Nil(response.User)
	
	// Verify expectations
	suite.mockOrgRepo.AssertExpectations(suite.T())
	suite.mockUserRepo.AssertExpectations(suite.T())
}

// TestRegister_Failure_DuplicateOrganizationDomain tests registration failure with duplicate organization domain
func (suite *ComprehensiveAuthServiceTestSuite) TestRegister_Failure_DuplicateOrganizationDomain() {
	// Arrange
	req := &RegistrationRequest{
		Email:              "newuser@example.com",
		Password:           "password123",
		FirstName:          "New",
		LastName:           "User",
		OrganizationName:   "Test Organization",
		OrganizationDomain: "existingorg.com",
		SecurityContext: &SecurityContext{
			IPAddress: "192.168.1.1",
			UserAgent: "test-agent",
		},
	}
	
	existingOrg := &models.Organization{
		ID:     uuid.New(),
		Domain: req.OrganizationDomain,
	}
	
	// Mock expectations - organization exists
	suite.mockOrgRepo.On("GetByDomain", suite.ctx, req.OrganizationDomain).Return(existingOrg, nil)
	
	// Act
	response, err := suite.authService.Register(suite.ctx, req)
	
	// Assert
	suite.NoError(err)
	suite.False(response.Success)
	suite.Equal("organization domain already exists", response.Error)
	suite.Nil(response.TokenPair)
	suite.Nil(response.User)
	
	// Verify expectations
	suite.mockOrgRepo.AssertExpectations(suite.T())
}

// TestChangePassword_Success tests successful password change
func (suite *ComprehensiveAuthServiceTestSuite) TestChangePassword_Success() {
	// Arrange
	userID := uuid.New()
	orgID := uuid.New()
	
	user := &models.User{
		ID:             userID,
		OrganizationID: orgID,
		Email:          "test@example.com",
		PasswordHash:   "$2a$10$N9qo8uLOickgx2ZMRZoMye.IjPeGvGzjYwSgjkMIUmEWqbBtxiupe", // "password123"
		FirstName:      "Test",
		LastName:       "User",
		IsActive:       true,
	}
	
	req := &PasswordChangeRequest{
		UserID:          userID,
		CurrentPassword: "password123",
		NewPassword:     "newpassword456",
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
	suite.NoError(err)
	
	// Verify expectations
	suite.mockUserRepo.AssertExpectations(suite.T())
	suite.mockTokenSvc.AssertExpectations(suite.T())
	suite.mockEventPub.AssertExpectations(suite.T())
}

// TestChangePassword_Failure_WrongCurrentPassword tests password change failure with wrong current password
func (suite *ComprehensiveAuthServiceTestSuite) TestChangePassword_Failure_WrongCurrentPassword() {
	// Arrange
	userID := uuid.New()
	
	user := &models.User{
		ID:           userID,
		Email:        "test@example.com",
		PasswordHash: "$2a$10$N9qo8uLOickgx2ZMRZoMye.IjPeGvGzjYwSgjkMIUmEWqbBtxiupe", // "password123"
		FirstName:    "Test",
		LastName:     "User",
		IsActive:     true,
	}
	
	req := &PasswordChangeRequest{
		UserID:          userID,
		CurrentPassword: "wrongpassword",
		NewPassword:     "newpassword456",
		SecurityContext: &SecurityContext{
			IPAddress: "192.168.1.1",
			UserAgent: "test-agent",
		},
	}
	
	// Mock expectations
	suite.mockUserRepo.On("GetByID", suite.ctx, userID).Return(user, nil)
	
	// Act
	err := suite.authService.ChangePassword(suite.ctx, req)
	
	// Assert
	suite.Error(err)
	suite.Contains(err.Error(), "current password is incorrect")
	
	// Verify expectations
	suite.mockUserRepo.AssertExpectations(suite.T())
}

// TestSetupTwoFactor_Enable tests enabling 2FA
func (suite *ComprehensiveAuthServiceTestSuite) TestSetupTwoFactor_Enable() {
	// Arrange
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
	suite.NoError(err)
	suite.True(response.Success)
	suite.NotEmpty(response.Secret)
	suite.NotEmpty(response.QRCodeURL)
	suite.Len(response.BackupCodes, 10)
	suite.Empty(response.Error)
	
	// Verify expectations
	suite.mockUserRepo.AssertExpectations(suite.T())
}

// TestSetupTwoFactor_Disable tests disabling 2FA
func (suite *ComprehensiveAuthServiceTestSuite) TestSetupTwoFactor_Disable() {
	// Arrange
	userID := uuid.New()
	
	user := &models.User{
		ID:               userID,
		Email:            "test@example.com",
		TwoFactorEnabled: true,
		TwoFactorSecret:  "JBSWY3DPEHPK3PXP",
	}
	
	req := &TwoFactorSetupRequest{
		UserID: userID,
		Enable: false,
	}
	
	// Mock expectations
	suite.mockUserRepo.On("GetByID", suite.ctx, userID).Return(user, nil)
	suite.mockUserRepo.On("Update", suite.ctx, mock.AnythingOfType("*models.User")).Return(nil)
	
	// Act
	response, err := suite.authService.SetupTwoFactor(suite.ctx, req)
	
	// Assert
	suite.NoError(err)
	suite.True(response.Success)
	suite.Empty(response.Secret)
	suite.Empty(response.QRCodeURL)
	suite.Empty(response.BackupCodes)
	suite.Empty(response.Error)
	
	// Verify expectations
	suite.mockUserRepo.AssertExpectations(suite.T())
}

// TestValidateAuthRequest tests input validation for authentication requests
func (suite *ComprehensiveAuthServiceTestSuite) TestValidateAuthRequest() {
	testCases := []struct {
		name        string
		request     *AuthRequest
		expectError bool
		errorMsg    string
	}{
		{
			name: "valid request",
			request: &AuthRequest{
				Email:    "test@example.com",
				Password: "password123",
				SecurityContext: &SecurityContext{
					IPAddress: "192.168.1.1",
					UserAgent: "test-agent",
				},
			},
			expectError: false,
		},
		{
			name: "missing email",
			request: &AuthRequest{
				Password: "password123",
				SecurityContext: &SecurityContext{
					IPAddress: "192.168.1.1",
					UserAgent: "test-agent",
				},
			},
			expectError: true,
			errorMsg:    "email is required",
		},
		{
			name: "missing password",
			request: &AuthRequest{
				Email: "test@example.com",
				SecurityContext: &SecurityContext{
					IPAddress: "192.168.1.1",
					UserAgent: "test-agent",
				},
			},
			expectError: true,
			errorMsg:    "password is required",
		},
		{
			name: "missing security context",
			request: &AuthRequest{
				Email:    "test@example.com",
				Password: "password123",
			},
			expectError: true,
			errorMsg:    "security context is required",
		},
	}
	
	for _, tc := range testCases {
		suite.Run(tc.name, func() {
			err := suite.authService.validateAuthRequest(tc.request)
			if tc.expectError {
				suite.Error(err)
				suite.Contains(err.Error(), tc.errorMsg)
			} else {
				suite.NoError(err)
			}
		})
	}
}

// TestValidateRegistrationRequest tests input validation for registration requests
func (suite *ComprehensiveAuthServiceTestSuite) TestValidateRegistrationRequest() {
	testCases := []struct {
		name        string
		request     *RegistrationRequest
		expectError bool
		errorMsg    string
	}{
		{
			name: "valid request",
			request: &RegistrationRequest{
				Email:              "test@example.com",
				Password:           "password123",
				FirstName:          "Test",
				LastName:           "User",
				OrganizationName:   "Test Org",
				OrganizationDomain: "testorg.com",
			},
			expectError: false,
		},
		{
			name: "missing email",
			request: &RegistrationRequest{
				Password:           "password123",
				FirstName:          "Test",
				LastName:           "User",
				OrganizationName:   "Test Org",
				OrganizationDomain: "testorg.com",
			},
			expectError: true,
			errorMsg:    "email is required",
		},
		{
			name: "short password",
			request: &RegistrationRequest{
				Email:              "test@example.com",
				Password:           "short",
				FirstName:          "Test",
				LastName:           "User",
				OrganizationName:   "Test Org",
				OrganizationDomain: "testorg.com",
			},
			expectError: true,
			errorMsg:    "password must be at least 8 characters",
		},
		{
			name: "missing first name",
			request: &RegistrationRequest{
				Email:              "test@example.com",
				Password:           "password123",
				LastName:           "User",
				OrganizationName:   "Test Org",
				OrganizationDomain: "testorg.com",
			},
			expectError: true,
			errorMsg:    "first name is required",
		},
	}
	
	for _, tc := range testCases {
		suite.Run(tc.name, func() {
			err := suite.authService.validateRegistrationRequest(tc.request)
			if tc.expectError {
				suite.Error(err)
				suite.Contains(err.Error(), tc.errorMsg)
			} else {
				suite.NoError(err)
			}
		})
	}
}

// TestPasswordHashing tests password hashing and verification
func (suite *ComprehensiveAuthServiceTestSuite) TestPasswordHashing() {
	password := "testpassword123"
	
	// Test hashing
	hash, err := suite.authService.hashPassword(password)
	suite.NoError(err)
	suite.NotEmpty(hash)
	suite.NotEqual(password, hash)
	
	// Test verification with correct password
	isValid := suite.authService.verifyPassword(password, hash)
	suite.True(isValid)
	
	// Test verification with wrong password
	isValid = suite.authService.verifyPassword("wrongpassword", hash)
	suite.False(isValid)
}

// TestGetMetrics tests metrics collection
func (suite *ComprehensiveAuthServiceTestSuite) TestGetMetrics() {
	// Get initial metrics
	metrics := suite.authService.GetMetrics()
	suite.Equal(int64(0), metrics.AuthenticationAttempts)
	suite.Equal(int64(0), metrics.SuccessfulLogins)
	suite.Equal(int64(0), metrics.FailedLogins)
	
	// Update metrics manually for testing
	suite.authService.updateMetrics(func(m *AuthMetrics) {
		m.AuthenticationAttempts++
		m.SuccessfulLogins++
	})
	
	// Check updated metrics
	updatedMetrics := suite.authService.GetMetrics()
	suite.Equal(int64(1), updatedMetrics.AuthenticationAttempts)
	suite.Equal(int64(1), updatedMetrics.SuccessfulLogins)
	suite.Equal(int64(0), updatedMetrics.FailedLogins)
}

// TestConcurrentOperations tests thread safety of the auth service
func (suite *ComprehensiveAuthServiceTestSuite) TestConcurrentOperations() {
	const numGoroutines = 10
	const numOperations = 5
	
	// Setup mock expectations for concurrent operations
	suite.mockUserRepo.On("GetByID", mock.Anything, mock.AnythingOfType("uuid.UUID")).Return(&models.User{
		ID:           uuid.New(),
		Email:        "test@example.com",
		PasswordHash: "$2a$10$N9qo8uLOickgx2ZMRZoMye.IjPeGvGzjYwSgjkMIUmEWqbBtxiupe",
		IsActive:     true,
	}, nil).Maybe()
	
	// Run concurrent metric updates
	done := make(chan bool, numGoroutines)
	
	for i := 0; i < numGoroutines; i++ {
		go func() {
			defer func() { done <- true }()
			for j := 0; j < numOperations; j++ {
				suite.authService.updateMetrics(func(m *AuthMetrics) {
					m.AuthenticationAttempts++
				})
			}
		}()
	}
	
	// Wait for all goroutines to complete
	for i := 0; i < numGoroutines; i++ {
		<-done
	}
	
	// Check final metrics
	metrics := suite.authService.GetMetrics()
	suite.Equal(int64(numGoroutines*numOperations), metrics.AuthenticationAttempts)
}

// Run the comprehensive test suite
func TestComprehensiveAuthService(t *testing.T) {
	suite.Run(t, new(ComprehensiveAuthServiceTestSuite))
}