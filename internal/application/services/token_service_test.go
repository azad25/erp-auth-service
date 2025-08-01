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







func (m *MockCacheManager) GetCache(name string) cache.Cache {
	args := m.Called(name)
	return args.Get(0).(cache.Cache)
}

func (m *MockCacheManager) RegisterCache(name string, cache cache.Cache) error {
	args := m.Called(name, cache)
	return args.Error(0)
}

func (m *MockCacheManager) SetStrategy(strategy cache.CacheStrategy) {
	m.Called(strategy)
}

func (m *MockCacheManager) GetStrategy() cache.CacheStrategy {
	args := m.Called()
	return args.Get(0).(cache.CacheStrategy)
}

func (m *MockCacheManager) Sync(ctx context.Context) error {
	args := m.Called(ctx)
	return args.Error(0)
}

// TokenServiceTestSuite defines the test suite for TokenService
type TokenServiceTestSuite struct {
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
func (suite *TokenServiceTestSuite) SetupSuite() {
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
			Secret:              "test-secret-key-for-jwt-signing",
			AccessExpiry:        3600,
			RefreshExpiry:       604800,
			KeyRotationEnabled:  false,
			KeyRotationInterval: 24,
			RevokeRefreshOnUse:  false,
		},
	}
}

// SetupTest sets up each test
func (suite *TokenServiceTestSuite) SetupTest() {
	suite.mockRepo = &MockTokenRepository{}
	suite.mockCache = NewMockCacheManager()
	
	// Allow cache operations by default
	suite.mockCache.On("Set", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil)
	suite.mockCache.On("Get", mock.Anything, mock.Anything).Return(nil, cache.ErrCacheNotFound)
	suite.mockCache.On("Delete", mock.Anything, mock.Anything).Return(nil)

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
func (suite *TokenServiceTestSuite) TearDownSuite() {
	suite.miniRedis.Close()
	suite.redisClient.Close()
}

// TestGenerateTokenPair tests token pair generation
func (suite *TokenServiceTestSuite) TestGenerateTokenPair() {
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

	// Verify repository calls
	suite.mockRepo.AssertExpectations(suite.T())
}

// TestValidateToken tests token validation
func (suite *TokenServiceTestSuite) TestValidateToken() {
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

	// Verify repository calls
	suite.mockRepo.AssertExpectations(suite.T())
}

// TestValidateExpiredToken tests validation of expired tokens
func (suite *TokenServiceTestSuite) TestValidateExpiredToken() {
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

// TestRevokeToken tests token revocation
func (suite *TokenServiceTestSuite) TestRevokeToken() {
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

// TestRefreshToken tests token refresh functionality
func (suite *TokenServiceTestSuite) TestRefreshToken() {
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

// TestRevokeAllUserTokens tests revoking all user tokens
func (suite *TokenServiceTestSuite) TestRevokeAllUserTokens() {
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

// TestTokenValidationWithBlacklist tests that blacklisted tokens are rejected
func (suite *TokenServiceTestSuite) TestTokenValidationWithBlacklist() {
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

// TestTokenServiceMetrics tests metrics collection
func (suite *TokenServiceTestSuite) TestTokenServiceMetrics() {
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
}

// TestInvalidTokenFormat tests validation of malformed tokens
func (suite *TokenServiceTestSuite) TestInvalidTokenFormat() {
	ctx := context.Background()

	testCases := []struct {
		name  string
		token string
	}{
		{"empty token", ""},
		{"invalid format", "invalid.token"},
		{"too many parts", "part1.part2.part3.part4"},
		{"random string", "this-is-not-a-jwt-token"},
	}

	for _, tc := range testCases {
		suite.Run(tc.name, func() {
			_, err := suite.tokenService.ValidateToken(ctx, tc.token)
			suite.Error(err)
		})
	}
}

// TestConcurrentTokenOperations tests thread safety
func (suite *TokenServiceTestSuite) TestConcurrentTokenOperations() {
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

// Run the test suite
func TestTokenServiceTestSuite(t *testing.T) {
	suite.Run(t, new(TokenServiceTestSuite))
}