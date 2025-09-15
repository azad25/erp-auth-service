package services

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"

	"erp-auth-service/internal/cache"
	"erp-auth-service/internal/config"
	"erp-auth-service/internal/domain/entities"
	"erp-auth-service/internal/domain/interfaces"
	"erp-auth-service/internal/models"
)

// TokenService handles JWT token operations with high performance and security
type TokenService struct {
	config         *config.Config
	logger         *zap.Logger
	tokenRepo      interfaces.TokenRepository
	cacheManager   cache.CacheManager
	redisClient    *redis.Client
	
	// Key rotation support
	currentKey     []byte
	previousKey    []byte
	keyRotationMu  sync.RWMutex
	keyVersion     int
	
	// Memory pools for high-throughput operations
	tokenPool      sync.Pool
	claimsPool     sync.Pool
	
	// Performance metrics
	metrics        *TokenMetrics
}

// TokenClaims represents JWT claims with additional security context
type TokenClaims struct {
	UserID           uuid.UUID `json:"user_id"`
	OrganizationID   uuid.UUID `json:"organization_id"`
	Email            string    `json:"email"`
	TokenType        string    `json:"token_type"`
	KeyVersion       int       `json:"key_version"`
	IPAddress        string    `json:"ip_address,omitempty"`
	DeviceFingerprint string   `json:"device_fingerprint,omitempty"`
	SessionID        string    `json:"session_id,omitempty"`
	jwt.RegisteredClaims
}

// TokenMetrics tracks token service performance
type TokenMetrics struct {
	TokensGenerated   int64
	TokensValidated   int64
	TokensRevoked     int64
	CacheHits         int64
	CacheMisses       int64
	ValidationErrors  int64
	KeyRotations      int64
	mu                sync.RWMutex
}

// SecurityContext contains security information for token operations
type SecurityContext struct {
	IPAddress         string
	UserAgent         string
	DeviceFingerprint string
	SessionID         string
}

// NewTokenService creates a new token service instance
func NewTokenService(
	config *config.Config,
	logger *zap.Logger,
	tokenRepo interfaces.TokenRepository,
	cacheManager cache.CacheManager,
	redisClient *redis.Client,
) *TokenService {
	service := &TokenService{
		config:       config,
		logger:       logger,
		tokenRepo:    tokenRepo,
		cacheManager: cacheManager,
		redisClient:  redisClient,
		currentKey:   []byte(config.JWT.Secret),
		keyVersion:   1,
		metrics:      &TokenMetrics{},
	}
	
	// Initialize memory pools
	service.initializePools()
	
	// Start key rotation if enabled
	if config.JWT.KeyRotationEnabled {
		go service.startKeyRotation()
	}
	
	// Start cleanup routines
	go service.startCleanupRoutines()
	
	return service
}

// initializePools sets up memory pools for high-throughput operations
func (s *TokenService) initializePools() {
	s.tokenPool = sync.Pool{
		New: func() interface{} {
			return &models.Token{}
		},
	}
	
	s.claimsPool = sync.Pool{
		New: func() interface{} {
			return &TokenClaims{}
		},
	}
}

// GenerateTokenPair creates access and refresh token pair with security features
func (s *TokenService) GenerateTokenPair(ctx context.Context, userID, organizationID uuid.UUID, email string, securityCtx *SecurityContext) (*entities.TokenPair, error) {
	now := time.Now()
	sessionID := s.generateSessionID()
	
	// Generate access token
	accessToken, accessTokenEntity, err := s.generateToken(ctx, userID, organizationID, email, entities.AccessToken, securityCtx, sessionID, now)
	if err != nil {
		s.logger.Error("Failed to generate access token", zap.Error(err), zap.String("user_id", userID.String()))
		return nil, fmt.Errorf("failed to generate access token: %w", err)
	}
	
	// Generate refresh token
	refreshToken, refreshTokenEntity, err := s.generateToken(ctx, userID, organizationID, email, entities.RefreshToken, securityCtx, sessionID, now)
	if err != nil {
		s.logger.Error("Failed to generate refresh token", zap.Error(err), zap.String("user_id", userID.String()))
		return nil, fmt.Errorf("failed to generate refresh token: %w", err)
	}
	
	// Store tokens in database
	if err := s.tokenRepo.Create(ctx, accessTokenEntity); err != nil {
		s.logger.Error("Failed to store access token", zap.Error(err))
		return nil, fmt.Errorf("failed to store access token: %w", err)
	}
	
	if err := s.tokenRepo.Create(ctx, refreshTokenEntity); err != nil {
		s.logger.Error("Failed to store refresh token", zap.Error(err))
		return nil, fmt.Errorf("failed to store refresh token: %w", err)
	}
	
	// Cache tokens for fast validation
	if err := s.cacheTokens(ctx, accessToken, refreshToken, accessTokenEntity, refreshTokenEntity); err != nil {
		s.logger.Warn("Failed to cache tokens", zap.Error(err))
	}
	
	// Update metrics
	s.updateMetrics(func(m *TokenMetrics) {
		m.TokensGenerated += 2
	})
	
	return &entities.TokenPair{
		AccessToken:       accessToken,
		RefreshToken:      refreshToken,
		ExpiresAt:         accessTokenEntity.ExpiresAt,
		RefreshExpiresAt:  refreshTokenEntity.ExpiresAt,
		TokenType:         "Bearer",
	}, nil
}

// generateToken creates a JWT token with the specified parameters
func (s *TokenService) generateToken(ctx context.Context, userID, organizationID uuid.UUID, email string, tokenType entities.TokenType, securityCtx *SecurityContext, sessionID string, issuedAt time.Time) (string, *models.Token, error) {
	// Get claims from pool
	claims := s.claimsPool.Get().(*TokenClaims)
	defer s.claimsPool.Put(claims)
	
	// Reset claims
	*claims = TokenClaims{}
	
	// Set token expiry based on type
	var expiresAt time.Time
	switch tokenType {
	case entities.AccessToken:
		expiresAt = issuedAt.Add(time.Duration(s.config.JWT.AccessExpiry) * time.Second)
	case entities.RefreshToken:
		expiresAt = issuedAt.Add(time.Duration(s.config.JWT.RefreshExpiry) * time.Second)
	default:
		return "", nil, fmt.Errorf("invalid token type: %s", tokenType)
	}
	
	// Set claims
	claims.UserID = userID
	claims.OrganizationID = organizationID
	claims.Email = email
	claims.TokenType = string(tokenType)
	claims.KeyVersion = s.getKeyVersion()
	claims.SessionID = sessionID
	
	if securityCtx != nil {
		claims.IPAddress = securityCtx.IPAddress
		claims.DeviceFingerprint = securityCtx.DeviceFingerprint
	}
	
	claims.RegisteredClaims = jwt.RegisteredClaims{
		ID:        uuid.New().String(),
		Subject:   userID.String(),
		IssuedAt:  jwt.NewNumericDate(issuedAt),
		ExpiresAt: jwt.NewNumericDate(expiresAt),
		NotBefore: jwt.NewNumericDate(issuedAt),
		Issuer:    "erp-auth-service",
		Audience:  []string{"erp-suite"},
	}
	
	// Create JWT token
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	
	// Sign token with current key
	s.keyRotationMu.RLock()
	tokenString, err := token.SignedString(s.currentKey)
	s.keyRotationMu.RUnlock()
	
	if err != nil {
		return "", nil, fmt.Errorf("failed to sign token: %w", err)
	}
	
	// Create token hash for storage
	tokenHash := s.hashToken(tokenString)
	
	// Create token entity
	tokenEntity := &models.Token{
		ID:        uuid.New(),
		UserID:    userID,
		TokenType: models.TokenType(tokenType),
		TokenHash: tokenHash,
		IssuedAt:  issuedAt,
		ExpiresAt: expiresAt,
		CreatedAt: issuedAt,
		UpdatedAt: issuedAt,
	}
	
	if securityCtx != nil {
		tokenEntity.IPAddress = securityCtx.IPAddress
		tokenEntity.UserAgent = securityCtx.UserAgent
		tokenEntity.DeviceFingerprint = securityCtx.DeviceFingerprint
	}
	
	return tokenString, tokenEntity, nil
}

// ValidateToken validates a JWT token with Redis-based blacklisting
func (s *TokenService) ValidateToken(ctx context.Context, tokenString string) (*TokenClaims, error) {
	// Check if token is blacklisted in cache first
	tokenHash := s.hashToken(tokenString)
	if s.isTokenBlacklisted(ctx, tokenHash) {
		s.updateMetrics(func(m *TokenMetrics) {
			m.ValidationErrors++
		})
		return nil, fmt.Errorf("token is blacklisted")
	}
	
	// Try to get token from cache
	if cachedClaims, err := s.getTokenFromCache(ctx, tokenHash); err == nil {
		s.updateMetrics(func(m *TokenMetrics) {
			m.TokensValidated++
			m.CacheHits++
		})
		return cachedClaims, nil
	}
	
	s.updateMetrics(func(m *TokenMetrics) {
		m.CacheMisses++
	})
	
	// Parse and validate JWT token
	claims, err := s.parseAndValidateJWT(tokenString)
	if err != nil {
		s.updateMetrics(func(m *TokenMetrics) {
			m.ValidationErrors++
		})
		return nil, fmt.Errorf("invalid token: %w", err)
	}
	
	// Verify token exists in database and is not revoked
	tokenEntity, err := s.tokenRepo.GetByTokenHash(ctx, tokenHash)
	if err != nil {
		s.updateMetrics(func(m *TokenMetrics) {
			m.ValidationErrors++
		})
		return nil, fmt.Errorf("token not found or revoked: %w", err)
	}
	
	if !tokenEntity.IsValid() {
		s.updateMetrics(func(m *TokenMetrics) {
			m.ValidationErrors++
		})
		return nil, fmt.Errorf("token is expired or revoked")
	}
	
	// Cache valid token for future validations
	if err := s.cacheTokenClaims(ctx, tokenHash, claims); err != nil {
		s.logger.Warn("Failed to cache token claims", zap.Error(err))
	}
	
	s.updateMetrics(func(m *TokenMetrics) {
		m.TokensValidated++
	})
	
	return claims, nil
}

// RefreshToken creates a new access token using a valid refresh token
func (s *TokenService) RefreshToken(ctx context.Context, refreshTokenString string, securityCtx *SecurityContext) (*entities.TokenPair, error) {
	// Validate refresh token
	claims, err := s.ValidateToken(ctx, refreshTokenString)
	if err != nil {
		return nil, fmt.Errorf("invalid refresh token: %w", err)
	}
	
	if claims.TokenType != string(entities.RefreshToken) {
		return nil, fmt.Errorf("token is not a refresh token")
	}
	
	// Generate new token pair
	tokenPair, err := s.GenerateTokenPair(ctx, claims.UserID, claims.OrganizationID, claims.Email, securityCtx)
	if err != nil {
		return nil, fmt.Errorf("failed to generate new token pair: %w", err)
	}
	
	// Optionally revoke the old refresh token (depending on security policy)
	if s.config.JWT.RevokeRefreshOnUse {
		if err := s.RevokeToken(ctx, refreshTokenString); err != nil {
			s.logger.Warn("Failed to revoke old refresh token", zap.Error(err))
		}
	}
	
	return tokenPair, nil
}

// RevokeToken revokes a token by adding it to the blacklist
func (s *TokenService) RevokeToken(ctx context.Context, tokenString string) error {
	tokenHash := s.hashToken(tokenString)
	
	// Add to Redis blacklist
	if err := s.blacklistToken(ctx, tokenHash); err != nil {
		s.logger.Error("Failed to blacklist token", zap.Error(err))
		return fmt.Errorf("failed to blacklist token: %w", err)
	}
	
	// Mark as revoked in database
	if err := s.tokenRepo.RevokeToken(ctx, tokenHash); err != nil {
		s.logger.Error("Failed to revoke token in database", zap.Error(err))
		return fmt.Errorf("failed to revoke token in database: %w", err)
	}
	
	// Remove from cache
	if err := s.removeTokenFromCache(ctx, tokenHash); err != nil {
		s.logger.Warn("Failed to remove token from cache", zap.Error(err))
	}
	
	s.updateMetrics(func(m *TokenMetrics) {
		m.TokensRevoked++
	})
	
	return nil
}

// RevokeAllUserTokens revokes all tokens for a specific user
func (s *TokenService) RevokeAllUserTokens(ctx context.Context, userID uuid.UUID, tokenType entities.TokenType) error {
	// Get all active tokens for the user
	tokens, err := s.tokenRepo.GetActiveTokensByUser(ctx, userID, models.TokenType(tokenType))
	if err != nil {
		return fmt.Errorf("failed to get user tokens: %w", err)
	}
	
	// Revoke each token
	for _, token := range tokens {
		if err := s.blacklistToken(ctx, token.TokenHash); err != nil {
			s.logger.Error("Failed to blacklist token", zap.Error(err), zap.String("token_id", token.ID.String()))
		}
		
		if err := s.removeTokenFromCache(ctx, token.TokenHash); err != nil {
			s.logger.Warn("Failed to remove token from cache", zap.Error(err))
		}
	}
	
	// Bulk revoke in database
	if err := s.tokenRepo.RevokeAllUserTokens(ctx, userID, models.TokenType(tokenType)); err != nil {
		return fmt.Errorf("failed to revoke user tokens in database: %w", err)
	}
	
	s.updateMetrics(func(m *TokenMetrics) {
		m.TokensRevoked += int64(len(tokens))
	})
	
	return nil
}

// parseAndValidateJWT parses and validates a JWT token
func (s *TokenService) parseAndValidateJWT(tokenString string) (*TokenClaims, error) {
	claims := &TokenClaims{}
	
	token, err := jwt.ParseWithClaims(tokenString, claims, func(token *jwt.Token) (interface{}, error) {
		// Validate signing method
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		
		// Get the appropriate key based on key version
		s.keyRotationMu.RLock()
		defer s.keyRotationMu.RUnlock()
		
		if claims.KeyVersion == s.keyVersion {
			return s.currentKey, nil
		} else if claims.KeyVersion == s.keyVersion-1 && s.previousKey != nil {
			return s.previousKey, nil
		}
		
		return nil, fmt.Errorf("invalid key version: %d", claims.KeyVersion)
	})
	
	if err != nil {
		return nil, err
	}
	
	if !token.Valid {
		return nil, fmt.Errorf("token is invalid")
	}
	
	return claims, nil
}

// hashToken creates a SHA256 hash of the token for storage
func (s *TokenService) hashToken(token string) string {
	hash := sha256.Sum256([]byte(token))
	return hex.EncodeToString(hash[:])
}

// generateSessionID creates a unique session identifier
func (s *TokenService) generateSessionID() string {
	bytes := make([]byte, 16)
	rand.Read(bytes)
	return hex.EncodeToString(bytes)
}

// getKeyVersion returns the current key version
func (s *TokenService) getKeyVersion() int {
	s.keyRotationMu.RLock()
	defer s.keyRotationMu.RUnlock()
	return s.keyVersion
}

// updateMetrics safely updates token metrics
func (s *TokenService) updateMetrics(updateFunc func(*TokenMetrics)) {
	s.metrics.mu.Lock()
	defer s.metrics.mu.Unlock()
	updateFunc(s.metrics)
}

// GetMetrics returns current token service metrics
func (s *TokenService) GetMetrics() TokenMetrics {
	s.metrics.mu.RLock()
	defer s.metrics.mu.RUnlock()
	// Return a copy without the mutex
	return TokenMetrics{
		TokensGenerated:  s.metrics.TokensGenerated,
		TokensValidated:  s.metrics.TokensValidated,
		TokensRevoked:    s.metrics.TokensRevoked,
		CacheHits:        s.metrics.CacheHits,
		CacheMisses:      s.metrics.CacheMisses,
		ValidationErrors: s.metrics.ValidationErrors,
		KeyRotations:     s.metrics.KeyRotations,
	}
}

// Cache management methods

// cacheTokens stores tokens in cache for fast validation
func (s *TokenService) cacheTokens(ctx context.Context, accessToken, refreshToken string, accessEntity, refreshEntity *models.Token) error {
	// Cache access token
	accessHash := s.hashToken(accessToken)
	accessClaims := &TokenClaims{
		UserID:         accessEntity.UserID,
		OrganizationID: accessEntity.UserID, // This should be from user data
		Email:          "", // This should be from user data
		TokenType:      string(accessEntity.TokenType),
		KeyVersion:     s.getKeyVersion(),
		IPAddress:      accessEntity.IPAddress,
		DeviceFingerprint: accessEntity.DeviceFingerprint,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   accessEntity.UserID.String(),
			IssuedAt:  jwt.NewNumericDate(accessEntity.IssuedAt),
			ExpiresAt: jwt.NewNumericDate(accessEntity.ExpiresAt),
			NotBefore: jwt.NewNumericDate(accessEntity.IssuedAt),
			Issuer:    "erp-auth-service",
			Audience:  []string{"erp-suite"},
		},
	}
	
	if err := s.cacheTokenClaims(ctx, accessHash, accessClaims); err != nil {
		return fmt.Errorf("failed to cache access token: %w", err)
	}
	
	// Cache refresh token
	refreshHash := s.hashToken(refreshToken)
	refreshClaims := &TokenClaims{
		UserID:         refreshEntity.UserID,
		OrganizationID: refreshEntity.UserID, // This should be from user data
		Email:          "", // This should be from user data
		TokenType:      string(refreshEntity.TokenType),
		KeyVersion:     s.getKeyVersion(),
		IPAddress:      refreshEntity.IPAddress,
		DeviceFingerprint: refreshEntity.DeviceFingerprint,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   refreshEntity.UserID.String(),
			IssuedAt:  jwt.NewNumericDate(refreshEntity.IssuedAt),
			ExpiresAt: jwt.NewNumericDate(refreshEntity.ExpiresAt),
			NotBefore: jwt.NewNumericDate(refreshEntity.IssuedAt),
			Issuer:    "erp-auth-service",
			Audience:  []string{"erp-suite"},
		},
	}
	
	if err := s.cacheTokenClaims(ctx, refreshHash, refreshClaims); err != nil {
		return fmt.Errorf("failed to cache refresh token: %w", err)
	}
	
	return nil
}

// cacheTokenClaims stores token claims in cache
func (s *TokenService) cacheTokenClaims(ctx context.Context, tokenHash string, claims *TokenClaims) error {
	cacheKey := fmt.Sprintf("token:claims:%s", tokenHash)
	
	claimsData, err := json.Marshal(claims)
	if err != nil {
		return fmt.Errorf("failed to marshal claims: %w", err)
	}
	
	ttl := time.Duration(s.config.JWT.AccessExpiry) * time.Second
	if claims.TokenType == string(entities.RefreshToken) {
		ttl = time.Duration(s.config.JWT.RefreshExpiry) * time.Second
	}
	
	return s.cacheManager.Set(ctx, cacheKey, claimsData, ttl)
}

// getTokenFromCache retrieves token claims from cache
func (s *TokenService) getTokenFromCache(ctx context.Context, tokenHash string) (*TokenClaims, error) {
	cacheKey := fmt.Sprintf("token:claims:%s", tokenHash)
	
	data, err := s.cacheManager.Get(ctx, cacheKey)
	if err != nil {
		return nil, err
	}
	
	claims := &TokenClaims{}
	if err := json.Unmarshal(data, claims); err != nil {
		return nil, fmt.Errorf("failed to unmarshal cached claims: %w", err)
	}
	
	return claims, nil
}

// removeTokenFromCache removes token from cache
func (s *TokenService) removeTokenFromCache(ctx context.Context, tokenHash string) error {
	cacheKey := fmt.Sprintf("token:claims:%s", tokenHash)
	return s.cacheManager.Delete(ctx, cacheKey)
}

// Blacklist management methods

// blacklistToken adds a token to the Redis blacklist
func (s *TokenService) blacklistToken(ctx context.Context, tokenHash string) error {
	blacklistKey := fmt.Sprintf("token:blacklist:%s", tokenHash)
	
	// Set with expiration matching the token's remaining lifetime
	ttl := time.Duration(s.config.JWT.RefreshExpiry) * time.Second // Use max expiry
	
	return s.redisClient.Set(ctx, blacklistKey, "revoked", ttl).Err()
}

// isTokenBlacklisted checks if a token is in the blacklist
func (s *TokenService) isTokenBlacklisted(ctx context.Context, tokenHash string) bool {
	blacklistKey := fmt.Sprintf("token:blacklist:%s", tokenHash)
	
	result := s.redisClient.Exists(ctx, blacklistKey)
	return result.Val() > 0
}

// Key rotation methods

// startKeyRotation starts the key rotation process
func (s *TokenService) startKeyRotation() {
	if s.config.JWT.KeyRotationInterval <= 0 {
		return
	}
	
	ticker := time.NewTicker(time.Duration(s.config.JWT.KeyRotationInterval) * time.Hour)
	defer ticker.Stop()
	
	for range ticker.C {
		if err := s.rotateKeys(); err != nil {
			s.logger.Error("Failed to rotate keys", zap.Error(err))
		}
	}
}

// rotateKeys performs key rotation
func (s *TokenService) rotateKeys() error {
	s.keyRotationMu.Lock()
	defer s.keyRotationMu.Unlock()
	
	// Generate new key
	newKey := make([]byte, 32)
	if _, err := rand.Read(newKey); err != nil {
		return fmt.Errorf("failed to generate new key: %w", err)
	}
	
	// Store previous key
	s.previousKey = s.currentKey
	s.currentKey = newKey
	s.keyVersion++
	
	s.updateMetrics(func(m *TokenMetrics) {
		m.KeyRotations++
	})
	
	s.logger.Info("Key rotation completed", zap.Int("new_version", s.keyVersion))
	return nil
}

// Cleanup routines

// startCleanupRoutines starts background cleanup processes
func (s *TokenService) startCleanupRoutines() {
	// Cleanup expired tokens every hour
	go func() {
		ticker := time.NewTicker(1 * time.Hour)
		defer ticker.Stop()
		
		for range ticker.C {
			ctx := context.Background()
			if err := s.tokenRepo.CleanupExpiredTokens(ctx); err != nil {
				s.logger.Error("Failed to cleanup expired tokens", zap.Error(err))
			}
		}
	}()
	
	// Cleanup old revoked tokens every 24 hours
	go func() {
		ticker := time.NewTicker(24 * time.Hour)
		defer ticker.Stop()
		
		for range ticker.C {
			ctx := context.Background()
			olderThan := time.Now().AddDate(0, 0, -30) // Keep revoked tokens for 30 days
			if err := s.tokenRepo.DeleteRevokedTokens(ctx, olderThan); err != nil {
				s.logger.Error("Failed to cleanup old revoked tokens", zap.Error(err))
			}
		}
	}()
}

// Utility methods

// ValidateTokenString validates a token string without full parsing
func (s *TokenService) ValidateTokenString(tokenString string) error {
	if tokenString == "" {
		return fmt.Errorf("token string is empty")
	}
	
	// Basic JWT format validation
	parts := strings.Split(tokenString, ".")
	if len(parts) != 3 {
		return fmt.Errorf("invalid token format")
	}
	
	return nil
}

// GetTokenInfo extracts basic information from a token without full validation
func (s *TokenService) GetTokenInfo(ctx context.Context, tokenString string) (*TokenClaims, error) {
	if err := s.ValidateTokenString(tokenString); err != nil {
		return nil, err
	}
	
	// Parse without validation for info extraction
	parser := jwt.NewParser(jwt.WithoutClaimsValidation())
	claims := &TokenClaims{}
	
	_, _, err := parser.ParseUnverified(tokenString, claims)
	if err != nil {
		return nil, fmt.Errorf("failed to parse token: %w", err)
	}
	
	return claims, nil
}

// IsTokenExpired checks if a token is expired without full validation
func (s *TokenService) IsTokenExpired(tokenString string) (bool, error) {
	claims, err := s.GetTokenInfo(context.Background(), tokenString)
	if err != nil {
		return false, err
	}
	
	if claims.ExpiresAt == nil {
		return false, fmt.Errorf("token has no expiration time")
	}
	
	return time.Now().After(claims.ExpiresAt.Time), nil
}