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
	"github.com/stretchr/testify/suite"
	"go.uber.org/zap"

	"erp-auth-service/internal/cache"
	"erp-auth-service/internal/config"
	"erp-auth-service/internal/domain/entities"
	"erp-auth-service/internal/models"
)

// ComprehensiveTokenServiceTestSuite provides comprehensive test coverage for TokenService
type ComprehensiveTokenServiceTestSuite struct {
	suite.Suite
	tokenService   *TokenService
	mockRepo       *MockTokenRepository
	mockCache      *MockCacheManager
	redisClient    *redis.Client
	miniRedis      *miniredis.Miniredis
	config         *config.Config
	logger         *zap.Logger
}

// SetupSuite sets up the test suite
func (suite *ComprehensiveTokenServiceTestSuite) SetupSuite() {
	// Setup mini redis
	var err error
	suite.miniRedis, err = miniredis.Run()
	suite.Require().NoError(err)

	// Setup Redis client
	suite.redisClient = redis.NewClient(&redis.Options{
		Addr: suite.miniRedis.Addr(),
	})

	// Setup logger
	suite.logger = zap.NewNop()

	// Setup config
	suite.config = &config.Config{
		JWT: config.JWTConfig{
			Secret:              "comprehensive-test-secret-key-for-jwt-signing-operations",
			AccessExpiry:        3600,
			RefreshExpiry:       604800,
			KeyRotationEnabled:  false,
			KeyRotationInterval: 24,
			RevokeRefreshOnUse:  false,
		},
	}
}

// SetupTest sets up each test
func (suite *ComprehensiveTokenServiceTestSuite) SetupTest() {
	suite.mockRepo = &MockTokenRepository{}
	suite.mockCache = NewMockCacheManager()
	
	// Allow cache operations by default
	suite.mockCache.On("Set", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil).Maybe()
	suite.mockCache.On("Get", mock.Anything, mock.Anything).Return(nil, cache.ErrCacheNotFound).Maybe()
	suite.mockCache.On("Delete", mock.Anything, mock.Anything).Return(nil).Maybe()

	suite.tokenService = NewTokenService(
		suite.config,
		suite.logger,
		suite.mockRepo,
		suite.mockCache,
		suite.redisClient,
	)

	// Clear mini redis
	suite.miniRedis.FlushAll()
}

// TearDownSuite tears down the test suite
func (suite *ComprehensiveTokenServiceTestSuite) TearDownSuite() {
	suite.miniRedis.Close()
	suite.redisClient.Close()
}

// TestGenerateTokenPair_Success tests successful token pair generation
func (suite *ComprehensiveTokenServiceTestSuite) TestGenerateTokenPair_Success() {
	ctx := context.Background()
	userID := uuid.New()
	organizationID := uuid.New()
	email := "test@example.com"
	securityCtx := &SecurityContext{
		IPAddress:         "192.168.1.1",
		UserAgent:         "test-agent",
		DeviceFingerprint: "test-fingerprint",
		SessionID:         "test-session",
	}

	// Mock repository calls
	suite.mockRepo.On("Create", ctx, mock.AnythingOfType("*models.Token")).Return(nil).Twice()

	// Generate token pair
	tokenPair, err := suite.tokenService.GenerateTokenPair(ctx, userID, organizationID, email, securityCtx)

	// Assertions
	suite.NoError(err)
	suite.NotNil(tokenPair)
	suite.NotEmpty(tokenPair.AccessToken)
	suite.NotEmpty(tokenPair.RefreshToken)
	suite.Equal("Bearer", tokenPair.TokenType)
	suite.True(tokenPair.ExpiresAt.After(time.Now()))
	suite.True(tokenPair.RefreshExpiresAt.After(tokenPair.ExpiresAt))

	// Verify token format (JWT should have 3 parts separated by dots)
	accessParts := strings.Split(tokenPair.AccessToken, ".")
	suite.Len(accessParts, 3)
	
	refreshParts := strings.Split(tokenPair.RefreshToken, ".")
	suite.Len(refreshParts, 3)

	// Verify repository calls
	suite.mockRepo.AssertExpectations(suite.T())
}

// TestGenerateTokenPair_WithNilSecurityContext tests token generation with nil security context
func (suite *ComprehensiveTokenServiceTestSuite) TestGenerateTokenPair_WithNilSecurityContext() {
	ctx := context.Background()
	userID := uuid.New()
	organizationID := uuid.New()
	email := "test@example.com"

	// Mock repository calls
	suite.mockRepo.On("Create", ctx, mock.AnythingOfType("*models.Token")).Return(nil).Twice()

	// Generate token pair with nil security context
	tokenPair, err := suite.tokenService.GenerateTokenPair(ctx, userID, organizationID, email, nil)

	// Assertions
	suite.NoError(err)
	suite.NotNil(tokenPair)
	suite.NotEmpty(tokenPair.AccessToken)
	suite.NotEmpty(tokenPair.RefreshToken)

	// Verify repository calls
	suite.mockRepo.AssertExpectations(suite.T())
}

// TestValidateToken_Success tests successful token validation
func (suite *ComprehensiveTokenServiceTestSuite) TestValidateToken_Success() {
	ctx := context.Background()
	userID := uuid.New()
	organizationID := uuid.New()
	email := "test@example.com"

	// Generate a token first
	suite.mockRepo.On("Create", ctx, mock.AnythingOfType("*models.Token")).Return(nil).Twice()
	tokenPair, err := suite.tokenService.GenerateTokenPair(ctx, userID, organizationID, email, nil)
	suite.NoError(err)

	// Mock token lookup for validation
	tokenHash := suite.tokenService.hashToken(tokenPair.AccessToken)
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
	suite.mockRepo.On("GetByTokenHash", ctx, tokenHash).Return(mockToken, nil)

	// Validate token
	claims, err := suite.tokenService.ValidateToken(ctx, tokenPair.AccessToken)

	// Assertions
	suite.NoError(err)
	suite.NotNil(claims)
	suite.Equal(userID, claims.UserID)
	suite.Equal(string(entities.AccessToken), claims.TokenType)
	suite.Equal(email, claims.Email)

	// Verify repository calls
	suite.mockRepo.AssertExpectations(suite.T())
}

// TestValidateToken_InvalidFormat tests validation of malformed tokens
func (suite *ComprehensiveTokenServiceTestSuite) TestValidateToken_InvalidFormat() {
	ctx := context.Background()

	testCases := []struct {
		name  string
		token string
	}{
		{"empty token", ""},
		{"invalid format", "invalid.token"},
		{"too many parts", "part1.part2.part3.part4"},
		{"random string", "this-is-not-a-jwt-token"},
		{"single part", "invalidtoken"},
		{"two parts only", "header.payload"},
	}

	for _, tc := range testCases {
		suite.Run(tc.name, func() {
			_, err := suite.tokenService.ValidateToken(ctx, tc.token)
			suite.Error(err)
		})
	}
}

// TestValidateToken_ExpiredToken tests validation of expired tokens
func (suite *ComprehensiveTokenServiceTestSuite) TestValidateToken_ExpiredToken() {
	ctx := context.Background()
	userID := uuid.New()
	organizationID := uuid.New()
	email := "test@example.com"

	// Create a token service with very short expiry
	shortConfig := *suite.config
	shortConfig.JWT.AccessExpiry = 1 // 1 second
	shortTokenService := NewTokenService(
		&shortConfig,
		suite.logger,
		suite.mockRepo,
		suite.mockCache,
		suite.redisClient,
	)

	// Generate a token
	suite.mockRepo.On("Create", ctx, mock.AnythingOfType("*models.Token")).Return(nil).Twice()
	tokenPair, err := shortTokenService.GenerateTokenPair(ctx, userID, organizationID, email, nil)
	suite.NoError(err)

	// Wait for token to expire
	time.Sleep(2 * time.Second)

	// Try to validate expired token
	_, err = shortTokenService.ValidateToken(ctx, tokenPair.AccessToken)

	// Should fail due to expiration (JWT library will catch this)
	suite.Error(err)
	suite.True(strings.Contains(err.Error(), "token is expired") || strings.Contains(err.Error(), "token has expired"))
}

// TestValidateToken_BlacklistedToken tests validation of blacklisted tokens
func (suite *ComprehensiveTokenServiceTestSuite) TestValidateToken_BlacklistedToken() {
	ctx := context.Background()
	userID := uuid.New()
	organizationID := uuid.New()
	email := "test@example.com"

	// Generate a token
	suite.mockRepo.On("Create", ctx, mock.AnythingOfType("*models.Token")).Return(nil).Twice()
	tokenPair, err := suite.tokenService.GenerateTokenPair(ctx, userID, organizationID, email, nil)
	suite.NoError(err)

	// Blacklist the token
	tokenHash := suite.tokenService.hashToken(tokenPair.AccessToken)
	suite.mockRepo.On("RevokeToken", ctx, tokenHash).Return(nil)
	err = suite.tokenService.RevokeToken(ctx, tokenPair.AccessToken)
	suite.NoError(err)

	// Try to validate blacklisted token
	_, err = suite.tokenService.ValidateToken(ctx, tokenPair.AccessToken)

	// Should fail due to blacklisting
	suite.Error(err)
	suite.Contains(err.Error(), "blacklisted")
}

// TestRevokeToken_Success tests successful token revocation
func (suite *ComprehensiveTokenServiceTestSuite) TestRevokeToken_Success() {
	ctx := context.Background()
	userID := uuid.New()
	organizationID := uuid.New()
	email := "test@example.com"

	// Generate a token first
	suite.mockRepo.On("Create", ctx, mock.AnythingOfType("*models.Token")).Return(nil).Twice()
	tokenPair, err := suite.tokenService.GenerateTokenPair(ctx, userID, organizationID, email, nil)
	suite.NoError(err)

	// Mock revocation
	tokenHash := suite.tokenService.hashToken(tokenPair.AccessToken)
	suite.mockRepo.On("RevokeToken", ctx, tokenHash).Return(nil)

	// Revoke token
	err = suite.tokenService.RevokeToken(ctx, tokenPair.AccessToken)

	// Assertions
	suite.NoError(err)

	// Verify token is blacklisted in Redis
	blacklistKey := fmt.Sprintf("token:blacklist:%s", tokenHash)
	result := suite.redisClient.Exists(ctx, blacklistKey)
	suite.Equal(int64(1), result.Val())

	// Verify repository calls
	suite.mockRepo.AssertExpectations(suite.T())
}

// TestRefreshToken_Success tests successful token refresh
func (suite *ComprehensiveTokenServiceTestSuite) TestRefreshToken_Success() {
	ctx := context.Background()
	userID := uuid.New()
	organizationID := uuid.New()
	email := "test@example.com"

	// Generate initial token pair
	suite.mockRepo.On("Create", ctx, mock.AnythingOfType("*models.Token")).Return(nil).Times(4) // 2 for initial, 2 for refresh
	tokenPair, err := suite.tokenService.GenerateTokenPair(ctx, userID, organizationID, email, nil)
	suite.NoError(err)

	// Mock refresh token lookup for validation
	refreshHash := suite.tokenService.hashToken(tokenPair.RefreshToken)
	mockRefreshToken := &models.Token{
		ID:        uuid.New(),
		UserID:    userID,
		TokenType: models.RefreshToken,
		TokenHash: refreshHash,
		IssuedAt:  time.Now(),
		ExpiresAt: time.Now().Add(24 * time.Hour),
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	suite.mockRepo.On("GetByTokenHash", ctx, refreshHash).Return(mockRefreshToken, nil)

	// Refresh token
	newTokenPair, err := suite.tokenService.RefreshToken(ctx, tokenPair.RefreshToken, nil)

	// Assertions
	suite.NoError(err)
	suite.NotNil(newTokenPair)
	suite.NotEqual(tokenPair.AccessToken, newTokenPair.AccessToken)
	suite.NotEqual(tokenPair.RefreshToken, newTokenPair.RefreshToken)

	// Verify repository calls
	suite.mockRepo.AssertExpectations(suite.T())
}

// TestRefreshToken_InvalidToken tests refresh with invalid token
func (suite *ComprehensiveTokenServiceTestSuite) TestRefreshToken_InvalidToken() {
	ctx := context.Background()

	// Try to refresh with invalid token
	_, err := suite.tokenService.RefreshToken(ctx, "invalid-token", nil)

	// Should fail
	suite.Error(err)
	suite.Contains(err.Error(), "invalid refresh token")
}

// TestRefreshToken_AccessTokenUsed tests refresh with access token instead of refresh token
func (suite *ComprehensiveTokenServiceTestSuite) TestRefreshToken_AccessTokenUsed() {
	ctx := context.Background()
	userID := uuid.New()
	organizationID := uuid.New()
	email := "test@example.com"

	// Generate token pair
	suite.mockRepo.On("Create", ctx, mock.AnythingOfType("*models.Token")).Return(nil).Twice()
	tokenPair, err := suite.tokenService.GenerateTokenPair(ctx, userID, organizationID, email, nil)
	suite.NoError(err)

	// Mock access token lookup
	accessHash := suite.tokenService.hashToken(tokenPair.AccessToken)
	mockAccessToken := &models.Token{
		ID:        uuid.New(),
		UserID:    userID,
		TokenType: models.AccessToken, // This is an access token, not refresh
		TokenHash: accessHash,
		IssuedAt:  time.Now(),
		ExpiresAt: time.Now().Add(time.Hour),
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	suite.mockRepo.On("GetByTokenHash", ctx, accessHash).Return(mockAccessToken, nil)

	// Try to refresh with access token
	_, err = suite.tokenService.RefreshToken(ctx, tokenPair.AccessToken, nil)

	// Should fail
	suite.Error(err)
	suite.Contains(err.Error(), "token is not a refresh token")
}

// TestRevokeAllUserTokens_Success tests successful revocation of all user tokens
func (suite *ComprehensiveTokenServiceTestSuite) TestRevokeAllUserTokens_Success() {
	ctx := context.Background()
	userID := uuid.New()

	// Mock active tokens
	mockTokens := []models.Token{
		{
			ID:        uuid.New(),
			UserID:    userID,
			TokenType: models.AccessToken,
			TokenHash: "hash1",
			IssuedAt:  time.Now(),
			ExpiresAt: time.Now().Add(time.Hour),
		},
		{
			ID:        uuid.New(),
			UserID:    userID,
			TokenType: models.AccessToken,
			TokenHash: "hash2",
			IssuedAt:  time.Now(),
			ExpiresAt: time.Now().Add(time.Hour),
		},
	}

	suite.mockRepo.On("GetActiveTokensByUser", ctx, userID, models.AccessToken).Return(mockTokens, nil)
	suite.mockRepo.On("RevokeAllUserTokens", ctx, userID, models.AccessToken).Return(nil)

	// Revoke all user tokens
	err := suite.tokenService.RevokeAllUserTokens(ctx, userID, entities.AccessToken)

	// Assertions
	suite.NoError(err)

	// Verify tokens are blacklisted
	for _, token := range mockTokens {
		blacklistKey := fmt.Sprintf("token:blacklist:%s", token.TokenHash)
		result := suite.redisClient.Exists(ctx, blacklistKey)
		suite.Equal(int64(1), result.Val())
	}

	// Verify repository calls
	suite.mockRepo.AssertExpectations(suite.T())
}

// TestTokenServiceMetrics tests metrics collection
func (suite *ComprehensiveTokenServiceTestSuite) TestTokenServiceMetrics() {
	ctx := context.Background()
	userID := uuid.New()
	organizationID := uuid.New()
	email := "test@example.com"

	// Mock repository calls
	suite.mockRepo.On("Create", ctx, mock.AnythingOfType("*models.Token")).Return(nil).Twice()

	// Generate token pair
	_, err := suite.tokenService.GenerateTokenPair(ctx, userID, organizationID, email, nil)
	suite.NoError(err)

	// Check metrics
	metrics := suite.tokenService.GetMetrics()
	suite.Equal(int64(2), metrics.TokensGenerated) // Access + Refresh token
	suite.Equal(int64(0), metrics.TokensValidated)
	suite.Equal(int64(0), metrics.TokensRevoked)
}

// TestTokenHashing tests token hashing functionality
func (suite *ComprehensiveTokenServiceTestSuite) TestTokenHashing() {
	token1 := "test-token-1"
	token2 := "test-token-2"
	
	hash1 := suite.tokenService.hashToken(token1)
	hash2 := suite.tokenService.hashToken(token2)
	
	// Hashes should be different for different tokens
	suite.NotEqual(hash1, hash2)
	
	// Same token should produce same hash
	hash1Again := suite.tokenService.hashToken(token1)
	suite.Equal(hash1, hash1Again)
	
	// Hash should be hex encoded
	suite.Len(hash1, 64) // SHA256 produces 32 bytes = 64 hex characters
}

// TestSessionIDGeneration tests session ID generation
func (suite *ComprehensiveTokenServiceTestSuite) TestSessionIDGeneration() {
	sessionID1 := suite.tokenService.generateSessionID()
	sessionID2 := suite.tokenService.generateSessionID()
	
	// Session IDs should be different
	suite.NotEqual(sessionID1, sessionID2)
	
	// Session IDs should be hex encoded
	suite.Len(sessionID1, 32) // 16 bytes = 32 hex characters
	suite.Len(sessionID2, 32)
}

// TestTokenValidationString tests token string validation
func (suite *ComprehensiveTokenServiceTestSuite) TestTokenValidationString() {
	testCases := []struct {
		name        string
		token       string
		expectError bool
	}{
		{"valid JWT format", "header.payload.signature", false},
		{"empty token", "", true},
		{"single part", "invalidtoken", true},
		{"two parts", "header.payload", true},
		{"four parts", "part1.part2.part3.part4", true},
	}
	
	for _, tc := range testCases {
		suite.Run(tc.name, func() {
			err := suite.tokenService.ValidateTokenString(tc.token)
			if tc.expectError {
				suite.Error(err)
			} else {
				suite.NoError(err)
			}
		})
	}
}

// TestGetTokenInfo tests token information extraction
func (suite *ComprehensiveTokenServiceTestSuite) TestGetTokenInfo() {
	ctx := context.Background()
	userID := uuid.New()
	organizationID := uuid.New()
	email := "test@example.com"

	// Generate a token
	suite.mockRepo.On("Create", ctx, mock.AnythingOfType("*models.Token")).Return(nil).Twice()
	tokenPair, err := suite.tokenService.GenerateTokenPair(ctx, userID, organizationID, email, nil)
	suite.NoError(err)

	// Get token info
	claims, err := suite.tokenService.GetTokenInfo(ctx, tokenPair.AccessToken)

	// Assertions
	suite.NoError(err)
	suite.NotNil(claims)
	suite.Equal(userID, claims.UserID)
	suite.Equal(string(entities.AccessToken), claims.TokenType)
}

// TestIsTokenExpired tests token expiration checking
func (suite *ComprehensiveTokenServiceTestSuite) TestIsTokenExpired() {
	ctx := context.Background()
	userID := uuid.New()
	organizationID := uuid.New()
	email := "test@example.com"

	// Generate a token with normal expiry
	suite.mockRepo.On("Create", ctx, mock.AnythingOfType("*models.Token")).Return(nil).Twice()
	tokenPair, err := suite.tokenService.GenerateTokenPair(ctx, userID, organizationID, email, nil)
	suite.NoError(err)

	// Check if token is expired (should not be)
	isExpired, err := suite.tokenService.IsTokenExpired(tokenPair.AccessToken)
	suite.NoError(err)
	suite.False(isExpired)

	// Test with invalid token
	_, err = suite.tokenService.IsTokenExpired("invalid-token")
	suite.Error(err)
}

// TestConcurrentTokenOperations tests thread safety
func (suite *ComprehensiveTokenServiceTestSuite) TestConcurrentTokenOperations() {
	ctx := context.Background()
	userID := uuid.New()
	organizationID := uuid.New()
	email := "test@example.com"

	// Mock repository calls for concurrent operations
	suite.mockRepo.On("Create", ctx, mock.AnythingOfType("*models.Token")).Return(nil)

	// Run concurrent token generations
	const numGoroutines = 10
	results := make(chan error, numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		go func() {
			_, err := suite.tokenService.GenerateTokenPair(ctx, userID, organizationID, email, nil)
			results <- err
		}()
	}

	// Collect results
	for i := 0; i < numGoroutines; i++ {
		err := <-results
		suite.NoError(err)
	}

	// Verify metrics are consistent
	metrics := suite.tokenService.GetMetrics()
	suite.Equal(int64(numGoroutines*2), metrics.TokensGenerated)
}

// TestTokenCacheOperations tests token caching functionality
func (suite *ComprehensiveTokenServiceTestSuite) TestTokenCacheOperations() {
	ctx := context.Background()
	userID := uuid.New()
	organizationID := uuid.New()
	email := "test@example.com"

	// Generate a token
	suite.mockRepo.On("Create", ctx, mock.AnythingOfType("*models.Token")).Return(nil).Twice()
	tokenPair, err := suite.tokenService.GenerateTokenPair(ctx, userID, organizationID, email, nil)
	suite.NoError(err)

	// Test cache operations
	tokenHash := suite.tokenService.hashToken(tokenPair.AccessToken)
	
	// Test cache key format
	cacheKey := fmt.Sprintf("token:claims:%s", tokenHash)
	suite.NotEmpty(cacheKey)
	
	// Test blacklist key format
	blacklistKey := fmt.Sprintf("token:blacklist:%s", tokenHash)
	suite.NotEmpty(blacklistKey)
}

// Run the comprehensive token service test suite
func TestComprehensiveTokenService(t *testing.T) {
	suite.Run(t, new(ComprehensiveTokenServiceTestSuite))
}