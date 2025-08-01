package services

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/mock"
	"go.uber.org/zap"

	"erp-auth-service/internal/config"
	"erp-auth-service/internal/models"
)

// BenchmarkTokenGeneration benchmarks token generation performance
func BenchmarkTokenGeneration(b *testing.B) {
	// Setup mini redis
	miniRedis, err := miniredis.Run()
	if err != nil {
		b.Fatal(err)
	}
	defer miniRedis.Close()

	// Setup Redis client
	redisClient := redis.NewClient(&redis.Options{
		Addr: miniRedis.Addr(),
	})
	defer redisClient.Close()

	// Setup config
	config := &config.Config{
		JWT: config.JWTConfig{
			Secret:       "benchmark-secret-key-for-jwt-signing",
			AccessExpiry: 3600,
			RefreshExpiry: 604800,
		},
	}

	// Setup mocks
	mockRepo := &MockTokenRepository{}
	mockCache := NewMockCacheManager()
	
	// Allow cache operations without storing data to avoid race conditions
	mockCache.On("Set", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil)
	mockRepo.On("Create", mock.Anything, mock.AnythingOfType("*models.Token")).Return(nil)

	// Create token service
	tokenService := NewTokenService(config, zap.NewNop(), mockRepo, mockCache, redisClient)

	// Test data
	userID := uuid.New()
	organizationID := uuid.New()
	email := "benchmark@example.com"
	ctx := context.Background()

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_, err := tokenService.GenerateTokenPair(ctx, userID, organizationID, email, nil)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkTokenHashing benchmarks token hashing performance
func BenchmarkTokenHashing(b *testing.B) {
	// Setup mini redis
	miniRedis, err := miniredis.Run()
	if err != nil {
		b.Fatal(err)
	}
	defer miniRedis.Close()

	// Setup Redis client
	redisClient := redis.NewClient(&redis.Options{
		Addr: miniRedis.Addr(),
	})
	defer redisClient.Close()

	// Setup config
	config := &config.Config{
		JWT: config.JWTConfig{
			Secret:       "benchmark-secret-key-for-jwt-signing",
			AccessExpiry: 3600,
			RefreshExpiry: 604800,
		},
	}

	// Setup mocks
	mockRepo := &MockTokenRepository{}
	mockCache := NewMockCacheManager()

	// Create token service
	tokenService := NewTokenService(config, zap.NewNop(), mockRepo, mockCache, redisClient)

	// Test token
	token := "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiIxMjM0NTY3ODkwIiwibmFtZSI6IkpvaG4gRG9lIiwiaWF0IjoxNTE2MjM5MDIyfQ.SflKxwRJSMeKKF2QT4fwpMeJf36POk6yJV_adQssw5c"

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_ = tokenService.hashToken(token)
	}
}

// BenchmarkTokenValidation benchmarks token validation performance
func BenchmarkTokenValidation(b *testing.B) {
	// Setup mini redis
	miniRedis, err := miniredis.Run()
	if err != nil {
		b.Fatal(err)
	}
	defer miniRedis.Close()

	// Setup Redis client
	redisClient := redis.NewClient(&redis.Options{
		Addr: miniRedis.Addr(),
	})
	defer redisClient.Close()

	// Setup config
	config := &config.Config{
		JWT: config.JWTConfig{
			Secret:       "benchmark-secret-key-for-jwt-signing",
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
	mockRepo.On("Create", mock.Anything, mock.AnythingOfType("*models.Token")).Return(nil)

	// Create token service
	tokenService := NewTokenService(config, zap.NewNop(), mockRepo, mockCache, redisClient)

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
	mockRepo.On("GetByTokenHash", ctx, tokenHash).Return(mockToken, nil)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_, err := tokenService.ValidateToken(ctx, tokenPair.AccessToken)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkKeyRotation benchmarks key rotation performance
func BenchmarkKeyRotation(b *testing.B) {
	// Setup mini redis
	miniRedis, err := miniredis.Run()
	if err != nil {
		b.Fatal(err)
	}
	defer miniRedis.Close()

	// Setup Redis client
	redisClient := redis.NewClient(&redis.Options{
		Addr: miniRedis.Addr(),
	})
	defer redisClient.Close()

	// Setup config
	config := &config.Config{
		JWT: config.JWTConfig{
			Secret:              "benchmark-secret-key-for-jwt-signing",
			AccessExpiry:        3600,
			RefreshExpiry:       604800,
			KeyRotationEnabled:  true,
			KeyRotationInterval: 24,
		},
	}

	// Setup mocks
	mockRepo := &MockTokenRepository{}
	mockCache := NewMockCacheManager()

	// Create token service
	tokenService := NewTokenService(config, zap.NewNop(), mockRepo, mockCache, redisClient)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		err := tokenService.rotateKeys()
		if err != nil {
			b.Fatal(err)
		}
	}
}