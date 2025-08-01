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



// AuthServiceTestSuite defines the test suite for AuthService
type AuthServiceTestSuite struct {
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

// SetupTest sets up the test suite
func (suite *AuthServiceTestSuite) SetupTest() {
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
func (suite *AuthServiceTestSuite) TearDownTest() {
	suite.authService.Close()
}

// TestAuthenticate_Success tests successful authentication
func (suite *AuthServiceTestSuite) TestAuthenticate_Success() {
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
		Password:        password,
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
	assert.Equal(suite.T(), tokenPair, response.TokenPair)
	assert.Equal(suite.T(), userID, response.User.ID)
	assert.Equal(suite.T(), email, response.User.Email)
	
	// Verify all expectations
	suite.mockUserRepo.AssertExpectations(suite.T())
	suite.mockTokenSvc.AssertExpectations(suite.T())
	suite.mockEventPub.AssertExpectations(suite.T())
}

// TestAuthenticate_InvalidCredentials tests authentication with invalid credentials
func (suite *AuthServiceTestSuite) TestAuthenticate_InvalidCredentials() {
	// Arrange
	email := "test@example.com"
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
	suite.mockCache.On("Get", mock.Anything, mock.AnythingOfType("string")).Return(nil, cache.ErrCacheNotFound)
	suite.mockCache.On("Exists", mock.Anything, mock.AnythingOfType("string")).Return(false, nil)
	suite.mockUserRepo.On("GetByEmailWithOrganization", suite.ctx, email).Return(nil, assert.AnError)
	
	// Act
	response, err := suite.authService.Authenticate(suite.ctx, req)
	
	// Assert
	assert.NoError(suite.T(), err)
	assert.False(suite.T(), response.Success)
	assert.Equal(suite.T(), "invalid credentials", response.Error)
	assert.Nil(suite.T(), response.TokenPair)
	
	// Verify expectations
	suite.mockUserRepo.AssertExpectations(suite.T())
}

// TestAuthenticate_WrongPassword tests authentication with wrong password
func (suite *AuthServiceTestSuite) TestAuthenticate_WrongPassword() {
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
	suite.mockCache.On("Get", mock.Anything, mock.AnythingOfType("string")).Return(nil, cache.ErrCacheNotFound)
	suite.mockCache.On("Exists", mock.Anything, mock.AnythingOfType("string")).Return(false, nil)
	suite.mockUserRepo.On("GetByEmailWithOrganization", suite.ctx, email).Return(user, nil)
	suite.mockUserRepo.On("IncrementFailedLoginAttempts", suite.ctx, userID).Return(nil)
	
	// Act
	response, err := suite.authService.Authenticate(suite.ctx, req)
	
	// Assert
	assert.NoError(suite.T(), err)
	assert.False(suite.T(), response.Success)
	assert.Equal(suite.T(), "invalid credentials", response.Error)
	assert.Nil(suite.T(), response.TokenPair)
	
	// Verify expectations
	suite.mockUserRepo.AssertExpectations(suite.T())
}

// TestAuthenticate_AccountLocked tests authentication with locked account
func (suite *AuthServiceTestSuite) TestAuthenticate_AccountLocked() {
	// Arrange
	userID := uuid.New()
	orgID := uuid.New()
	email := "test@example.com"
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
	suite.mockCache.On("Get", mock.Anything, mock.AnythingOfType("string")).Return(nil, cache.ErrCacheNotFound)
	suite.mockCache.On("Exists", mock.Anything, mock.AnythingOfType("string")).Return(false, nil)
	suite.mockUserRepo.On("GetByEmailWithOrganization", suite.ctx, email).Return(user, nil)
	
	// Act
	response, err := suite.authService.Authenticate(suite.ctx, req)
	
	// Assert
	assert.NoError(suite.T(), err)
	assert.False(suite.T(), response.Success)
	assert.Contains(suite.T(), response.Error, "account is temporarily locked")
	assert.Nil(suite.T(), response.TokenPair)
	
	// Verify expectations
	suite.mockUserRepo.AssertExpectations(suite.T())
}

// TestAuthenticate_TwoFactorRequired tests authentication requiring 2FA
func (suite *AuthServiceTestSuite) TestAuthenticate_TwoFactorRequired() {
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
	suite.mockCache.On("Get", mock.Anything, mock.AnythingOfType("string")).Return(nil, cache.ErrCacheNotFound)
	suite.mockCache.On("Exists", mock.Anything, mock.AnythingOfType("string")).Return(false, nil)
	suite.mockUserRepo.On("GetByEmailWithOrganization", suite.ctx, email).Return(user, nil)
	
	// Act
	response, err := suite.authService.Authenticate(suite.ctx, req)
	
	// Assert
	assert.NoError(suite.T(), err)
	assert.False(suite.T(), response.Success)
	assert.True(suite.T(), response.RequiresTwoFA)
	assert.Equal(suite.T(), "two-factor authentication code required", response.Error)
	assert.Nil(suite.T(), response.TokenPair)
	
	// Verify expectations
	suite.mockUserRepo.AssertExpectations(suite.T())
}

// TestRegister_Success tests successful user registration
func (suite *AuthServiceTestSuite) TestRegister_Success() {
	// Arrange
	req := &RegistrationRequest{
		Email:              "newuser@example.com",
		Password:           "password123",
		FirstName:          "New",
		LastName:           "User",
		OrganizationName:   "Test Org",
		OrganizationDomain: "testorg.com",
		SecurityContext: &SecurityContext{
			IPAddress: "192.168.1.1",
			UserAgent: "test-agent",
		},
	}
	
	tokenPair := &entities.TokenPair{
		AccessToken:  "access-token",
		RefreshToken: "refresh-token",
		ExpiresAt:    time.Now().Add(time.Hour),
		TokenType:    "Bearer",
	}
	
	// Mock expectations
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
	assert.NoError(suite.T(), err)
	assert.True(suite.T(), response.Success)
	assert.Equal(suite.T(), tokenPair, response.TokenPair)
	assert.NotNil(suite.T(), response.User)
	assert.Equal(suite.T(), req.Email, response.User.Email)
	
	// Verify expectations
	suite.mockOrgRepo.AssertExpectations(suite.T())
	suite.mockUserRepo.AssertExpectations(suite.T())
	suite.mockTokenSvc.AssertExpectations(suite.T())
	suite.mockEventPub.AssertExpectations(suite.T())
}

// TestRegister_DuplicateEmail tests registration with duplicate email
func (suite *AuthServiceTestSuite) TestRegister_DuplicateEmail() {
	// Arrange
	req := &RegistrationRequest{
		Email:              "existing@example.com",
		Password:           "password123",
		FirstName:          "New",
		LastName:           "User",
		OrganizationName:   "Test Org",
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
	
	// Mock expectations
	suite.mockOrgRepo.On("GetByDomain", suite.ctx, req.OrganizationDomain).Return(nil, assert.AnError)
	suite.mockUserRepo.On("GetByEmail", suite.ctx, req.Email).Return(existingUser, nil)
	
	// Act
	response, err := suite.authService.Register(suite.ctx, req)
	
	// Assert
	assert.NoError(suite.T(), err)
	assert.False(suite.T(), response.Success)
	assert.Equal(suite.T(), "user email already exists", response.Error)
	assert.Nil(suite.T(), response.TokenPair)
	
	// Verify expectations
	suite.mockOrgRepo.AssertExpectations(suite.T())
	suite.mockUserRepo.AssertExpectations(suite.T())
}

// TestChangePassword_Success tests successful password change
func (suite *AuthServiceTestSuite) TestChangePassword_Success() {
	// Arrange
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
	
	// Verify expectations
	suite.mockUserRepo.AssertExpectations(suite.T())
	suite.mockTokenSvc.AssertExpectations(suite.T())
	suite.mockEventPub.AssertExpectations(suite.T())
}

// TestChangePassword_WrongCurrentPassword tests password change with wrong current password
func (suite *AuthServiceTestSuite) TestChangePassword_WrongCurrentPassword() {
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
		NewPassword:     "newpassword123",
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
	assert.Error(suite.T(), err)
	assert.Contains(suite.T(), err.Error(), "current password is incorrect")
	
	// Verify expectations
	suite.mockUserRepo.AssertExpectations(suite.T())
}

// TestSetupTwoFactor_Enable tests enabling 2FA
func (suite *AuthServiceTestSuite) TestSetupTwoFactor_Enable() {
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
	assert.NoError(suite.T(), err)
	assert.True(suite.T(), response.Success)
	assert.NotEmpty(suite.T(), response.Secret)
	assert.NotEmpty(suite.T(), response.QRCodeURL)
	assert.Len(suite.T(), response.BackupCodes, 10)
	
	// Verify expectations
	suite.mockUserRepo.AssertExpectations(suite.T())
}

// TestSetupTwoFactor_Disable tests disabling 2FA
func (suite *AuthServiceTestSuite) TestSetupTwoFactor_Disable() {
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
	assert.NoError(suite.T(), err)
	assert.True(suite.T(), response.Success)
	assert.Empty(suite.T(), response.Secret)
	assert.Empty(suite.T(), response.QRCodeURL)
	assert.Empty(suite.T(), response.BackupCodes)
	
	// Verify expectations
	suite.mockUserRepo.AssertExpectations(suite.T())
}

// TestAuthService runs the test suite
func TestAuthService(t *testing.T) {
	suite.Run(t, new(AuthServiceTestSuite))
}

// Benchmark tests for performance validation

func BenchmarkAuthenticate(b *testing.B) {
	// Setup
	mockUserRepo := new(MockUserRepository)
	mockOrgRepo := new(MockOrganizationRepository)
	mockRoleRepo := new(MockRoleRepository)
	mockTokenSvc := new(MockTokenService)
	mockEventPub := new(MockEventPublisher)
	mockCache := new(MockCacheManager)
	
	config := &config.Config{
		JWT: config.JWTConfig{
			Secret:        "test-secret",
			AccessExpiry:  3600,
			RefreshExpiry: 604800,
		},
	}
	
	logger := zaptest.NewLogger(b)
	
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
	email := "test@example.com"
	
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
	
	// Mock setup
	mockCache.On("Get", mock.Anything, mock.AnythingOfType("string")).Return(nil, cache.ErrCacheNotFound)
	mockUserRepo.On("GetByEmailWithOrganization", mock.Anything, email).Return(user, nil)
	mockUserRepo.On("ResetFailedLoginAttempts", mock.Anything, userID).Return(nil)
	mockUserRepo.On("UpdateLastLogin", mock.Anything, userID, mock.AnythingOfType("time.Time")).Return(nil)
	mockTokenSvc.On("GenerateTokenPair", mock.Anything, userID, orgID, email, securityCtx).Return(tokenPair, nil)
	mockEventPub.On("PublishUserLoggedIn", mock.Anything, mock.Anything, userID, orgID, securityCtx.IPAddress, securityCtx.UserAgent).Return(nil)
	
	ctx := context.Background()
	
	// Benchmark
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := authService.Authenticate(ctx, req)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkPasswordHashing(b *testing.B) {
	authService := &AuthService{}
	password := "password123"
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := authService.hashPassword(password)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkPasswordVerification(b *testing.B) {
	authService := &AuthService{}
	password := "password123"
	hash := "$2a$10$N9qo8uLOickgx2ZMRZoMye.IjPeGvGzjYwSgjkMIUmEWqbBtxiupe"
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		authService.verifyPassword(password, hash)
	}
}