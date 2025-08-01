package repositories

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/suite"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"erp-auth-service/internal/models"
)

// TokenRepositoryTestSuite defines the test suite for token repository
type TokenRepositoryTestSuite struct {
	suite.Suite
	db         *gorm.DB
	readDB     *gorm.DB
	repository *tokenRepository
	ctx        context.Context
}

// SetupSuite sets up the test suite
func (suite *TokenRepositoryTestSuite) SetupSuite() {
	// Use in-memory SQLite for testing
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	suite.Require().NoError(err)

	// Use same DB for read operations in tests
	readDB := db

	// Auto-migrate test schema
	err = db.AutoMigrate(
		&models.Organization{},
		&models.User{},
		&models.Token{},
	)
	suite.Require().NoError(err)

	suite.db = db
	suite.readDB = readDB
	suite.repository = &tokenRepository{db: db, readDB: readDB}
	suite.ctx = context.Background()
}

// TearDownSuite cleans up after tests
func (suite *TokenRepositoryTestSuite) TearDownSuite() {
	sqlDB, err := suite.db.DB()
	if err == nil {
		sqlDB.Close()
	}
}

// SetupTest sets up each individual test
func (suite *TokenRepositoryTestSuite) SetupTest() {
	// Clean up tables before each test
	suite.db.Exec("DELETE FROM tokens")
	suite.db.Exec("DELETE FROM users")
	suite.db.Exec("DELETE FROM organizations")
}

// TestCreateToken tests token creation
func (suite *TokenRepositoryTestSuite) TestCreateToken() {
	// Create test user
	user := suite.createTestUser()

	// Create test token
	token := &models.Token{
		ID:        uuid.New(),
		UserID:    user.ID,
		TokenType: models.AccessToken,
		TokenHash: "test-hash-123",
		IssuedAt:  time.Now(),
		ExpiresAt: time.Now().Add(1 * time.Hour),
		IPAddress: "192.168.1.1",
		UserAgent: "test-agent",
	}

	err := suite.repository.Create(suite.ctx, token)
	suite.Assert().NoError(err)
	suite.Assert().NotEqual(uuid.Nil, token.ID)

	// Verify token was created
	var createdToken models.Token
	err = suite.db.Where("token_hash = ?", token.TokenHash).First(&createdToken).Error
	suite.Assert().NoError(err)
	suite.Assert().Equal(token.TokenHash, createdToken.TokenHash)
}

// TestGetByTokenHash tests retrieving token by hash
func (suite *TokenRepositoryTestSuite) TestGetByTokenHash() {
	// Create test data
	user := suite.createTestUser()
	token := suite.createTestToken(user.ID, models.AccessToken, "test-hash-456")

	// Test successful retrieval
	retrievedToken, err := suite.repository.GetByTokenHash(suite.ctx, token.TokenHash)
	suite.Assert().NoError(err)
	suite.Assert().Equal(token.ID, retrievedToken.ID)
	suite.Assert().Equal(token.UserID, retrievedToken.UserID)

	// Test non-existent token
	_, err = suite.repository.GetByTokenHash(suite.ctx, "non-existent-hash")
	suite.Assert().Error(err)
	suite.Assert().Contains(err.Error(), "token not found")
}

// TestGetByTokenHashExpired tests that expired tokens are not returned
func (suite *TokenRepositoryTestSuite) TestGetByTokenHashExpired() {
	// Create test data with expired token
	user := suite.createTestUser()
	expiredToken := &models.Token{
		ID:        uuid.New(),
		UserID:    user.ID,
		TokenType: models.AccessToken,
		TokenHash: "expired-hash",
		IssuedAt:  time.Now().Add(-2 * time.Hour),
		ExpiresAt: time.Now().Add(-1 * time.Hour), // Expired 1 hour ago
		IPAddress: "192.168.1.1",
		UserAgent: "test-agent",
	}
	suite.Require().NoError(suite.db.Create(expiredToken).Error)

	// Test that expired token is not returned
	_, err := suite.repository.GetByTokenHash(suite.ctx, expiredToken.TokenHash)
	suite.Assert().Error(err)
	suite.Assert().Contains(err.Error(), "token not found")
}

// TestRevokeToken tests token revocation
func (suite *TokenRepositoryTestSuite) TestRevokeToken() {
	// Create test data
	user := suite.createTestUser()
	token := suite.createTestToken(user.ID, models.AccessToken, "revoke-test-hash")

	// Revoke token
	err := suite.repository.RevokeToken(suite.ctx, token.TokenHash)
	suite.Assert().NoError(err)

	// Verify token is revoked (not returned by GetByTokenHash)
	_, err = suite.repository.GetByTokenHash(suite.ctx, token.TokenHash)
	suite.Assert().Error(err)
	suite.Assert().Contains(err.Error(), "token not found")

	// Verify token exists in database but is revoked
	var dbToken models.Token
	err = suite.db.Where("token_hash = ?", token.TokenHash).First(&dbToken).Error
	suite.Assert().NoError(err)
	suite.Assert().NotNil(dbToken.RevokedAt)
}

// TestRevokeAllUserTokens tests revoking all tokens for a user
func (suite *TokenRepositoryTestSuite) TestRevokeAllUserTokens() {
	// Create test data
	user := suite.createTestUser()
	token1 := suite.createTestToken(user.ID, models.AccessToken, "user-token-1")
	token2 := suite.createTestToken(user.ID, models.AccessToken, "user-token-2")
	token3 := suite.createTestToken(user.ID, models.RefreshToken, "user-refresh-token")

	// Revoke all access tokens for user
	err := suite.repository.RevokeAllUserTokens(suite.ctx, user.ID, models.AccessToken)
	suite.Assert().NoError(err)

	// Verify access tokens are revoked
	_, err = suite.repository.GetByTokenHash(suite.ctx, token1.TokenHash)
	suite.Assert().Error(err)
	_, err = suite.repository.GetByTokenHash(suite.ctx, token2.TokenHash)
	suite.Assert().Error(err)

	// Verify refresh token is still active
	retrievedToken, err := suite.repository.GetByTokenHash(suite.ctx, token3.TokenHash)
	suite.Assert().NoError(err)
	suite.Assert().Equal(token3.ID, retrievedToken.ID)
}

// TestGetActiveTokensByUser tests retrieving active tokens for a user
func (suite *TokenRepositoryTestSuite) TestGetActiveTokensByUser() {
	// Create test data
	user := suite.createTestUser()
	activeToken1 := suite.createTestToken(user.ID, models.AccessToken, "active-1")
	activeToken2 := suite.createTestToken(user.ID, models.AccessToken, "active-2")
	
	// Create revoked token
	revokedToken := suite.createTestToken(user.ID, models.AccessToken, "revoked")
	suite.repository.RevokeToken(suite.ctx, revokedToken.TokenHash)

	// Get active tokens
	activeTokens, err := suite.repository.GetActiveTokensByUser(suite.ctx, user.ID, models.AccessToken)
	suite.Assert().NoError(err)
	suite.Assert().Len(activeTokens, 2)

	// Verify correct tokens are returned
	tokenHashes := make(map[string]bool)
	for _, token := range activeTokens {
		tokenHashes[token.TokenHash] = true
	}
	suite.Assert().True(tokenHashes[activeToken1.TokenHash])
	suite.Assert().True(tokenHashes[activeToken2.TokenHash])
	suite.Assert().False(tokenHashes[revokedToken.TokenHash])
}

// TestCleanupExpiredTokens tests cleanup of expired tokens
func (suite *TokenRepositoryTestSuite) TestCleanupExpiredTokens() {
	// Create test data
	user := suite.createTestUser()
	
	// Create active token
	activeToken := suite.createTestToken(user.ID, models.AccessToken, "active-token")
	
	// Create expired token
	expiredToken := &models.Token{
		ID:        uuid.New(),
		UserID:    user.ID,
		TokenType: models.AccessToken,
		TokenHash: "expired-token",
		IssuedAt:  time.Now().Add(-2 * time.Hour),
		ExpiresAt: time.Now().Add(-1 * time.Hour), // Expired
		IPAddress: "192.168.1.1",
		UserAgent: "test-agent",
	}
	suite.Require().NoError(suite.db.Create(expiredToken).Error)

	// Run cleanup
	err := suite.repository.CleanupExpiredTokens(suite.ctx)
	suite.Assert().NoError(err)

	// Verify expired token is deleted
	var count int64
	suite.db.Model(&models.Token{}).Where("token_hash = ?", expiredToken.TokenHash).Count(&count)
	suite.Assert().Equal(int64(0), count)

	// Verify active token still exists
	suite.db.Model(&models.Token{}).Where("token_hash = ?", activeToken.TokenHash).Count(&count)
	suite.Assert().Equal(int64(1), count)
}

// TestDeleteRevokedTokens tests deletion of old revoked tokens
func (suite *TokenRepositoryTestSuite) TestDeleteRevokedTokens() {
	// Create test data
	user := suite.createTestUser()
	
	// Create and revoke old token
	oldRevokedToken := suite.createTestToken(user.ID, models.AccessToken, "old-revoked")
	suite.repository.RevokeToken(suite.ctx, oldRevokedToken.TokenHash)
	
	// Manually set revoked_at to old date
	oldTime := time.Now().Add(-48 * time.Hour)
	suite.db.Model(&models.Token{}).Where("token_hash = ?", oldRevokedToken.TokenHash).
		Update("revoked_at", oldTime)

	// Create recently revoked token
	recentRevokedToken := suite.createTestToken(user.ID, models.AccessToken, "recent-revoked")
	suite.repository.RevokeToken(suite.ctx, recentRevokedToken.TokenHash)

	// Delete tokens revoked more than 24 hours ago
	cutoffTime := time.Now().Add(-24 * time.Hour)
	err := suite.repository.DeleteRevokedTokens(suite.ctx, cutoffTime)
	suite.Assert().NoError(err)

	// Verify old revoked token is deleted
	var count int64
	suite.db.Model(&models.Token{}).Where("token_hash = ?", oldRevokedToken.TokenHash).Count(&count)
	suite.Assert().Equal(int64(0), count)

	// Verify recent revoked token still exists
	suite.db.Model(&models.Token{}).Where("token_hash = ?", recentRevokedToken.TokenHash).Count(&count)
	suite.Assert().Equal(int64(1), count)
}

// Helper methods

func (suite *TokenRepositoryTestSuite) createTestUser() *models.User {
	// Create organization without Settings field for SQLite compatibility
	orgID := uuid.New()
	err := suite.db.Exec("INSERT INTO organizations (id, name, domain, is_active, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)",
		orgID.String(), "Test Org", "test.com", true, time.Now(), time.Now()).Error
	suite.Require().NoError(err)

	user := &models.User{
		ID:             uuid.New(),
		OrganizationID: orgID,
		Email:          "test@test.com",
		PasswordHash:   "hashedpassword",
		FirstName:      "Test",
		LastName:       "User",
		IsActive:       true,
	}
	suite.Require().NoError(suite.db.Create(user).Error)
	return user
}

func (suite *TokenRepositoryTestSuite) createTestToken(userID uuid.UUID, tokenType models.TokenType, hash string) *models.Token {
	token := &models.Token{
		ID:        uuid.New(),
		UserID:    userID,
		TokenType: tokenType,
		TokenHash: hash,
		IssuedAt:  time.Now(),
		ExpiresAt: time.Now().Add(1 * time.Hour),
		IPAddress: "192.168.1.1",
		UserAgent: "test-agent",
	}
	suite.Require().NoError(suite.db.Create(token).Error)
	return token
}

// TestTokenRepository runs the token repository test suite
func TestTokenRepository(t *testing.T) {
	suite.Run(t, new(TokenRepositoryTestSuite))
}