package entities

import (
	"time"

	"github.com/google/uuid"
)

// TokenType represents different types of tokens
type TokenType string

const (
	AccessToken  TokenType = "access"
	RefreshToken TokenType = "refresh"
	ResetToken   TokenType = "reset"
)

// Token represents a JWT token entity
type Token struct {
	ID        uuid.UUID `json:"id"`
	UserID    uuid.UUID `json:"user_id"`
	TokenType TokenType `json:"token_type"`
	TokenHash string    `json:"token_hash"` // SHA256 hash of the token
	
	// Token metadata
	IssuedAt  time.Time  `json:"issued_at"`
	ExpiresAt time.Time  `json:"expires_at"`
	RevokedAt *time.Time `json:"revoked_at"`
	
	// Security context
	IPAddress     string `json:"ip_address"`
	UserAgent     string `json:"user_agent"`
	DeviceFingerprint string `json:"device_fingerprint"`
	
	// Audit fields
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
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

// TokenPair represents an access and refresh token pair
type TokenPair struct {
	AccessToken  string    `json:"access_token"`
	RefreshToken string    `json:"refresh_token"`
	ExpiresAt    time.Time `json:"expires_at"`
	RefreshExpiresAt time.Time `json:"refresh_expires_at"`
	TokenType    string    `json:"token_type"` // "Bearer"
}