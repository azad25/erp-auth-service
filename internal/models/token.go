package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// TokenType represents different types of tokens
type TokenType string

const (
	AccessToken  TokenType = "access"
	RefreshToken TokenType = "refresh"
	ResetToken   TokenType = "reset"
)

// Token model optimized for high-performance token validation
type Token struct {
	ID        uuid.UUID `gorm:"type:uuid;primary_key;index:idx_token_id" json:"id"`
	UserID    uuid.UUID `gorm:"type:uuid;not null;index:idx_token_user_id" json:"user_id"`
	TokenType TokenType `gorm:"type:varchar(20);not null;index:idx_token_type" json:"token_type"`
	TokenHash string    `gorm:"unique;not null;size:64;index:idx_token_hash" json:"token_hash"` // SHA256 hash
	
	// Token metadata with performance indexes
	IssuedAt  time.Time  `gorm:"not null;index:idx_token_issued" json:"issued_at"`
	ExpiresAt time.Time  `gorm:"not null;index:idx_token_expires" json:"expires_at"`
	RevokedAt *time.Time `gorm:"index:idx_token_revoked" json:"revoked_at"`
	
	// Security context
	IPAddress         string `gorm:"size:45" json:"ip_address"` // IPv6 compatible
	UserAgent         string `gorm:"size:500" json:"user_agent"`
	DeviceFingerprint string `gorm:"size:255" json:"device_fingerprint"`
	
	// Audit fields
	CreatedAt time.Time `gorm:"index:idx_token_created" json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	
	// Relationships (lazy loaded for performance)
	User User `gorm:"foreignKey:UserID" json:"user,omitempty"`
}

func (t *Token) BeforeCreate(tx *gorm.DB) error {
	if t.ID == uuid.Nil {
		t.ID = uuid.New()
	}
	return nil
}

// IsExpired checks if the token has expired
func (t *Token) IsExpired() bool {
	return time.Now().After(t.ExpiresAt)
}

// IsRevoked checks if the token has been revoked
func (t *Token) IsRevoked() bool {
	return t.RevokedAt != nil
}

// IsValid checks if the token is currently valid
func (t *Token) IsValid() bool {
	return !t.IsExpired() && !t.IsRevoked()
}

// Revoke marks the token as revoked
func (t *Token) Revoke() {
	now := time.Now()
	t.RevokedAt = &now
	t.UpdatedAt = now
}

func (t *Token) TableName() string {
	return "tokens"
}