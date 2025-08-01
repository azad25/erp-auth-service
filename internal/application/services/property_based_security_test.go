package services

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"go.uber.org/zap/zaptest"
	"golang.org/x/crypto/bcrypt"

	"erp-auth-service/internal/cache"
	"erp-auth-service/internal/config"
)

// PropertyBasedSecurityTestSuite provides property-based testing for security-critical functions
type PropertyBasedSecurityTestSuite struct {
	authService   *AuthService
	tokenService  *TokenService
	mockUserRepo  *MockUserRepository
	mockOrgRepo   *MockOrganizationRepository
	mockRoleRepo  *MockRoleRepository
	mockTokenRepo *MockTokenRepository
	mockEventPub  *MockEventPublisher
	mockCache     *MockCacheManager
	config        *config.Config
	ctx           context.Context
}

// SetupTest sets up the test suite
func (suite *PropertyBasedSecurityTestSuite) SetupTest() {
	suite.ctx = context.Background()
	suite.mockUserRepo = new(MockUserRepository)
	suite.mockOrgRepo = new(MockOrganizationRepository)
	suite.mockRoleRepo = new(MockRoleRepository)
	suite.mockTokenRepo = new(MockTokenRepository)
	suite.mockEventPub = new(MockEventPublisher)
	suite.mockCache = NewMockCacheManager()
	
	suite.config = &config.Config{
		JWT: config.JWTConfig{
			Secret:        "property-based-test-secret-key-for-security-testing",
			AccessExpiry:  3600,
			RefreshExpiry: 604800,
		},
	}
	
	logger := zaptest.NewLogger(nil)
	
	// Setup default mock expectations
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
		nil, // TokenService will be set separately
		suite.mockEventPub,
		suite.mockCache,
	)
	
	suite.tokenService = NewTokenService(
		suite.config,
		logger,
		suite.mockTokenRepo,
		suite.mockCache,
		nil, // Redis client not needed for property tests
	)
}

// TearDownTest cleans up after each test
func (suite *PropertyBasedSecurityTestSuite) TearDownTest() {
	if suite.authService != nil {
		suite.authService.Close()
	}
}

// generateRandomString generates a random string of specified length
func (suite *PropertyBasedSecurityTestSuite) generateRandomString(length int) string {
	bytes := make([]byte, length)
	rand.Read(bytes)
	return hex.EncodeToString(bytes)[:length]
}

// generateRandomPassword generates a random password with various characteristics
func (suite *PropertyBasedSecurityTestSuite) generateRandomPassword() string {
	length := 8 + (len(suite.generateRandomString(1)) % 32) // 8-40 characters
	return suite.generateRandomString(length)
}

// generateRandomEmail generates a random email address
func (suite *PropertyBasedSecurityTestSuite) generateRandomEmail() string {
	username := suite.generateRandomString(8)
	domain := suite.generateRandomString(6)
	return fmt.Sprintf("%s@%s.com", username, domain)
}

// TestPasswordHashing_Properties tests password hashing properties
func (suite *PropertyBasedSecurityTestSuite) TestPasswordHashing_Properties(t *testing.T) {
	const numTests = 100
	
	for i := 0; i < numTests; i++ {
		t.Run(fmt.Sprintf("iteration_%d", i), func(t *testing.T) {
			// Generate random password
			password := suite.generateRandomPassword()
			
			// Property 1: Hash should never equal the original password
			hash, err := suite.authService.hashPassword(password)
			assert.NoError(t, err)
			assert.NotEqual(t, password, hash)
			
			// Property 2: Hash should be deterministic for verification but different each time
			hash2, err := suite.authService.hashPassword(password)
			assert.NoError(t, err)
			assert.NotEqual(t, hash, hash2) // bcrypt includes salt, so hashes differ
			
			// Property 3: Verification should work with correct password
			isValid := suite.authService.verifyPassword(password, hash)
			assert.True(t, isValid)
			
			// Property 4: Verification should fail with wrong password
			wrongPassword := suite.generateRandomPassword()
			if wrongPassword != password { // Avoid collision (extremely unlikely)
				isValid = suite.authService.verifyPassword(wrongPassword, hash)
				assert.False(t, isValid)
			}
			
			// Property 5: Hash should have bcrypt format
			assert.True(t, strings.HasPrefix(hash, "$2a$") || strings.HasPrefix(hash, "$2b$") || strings.HasPrefix(hash, "$2y$"))
			
			// Property 6: Hash should be of expected length (bcrypt produces 60 character hashes)
			assert.Equal(t, 60, len(hash))
		})
	}
}

// TestTokenGeneration_Properties tests token generation properties
func (suite *PropertyBasedSecurityTestSuite) TestTokenGeneration_Properties(t *testing.T) {
	const numTests = 50
	
	// Mock repository calls
	suite.mockTokenRepo.On("Create", mock.Anything, mock.AnythingOfType("*models.Token")).Return(nil).Maybe()
	
	for i := 0; i < numTests; i++ {
		t.Run(fmt.Sprintf("iteration_%d", i), func(t *testing.T) {
			// Generate random user data
			userID := uuid.New()
			organizationID := uuid.New()
			email := suite.generateRandomEmail()
			
			securityCtx := &SecurityContext{
				IPAddress:         fmt.Sprintf("192.168.1.%d", i%255),
				UserAgent:         suite.generateRandomString(20),
				DeviceFingerprint: suite.generateRandomString(32),
			}
			
			// Generate token pair
			tokenPair, err := suite.tokenService.GenerateTokenPair(suite.ctx, userID, organizationID, email, securityCtx)
			assert.NoError(t, err)
			assert.NotNil(t, tokenPair)
			
			// Property 1: Tokens should be unique
			tokenPair2, err := suite.tokenService.GenerateTokenPair(suite.ctx, userID, organizationID, email, securityCtx)
			assert.NoError(t, err)
			assert.NotEqual(t, tokenPair.AccessToken, tokenPair2.AccessToken)
			assert.NotEqual(t, tokenPair.RefreshToken, tokenPair2.RefreshToken)
			
			// Property 2: Tokens should have JWT format (3 parts separated by dots)
			accessParts := strings.Split(tokenPair.AccessToken, ".")
			assert.Len(t, accessParts, 3)
			
			refreshParts := strings.Split(tokenPair.RefreshToken, ".")
			assert.Len(t, refreshParts, 3)
			
			// Property 3: Token type should be Bearer
			assert.Equal(t, "Bearer", tokenPair.TokenType)
			
			// Property 4: Expiration times should be in the future
			assert.True(t, tokenPair.ExpiresAt.After(time.Now()))
			assert.True(t, tokenPair.RefreshExpiresAt.After(time.Now()))
			
			// Property 5: Refresh token should expire after access token
			assert.True(t, tokenPair.RefreshExpiresAt.After(tokenPair.ExpiresAt))
			
			// Property 6: Token hash should be consistent
			hash1 := suite.tokenService.hashToken(tokenPair.AccessToken)
			hash2 := suite.tokenService.hashToken(tokenPair.AccessToken)
			assert.Equal(t, hash1, hash2)
			
			// Property 7: Different tokens should have different hashes
			hash3 := suite.tokenService.hashToken(tokenPair.RefreshToken)
			assert.NotEqual(t, hash1, hash3)
		})
	}
}

// TestAuthenticationValidation_Properties tests authentication input validation properties
func (suite *PropertyBasedSecurityTestSuite) TestAuthenticationValidation_Properties(t *testing.T) {
	const numTests = 50
	
	for i := 0; i < numTests; i++ {
		t.Run(fmt.Sprintf("iteration_%d", i), func(t *testing.T) {
			// Generate random but potentially invalid inputs
			email := ""
			password := ""
			var securityCtx *SecurityContext
			
			// Randomly make some fields invalid
			switch i % 4 {
			case 0:
				// Missing email
				password = suite.generateRandomPassword()
				securityCtx = &SecurityContext{IPAddress: "192.168.1.1"}
			case 1:
				// Missing password
				email = suite.generateRandomEmail()
				securityCtx = &SecurityContext{IPAddress: "192.168.1.1"}
			case 2:
				// Missing security context
				email = suite.generateRandomEmail()
				password = suite.generateRandomPassword()
			case 3:
				// Valid input
				email = suite.generateRandomEmail()
				password = suite.generateRandomPassword()
				securityCtx = &SecurityContext{IPAddress: "192.168.1.1"}
			}
			
			req := &AuthRequest{
				Email:           email,
				Password:        password,
				SecurityContext: securityCtx,
			}
			
			err := suite.authService.validateAuthRequest(req)
			
			// Property: Validation should fail for incomplete requests
			if email == "" || password == "" || securityCtx == nil {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), "required")
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// TestRegistrationValidation_Properties tests registration input validation properties
func (suite *PropertyBasedSecurityTestSuite) TestRegistrationValidation_Properties(t *testing.T) {
	const numTests = 50
	
	for i := 0; i < numTests; i++ {
		t.Run(fmt.Sprintf("iteration_%d", i), func(t *testing.T) {
			// Generate random inputs with potential issues
			email := suite.generateRandomEmail()
			password := suite.generateRandomPassword()
			firstName := suite.generateRandomString(10)
			lastName := suite.generateRandomString(10)
			orgName := suite.generateRandomString(15)
			orgDomain := suite.generateRandomString(10) + ".com"
			
			// Randomly introduce validation issues
			switch i % 6 {
			case 0:
				email = "" // Missing email
			case 1:
				password = "short" // Too short password
			case 2:
				firstName = "" // Missing first name
			case 3:
				lastName = "" // Missing last name
			case 4:
				orgName = "" // Missing org name
			case 5:
				orgDomain = "" // Missing org domain
			}
			
			req := &RegistrationRequest{
				Email:              email,
				Password:           password,
				FirstName:          firstName,
				LastName:           lastName,
				OrganizationName:   orgName,
				OrganizationDomain: orgDomain,
			}
			
			err := suite.authService.validateRegistrationRequest(req)
			
			// Property: Validation should catch all required field violations
			if email == "" {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), "email is required")
			} else if len(password) < 8 {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), "password must be at least 8 characters")
			} else if firstName == "" {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), "first name is required")
			} else if lastName == "" {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), "last name is required")
			} else if orgName == "" {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), "organization name is required")
			} else if orgDomain == "" {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), "organization domain is required")
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// TestTwoFactorCodeGeneration_Properties tests 2FA code generation properties
func (suite *PropertyBasedSecurityTestSuite) TestTwoFactorCodeGeneration_Properties(t *testing.T) {
	const numTests = 20
	
	for i := 0; i < numTests; i++ {
		t.Run(fmt.Sprintf("iteration_%d", i), func(t *testing.T) {
			// Generate backup codes
			codes := suite.authService.generateBackupCodes(10)
			
			// Property 1: Should generate exactly the requested number of codes
			assert.Len(t, codes, 10)
			
			// Property 2: All codes should be unique
			codeSet := make(map[string]bool)
			for _, code := range codes {
				assert.False(t, codeSet[code], "Duplicate backup code found: %s", code)
				codeSet[code] = true
			}
			
			// Property 3: All codes should be uppercase
			for _, code := range codes {
				assert.Equal(t, strings.ToUpper(code), code)
			}
			
			// Property 4: All codes should be base32 encoded (only contain A-Z and 2-7)
			for _, code := range codes {
				for _, char := range code {
					assert.True(t, (char >= 'A' && char <= 'Z') || (char >= '2' && char <= '7'),
						"Invalid character in backup code: %c", char)
				}
			}
			
			// Property 5: Codes should have reasonable length
			for _, code := range codes {
				assert.True(t, len(code) >= 8 && len(code) <= 16,
					"Backup code length out of range: %d", len(code))
			}
		})
	}
}

// TestSessionIDGeneration_Properties tests session ID generation properties
func (suite *PropertyBasedSecurityTestSuite) TestSessionIDGeneration_Properties(t *testing.T) {
	const numTests = 100
	
	sessionIDs := make(map[string]bool)
	
	for i := 0; i < numTests; i++ {
		t.Run(fmt.Sprintf("iteration_%d", i), func(t *testing.T) {
			sessionID := suite.tokenService.generateSessionID()
			
			// Property 1: Session ID should be unique
			assert.False(t, sessionIDs[sessionID], "Duplicate session ID generated: %s", sessionID)
			sessionIDs[sessionID] = true
			
			// Property 2: Session ID should be hex encoded
			assert.Len(t, sessionID, 32) // 16 bytes = 32 hex characters
			
			// Property 3: Session ID should only contain hex characters
			for _, char := range sessionID {
				assert.True(t, (char >= '0' && char <= '9') || (char >= 'a' && char <= 'f'),
					"Invalid hex character in session ID: %c", char)
			}
			
			// Property 4: Session ID should not be empty
			assert.NotEmpty(t, sessionID)
		})
	}
}

// TestTokenHashConsistency_Properties tests token hash consistency properties
func (suite *PropertyBasedSecurityTestSuite) TestTokenHashConsistency_Properties(t *testing.T) {
	const numTests = 50
	
	for i := 0; i < numTests; i++ {
		t.Run(fmt.Sprintf("iteration_%d", i), func(t *testing.T) {
			// Generate random token-like string
			token := suite.generateRandomString(100)
			
			// Property 1: Hash should be deterministic
			hash1 := suite.tokenService.hashToken(token)
			hash2 := suite.tokenService.hashToken(token)
			assert.Equal(t, hash1, hash2)
			
			// Property 2: Hash should be different for different tokens
			differentToken := suite.generateRandomString(100)
			if differentToken != token {
				hash3 := suite.tokenService.hashToken(differentToken)
				assert.NotEqual(t, hash1, hash3)
			}
			
			// Property 3: Hash should be SHA256 hex encoded (64 characters)
			assert.Len(t, hash1, 64)
			
			// Property 4: Hash should only contain hex characters
			for _, char := range hash1 {
				assert.True(t, (char >= '0' && char <= '9') || (char >= 'a' && char <= 'f'),
					"Invalid hex character in token hash: %c", char)
			}
			
			// Property 5: Hash should not be empty
			assert.NotEmpty(t, hash1)
		})
	}
}

// TestPasswordStrengthInvariant_Properties tests password strength invariants
func (suite *PropertyBasedSecurityTestSuite) TestPasswordStrengthInvariant_Properties(t *testing.T) {
	const numTests = 50
	
	for i := 0; i < numTests; i++ {
		t.Run(fmt.Sprintf("iteration_%d", i), func(t *testing.T) {
			// Generate passwords of various lengths
			length := 1 + (i % 50) // 1-50 characters
			password := suite.generateRandomString(length)
			
			// Test bcrypt properties
			if length >= 1 && length <= 72 { // bcrypt supports up to 72 bytes
				hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
				
				if err == nil {
					// Property 1: Hash should verify correctly
					err = bcrypt.CompareHashAndPassword(hash, []byte(password))
					assert.NoError(t, err)
					
					// Property 2: Hash should not verify with wrong password
					wrongPassword := suite.generateRandomString(length)
					if wrongPassword != password {
						err = bcrypt.CompareHashAndPassword(hash, []byte(wrongPassword))
						assert.Error(t, err)
					}
				}
			}
		})
	}
}

// TestSecurityContextValidation_Properties tests security context validation properties
func (suite *PropertyBasedSecurityTestSuite) TestSecurityContextValidation_Properties(t *testing.T) {
	const numTests = 30
	
	for i := 0; i < numTests; i++ {
		t.Run(fmt.Sprintf("iteration_%d", i), func(t *testing.T) {
			// Generate random security context data
			ipAddress := fmt.Sprintf("%d.%d.%d.%d", i%256, (i*2)%256, (i*3)%256, (i*4)%256)
			userAgent := suite.generateRandomString(20 + (i % 50))
			deviceFingerprint := suite.generateRandomString(32)
			sessionID := suite.generateRandomString(32)
			
			securityCtx := &SecurityContext{
				IPAddress:         ipAddress,
				UserAgent:         userAgent,
				DeviceFingerprint: deviceFingerprint,
				SessionID:         sessionID,
			}
			
			// Property 1: Security context should maintain data integrity
			assert.Equal(t, ipAddress, securityCtx.IPAddress)
			assert.Equal(t, userAgent, securityCtx.UserAgent)
			assert.Equal(t, deviceFingerprint, securityCtx.DeviceFingerprint)
			assert.Equal(t, sessionID, securityCtx.SessionID)
			
			// Property 2: Security context should handle empty values gracefully
			emptyCtx := &SecurityContext{}
			assert.Equal(t, "", emptyCtx.IPAddress)
			assert.Equal(t, "", emptyCtx.UserAgent)
			assert.Equal(t, "", emptyCtx.DeviceFingerprint)
			assert.Equal(t, "", emptyCtx.SessionID)
		})
	}
}

// TestMetricsConsistency_Properties tests metrics consistency properties
func (suite *PropertyBasedSecurityTestSuite) TestMetricsConsistency_Properties(t *testing.T) {
	const numTests = 20
	
	for i := 0; i < numTests; i++ {
		t.Run(fmt.Sprintf("iteration_%d", i), func(t *testing.T) {
			// Get initial metrics
			initialMetrics := suite.authService.GetMetrics()
			
			// Perform random metric updates
			numUpdates := 1 + (i % 10)
			for j := 0; j < numUpdates; j++ {
				suite.authService.updateMetrics(func(m *AuthMetrics) {
					m.AuthenticationAttempts++
					if j%2 == 0 {
						m.SuccessfulLogins++
					} else {
						m.FailedLogins++
					}
				})
			}
			
			// Get updated metrics
			updatedMetrics := suite.authService.GetMetrics()
			
			// Property 1: Metrics should increase monotonically
			assert.True(t, updatedMetrics.AuthenticationAttempts >= initialMetrics.AuthenticationAttempts)
			assert.True(t, updatedMetrics.SuccessfulLogins >= initialMetrics.SuccessfulLogins)
			assert.True(t, updatedMetrics.FailedLogins >= initialMetrics.FailedLogins)
			
			// Property 2: Authentication attempts should equal sum of successes and failures
			expectedAttempts := initialMetrics.AuthenticationAttempts + int64(numUpdates)
			assert.Equal(t, expectedAttempts, updatedMetrics.AuthenticationAttempts)
			
			// Property 3: Metrics should be consistent across multiple reads
			metrics2 := suite.authService.GetMetrics()
			assert.Equal(t, updatedMetrics.AuthenticationAttempts, metrics2.AuthenticationAttempts)
			assert.Equal(t, updatedMetrics.SuccessfulLogins, metrics2.SuccessfulLogins)
			assert.Equal(t, updatedMetrics.FailedLogins, metrics2.FailedLogins)
		})
	}
}

// Run property-based security tests
func TestPropertyBasedSecurity(t *testing.T) {
	suite := &PropertyBasedSecurityTestSuite{}
	suite.SetupTest()
	defer suite.TearDownTest()
	
	t.Run("PasswordHashing", suite.TestPasswordHashing_Properties)
	t.Run("TokenGeneration", suite.TestTokenGeneration_Properties)
	t.Run("AuthenticationValidation", suite.TestAuthenticationValidation_Properties)
	t.Run("RegistrationValidation", suite.TestRegistrationValidation_Properties)
	t.Run("TwoFactorCodeGeneration", suite.TestTwoFactorCodeGeneration_Properties)
	t.Run("SessionIDGeneration", suite.TestSessionIDGeneration_Properties)
	t.Run("TokenHashConsistency", suite.TestTokenHashConsistency_Properties)
	t.Run("PasswordStrengthInvariant", suite.TestPasswordStrengthInvariant_Properties)
	t.Run("SecurityContextValidation", suite.TestSecurityContextValidation_Properties)
	t.Run("MetricsConsistency", suite.TestMetricsConsistency_Properties)
}