package entities

import (
	"time"

	"github.com/google/uuid"
)

// Organization represents the core organization entity
type Organization struct {
	ID          uuid.UUID `json:"id"`
	Name        string    `json:"name"`
	Domain      string    `json:"domain"`
	Description string    `json:"description"`
	
	// Status fields
	IsActive    bool `json:"is_active"`
	IsVerified  bool `json:"is_verified"`
	
	// Settings
	Settings OrganizationSettings `json:"settings"`
	
	// Audit fields
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// OrganizationSettings contains organization-specific configuration
type OrganizationSettings struct {
	MaxUsers                int  `json:"max_users"`
	TwoFactorRequired       bool `json:"two_factor_required"`
	PasswordMinLength       int  `json:"password_min_length"`
	PasswordRequireSpecial  bool `json:"password_require_special"`
	SessionTimeoutMinutes   int  `json:"session_timeout_minutes"`
	MaxFailedLoginAttempts  int  `json:"max_failed_login_attempts"`
	AccountLockoutMinutes   int  `json:"account_lockout_minutes"`
}

// DefaultOrganizationSettings returns default settings for new organizations
func DefaultOrganizationSettings() OrganizationSettings {
	return OrganizationSettings{
		MaxUsers:                100,
		TwoFactorRequired:       false,
		PasswordMinLength:       8,
		PasswordRequireSpecial:  true,
		SessionTimeoutMinutes:   480, // 8 hours
		MaxFailedLoginAttempts:  5,
		AccountLockoutMinutes:   15,
	}
}