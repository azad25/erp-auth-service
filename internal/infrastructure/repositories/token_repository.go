package repositories

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"erp-auth-service/internal/domain/interfaces"
	"erp-auth-service/internal/models"
)

// tokenRepository implements the TokenRepository interface
type tokenRepository struct {
	db     *gorm.DB
	readDB *gorm.DB // Read replica for read operations
}

// NewTokenRepository creates a new token repository
func NewTokenRepository(db, readDB *gorm.DB) interfaces.TokenRepository {
	return &tokenRepository{
		db:     db,
		readDB: readDB,
	}
}

// GetByID retrieves a token by ID
func (r *tokenRepository) GetByID(ctx context.Context, id uuid.UUID) (*models.Token, error) {
	var token models.Token
	err := r.readDB.WithContext(ctx).
		Where("id = ?", id).
		First(&token).Error
	
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, fmt.Errorf("token not found")
		}
		return nil, fmt.Errorf("failed to get token by ID: %w", err)
	}
	
	return &token, nil
}

// GetByTokenHash retrieves a token by its hash (optimized for validation)
func (r *tokenRepository) GetByTokenHash(ctx context.Context, tokenHash string) (*models.Token, error) {
	var token models.Token
	err := r.readDB.WithContext(ctx).
		Where("token_hash = ? AND revoked_at IS NULL AND expires_at > ?", tokenHash, time.Now()).
		First(&token).Error
	
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, fmt.Errorf("token not found or expired")
		}
		return nil, fmt.Errorf("failed to get token by hash: %w", err)
	}
	
	return &token, nil
}

// Create creates a new token
func (r *tokenRepository) Create(ctx context.Context, token *models.Token) error {
	err := r.db.WithContext(ctx).Create(token).Error
	if err != nil {
		return fmt.Errorf("failed to create token: %w", err)
	}
	return nil
}

// Update updates an existing token
func (r *tokenRepository) Update(ctx context.Context, token *models.Token) error {
	err := r.db.WithContext(ctx).Save(token).Error
	if err != nil {
		return fmt.Errorf("failed to update token: %w", err)
	}
	return nil
}

// Delete deletes a token
func (r *tokenRepository) Delete(ctx context.Context, id uuid.UUID) error {
	err := r.db.WithContext(ctx).
		Where("id = ?", id).
		Delete(&models.Token{}).Error
	
	if err != nil {
		return fmt.Errorf("failed to delete token: %w", err)
	}
	return nil
}

// RevokeToken revokes a token by setting revoked_at timestamp
func (r *tokenRepository) RevokeToken(ctx context.Context, tokenHash string) error {
	now := time.Now()
	err := r.db.WithContext(ctx).
		Model(&models.Token{}).
		Where("token_hash = ? AND revoked_at IS NULL", tokenHash).
		Updates(map[string]interface{}{
			"revoked_at": now,
			"updated_at": now,
		}).Error
	
	if err != nil {
		return fmt.Errorf("failed to revoke token: %w", err)
	}
	return nil
}

// RevokeAllUserTokens revokes all tokens of a specific type for a user
func (r *tokenRepository) RevokeAllUserTokens(ctx context.Context, userID uuid.UUID, tokenType models.TokenType) error {
	now := time.Now()
	err := r.db.WithContext(ctx).
		Model(&models.Token{}).
		Where("user_id = ? AND token_type = ? AND revoked_at IS NULL", userID, tokenType).
		Updates(map[string]interface{}{
			"revoked_at": now,
			"updated_at": now,
		}).Error
	
	if err != nil {
		return fmt.Errorf("failed to revoke all user tokens: %w", err)
	}
	return nil
}

// GetActiveTokensByUser retrieves all active tokens for a user of a specific type
func (r *tokenRepository) GetActiveTokensByUser(ctx context.Context, userID uuid.UUID, tokenType models.TokenType) ([]models.Token, error) {
	var tokens []models.Token
	err := r.readDB.WithContext(ctx).
		Where("user_id = ? AND token_type = ? AND revoked_at IS NULL AND expires_at > ?", 
			userID, tokenType, time.Now()).
		Order("created_at DESC").
		Find(&tokens).Error
	
	if err != nil {
		return nil, fmt.Errorf("failed to get active tokens by user: %w", err)
	}
	
	return tokens, nil
}

// CleanupExpiredTokens removes expired tokens from the database
func (r *tokenRepository) CleanupExpiredTokens(ctx context.Context) error {
	err := r.db.WithContext(ctx).
		Where("expires_at < ?", time.Now()).
		Delete(&models.Token{}).Error
	
	if err != nil {
		return fmt.Errorf("failed to cleanup expired tokens: %w", err)
	}
	return nil
}

// DeleteRevokedTokens removes revoked tokens older than the specified time
func (r *tokenRepository) DeleteRevokedTokens(ctx context.Context, olderThan time.Time) error {
	err := r.db.WithContext(ctx).
		Where("revoked_at IS NOT NULL AND revoked_at < ?", olderThan).
		Delete(&models.Token{}).Error
	
	if err != nil {
		return fmt.Errorf("failed to delete revoked tokens: %w", err)
	}
	return nil
}