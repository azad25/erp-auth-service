package entities

import (
	"time"

	"github.com/google/uuid"
)

// User represents the core user entity following clean architecture principles
type User struct {
	// Primary identifiers
	ID             uuid.UUID `json:"id"`
	OrganizationID uuid.UUID `json:"organization_id"`

	// Authentication fields
	Email        string `json:"email"`
	PasswordHash string `json:"-"`

	// Profile information
	FirstName string `json:"first_name"`
	LastName  string `json:"last_name"`

	// Status and security
	IsActive       bool       `json:"is_active"`
	IsVerified     bool       `json:"is_verified"`
	LastLoginAt    *time.Time `json:"last_login_at"`
	PasswordResetAt *time.Time `json:"password_reset_at"`

	// Two-factor authentication
	TwoFactorEnabled bool     `json:"two_factor_enabled"`
	TwoFactorSecret  string   `json:"-"`
	BackupCodes      []string `json:"-"`

	// Security tracking
	FailedLoginAttempts int        `json:"-"`
	LockedUntil        *time.Time `json:"-"`

	// Audit fields
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`

	// Computed fields for caching
	FullName        string `json:"full_name"`
	PermissionCache string `json:"-"` // JSON serialized permissions
}

// GetFullName returns the user's full name
func (u *User) GetFullName() string {
	if u.FullName != "" {
		return u.FullName
	}
	return u.FirstName + " " + u.LastName
}

// IsLocked checks if the user account is currently locked
func (u *User) IsLocked() bool {
	return u.LockedUntil != nil && u.LockedUntil.After(time.Now())
}

// CanAttemptLogin checks if the user can attempt to login
func (u *User) CanAttemptLogin() bool {
	return u.IsActive && u.IsVerified && !u.IsLocked()
}