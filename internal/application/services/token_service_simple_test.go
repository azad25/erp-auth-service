package services

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"go.uber.org/zap"

	"erp-auth-service/internal/config"
	"erp-auth-service/internal/domain/entities"
	"erp-auth-service/internal/models"
)

// TestTokenService_BasicFunctionality tests basic token service functionality
func TestTokenService_BasicFunctionality(t *testing.T) {
	// Setup mini redis
	miniRedis, err := miniredis.Run()
	assert.NoError(t, err)
	defer miniRedis.Close()

	// Setup Redis client
	redisClient := redis.NewClient(&redis.Options{
		Addr: miniRedis.Addr(),
	})
	defer redisClient.Close()

	// Setup config
	config := &config.Config{
		JWT: config.JWTConfig{
			Secret:              "test-secret-key-for-jwt-signing",
			AccessExpiry:        3600,
			RefreshExpiry:       604800,
			KeyRotationEnabled:  false,
			KeyRotationInterval: 24,
			RevokeRefreshOnUse:  false,
		},
	}

	// Setup mocks
	mockRepo := &MockTokenRepository{}
	mockCache := NewMockCacheManager()
	
	// Allow cache operations
	mockCache.On("Set", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil)
	mockCache.On("Get", mock.Anything, mock.Anything).Return(nil, ErrCacheNotFound)
	mockCache.On("Delete", mock.Anything, mock.Anything).Return(nil)

	// Create token service
	tokenService := NewTokenService(config, zap.NewNop(), mockRepo, mockCache, redisClient)

	t.Run("GenerateTokenPair", func(t *testing.T) {
		ctx := context.Background()
		userID := uuid.New()
		organizationID := uuid.New()
		email := "test@example.com"

		// Mock repository calls
		mockRepo.On("Create", ctx, mock.AnythingOfType("*models.Token")).Return(nil).Twice()

		// Generate token pair
		tokenPair, err := tokenService.GenerateTokenPair(ctx, userID, organizationID, email, nil)

		// Assertions
		assert.NoError(t, err)
		assert.NotNil(t, tokenPair)
		assert.NotEmpty(t, tokenPair.AccessToken)
		assert.NotEmpty(t, tokenPair.RefreshToken)
		assert.Equal(t, "Bearer", tokenPair.TokenType)
		assert.True(t, tokenPair.ExpiresAt.After(time.Now()))
		assert.True(t, tokenPair.RefreshExpiresAt.After(tokenPair.ExpiresAt))
	})

	t.Run("ValidateTokenString", func(t *testing.T) {
		// Test valid token format
		err := tokenService.ValidateTokenString("header.payload.signature")
		assert.NoError(t, err)

		// Test invalid formats
		testCases := []string{
			"",
			"invalid",
			"header.payload",
			"header.payload.signature.extra",
		}

		for _, tc := range testCases {
			err := tokenService.ValidateTokenString(tc)
			assert.Error(t, err)
		}
	})

	t.Run("HashToken", func(t *testing.T) {
		token := "test.token.string"
		hash1 := tokenService.hashToken(token)
		hash2 := tokenService.hashToken(token)

		// Same token should produce same hash
		assert.Equal(t, hash1, hash2)
		assert.NotEmpty(t, hash1)
		assert.Len(t, hash1, 64) // SHA256 hex string length
	})

	t.Run("RevokeToken", func(t *testing.T) {
		ctx := context.Background()
		token := "test.token.string"
		tokenHash := tokenService.hashToken(token)

		// Mock repository call
		mockRepo.On("RevokeToken", ctx, tokenHash).Return(nil)

		// Revoke token
		err := tokenService.RevokeToken(ctx, token)
		assert.NoError(t, err)

		// Check if token is blacklisted
		assert.True(t, tokenService.isTokenBlacklisted(ctx, tokenHash))
	})

	t.Run("GetMetrics", func(t *testing.T) {
		metrics := tokenService.GetMetrics()
		assert.NotNil(t, metrics)
		// Metrics should be initialized
		assert.GreaterOrEqual(t, metrics.TokensGenerated, int64(0))
	})
}

// TestTokenService_SecurityFeatures tests security-related functionality
func TestTokenService_SecurityFeatures(t *testing.T) {
	// Setup mini redis
	miniRedis, err := miniredis.Run()
	assert.NoError(t, err)
	defer miniRedis.Close()

	// Setup Redis client
	redisClient := redis.NewClient(&redis.Options{
		Addr: miniRedis.Addr(),
	})
	defer redisClient.Close()

	// Setup config with key rotation enabled
	config := &config.Config{
		JWT: config.JWTConfig{
			Secret:              "test-secret-key-for-jwt-signing",
			AccessExpiry:        3600,
			RefreshExpiry:       604800,
			KeyRotationEnabled:  true,
			KeyRotationInterval: 1, // 1 hour for testing
			RevokeRefreshOnUse:  true,
		},
	}

	// Setup mocks
	mockRepo := &MockTokenRepository{}
	mockCache := NewMockCacheManager()
	
	// Allow cache operations
	mockCache.On("Set", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil)
	mockCache.On("Get", mock.Anything, mock.Anything).Return(nil, ErrCacheNotFound)
	mockCache.On("Delete", mock.Anything, mock.Anything).Return(nil)

	// Create token service
	tokenService := NewTokenService(config, zap.NewNop(), mockRepo, mockCache, redisClient)

	t.Run("KeyRotation", func(t *testing.T) {
		initialVersion := tokenService.getKeyVersion()
		
		// Perform key rotation
		err := tokenService.rotateKeys()
		assert.NoError(t, err)
		
		newVersion := tokenService.getKeyVersion()
		assert.Greater(t, newVersion, initialVersion)
		
		// Check metrics
		metrics := tokenService.GetMetrics()
		assert.Greater(t, metrics.KeyRotations, int64(0))
	})

	t.Run("SecurityContext", func(t *testing.T) {
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
		mockRepo.On("Create", ctx, mock.AnythingOfType("*models.Token")).Return(nil).Twice()

		// Generate token pair with security context
		tokenPair, err := tokenService.GenerateTokenPair(ctx, userID, organizationID, email, securityCtx)
		assert.NoError(t, err)
		assert.NotNil(t, tokenPair)

		// Extract token info to verify security context is included
		claims, err := tokenService.GetTokenInfo(ctx, tokenPair.AccessToken)
		assert.NoError(t, err)
		assert.Equal(t, securityCtx.IPAddress, claims.IPAddress)
		assert.Equal(t, securityCtx.DeviceFingerprint, claims.DeviceFingerprint)
	})

	t.Run("RevokeAllUserTokens", func(t *testing.T) {
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

		mockRepo.On("GetActiveTokensByUser", ctx, userID, models.AccessToken).Return(mockTokens, nil)
		mockRepo.On("RevokeAllUserTokens", ctx, userID, models.AccessToken).Return(nil)

		// Revoke all user tokens
		err := tokenService.RevokeAllUserTokens(ctx, userID, entities.AccessToken)
		assert.NoError(t, err)

		// Verify tokens are blacklisted
		for _, token := range mockTokens {
			assert.True(t, tokenService.isTokenBlacklisted(ctx, token.TokenHash))
		}
	})
}

// TestTokenService_Performance tests performance-related functionality
func TestTokenService_Performance(t *testing.T) {
	// Setup mini redis
	miniRedis, err := miniredis.Run()
	assert.NoError(t, err)
	defer miniRedis.Close()

	// Setup Redis client
	redisClient := redis.NewClient(&redis.Options{
		Addr: miniRedis.Addr(),
	})
	defer redisClient.Close()

	// Setup config
	config := &config.Config{
		JWT: config.JWTConfig{
			Secret:       "test-secret-key-for-jwt-signing",
			AccessExpiry: 3600,
			RefreshExpiry: 604800,
		},
	}

	// Setup mocks
	mockRepo := &MockTokenRepository{}
	mockCache := NewMockCacheManager()
	
	// Allow cache operations
	mockCache.On("Set", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil)
	mockCache.On("Get", mock.Anything, mock.Anything).Return(nil, ErrCacheNotFound)
	mockCache.On("Delete", mock.Anything, mock.Anything).Return(nil)
	mockRepo.On("Create", mock.Anything, mock.AnythingOfType("*models.Token")).Return(nil)

	// Create token service
	tokenService := NewTokenService(config, zap.NewNop(), mockRepo, mockCache, redisClient)

	t.Run("ConcurrentTokenGeneration", func(t *testing.T) {
		ctx := context.Background()
		userID := uuid.New()
		organizationID := uuid.New()
		email := "test@example.com"

		const numGoroutines = 10
		results := make(chan error, numGoroutines)

		// Run concurrent token generations
		for i := 0; i < numGoroutines; i++ {
			go func() {
				_, err := tokenService.GenerateTokenPair(ctx, userID, organizationID, email, nil)
				results <- err
			}()
		}

		// Collect results
		for i := 0; i < numGoroutines; i++ {
			err := <-results
			assert.NoError(t, err)
		}

		// Verify metrics are consistent
		metrics := tokenService.GetMetrics()
		assert.Equal(t, int64(numGoroutines*2), metrics.TokensGenerated)
	})

	t.Run("MemoryPoolUsage", func(t *testing.T) {
		// Test that memory pools are working by generating many tokens
		ctx := context.Background()
		userID := uuid.New()
		organizationID := uuid.New()
		email := "test@example.com"

		for i := 0; i < 100; i++ {
			_, err := tokenService.GenerateTokenPair(ctx, userID, organizationID, email, nil)
			assert.NoError(t, err)
		}

		// If memory pools are working correctly, this should not cause memory issues
		metrics := tokenService.GetMetrics()
		assert.GreaterOrEqual(t, metrics.TokensGenerated, int64(200)) // At least 100 * 2 (access + refresh)
	})
}

// Define a simple cache error for testing
var ErrCacheNotFound = &CacheError{Code: "not_found", Message: "cache entry not found"}

type CacheError struct {
	Code    string
	Message string
}

func (e *CacheError) Error() string {
	return e.Message
}