package dto

import (
	"time"

	"github.com/google/uuid"
)

// CreateOrganizationRequest represents the organization creation request payload
type CreateOrganizationRequest struct {
	Name        string `json:"name" binding:"required,min=1"`
	Domain      string `json:"domain" binding:"required,min=1"`
	Description string `json:"description,omitempty"`
}

// UpdateOrganizationRequest represents the organization update request payload
type UpdateOrganizationRequest struct {
	Name        string `json:"name,omitempty" binding:"omitempty,min=1"`
	Domain      string `json:"domain,omitempty" binding:"omitempty,min=1"`
	Description string `json:"description,omitempty"`
}

// OrganizationResponse represents the organization response payload
type OrganizationResponse struct {
	ID          uuid.UUID                    `json:"id"`
	Name        string                       `json:"name"`
	Domain      string                       `json:"domain"`
	Description string                       `json:"description"`
	IsActive    bool                         `json:"is_active"`
	IsVerified  bool                         `json:"is_verified"`
	Settings    OrganizationSettingsResponse `json:"settings"`
	CreatedAt   time.Time                    `json:"created_at"`
	UpdatedAt   time.Time                    `json:"updated_at"`
}

// OrganizationSettingsResponse represents organization settings in responses
type OrganizationSettingsResponse struct {
	MaxUsers                int  `json:"max_users"`
	TwoFactorRequired       bool `json:"two_factor_required"`
	PasswordMinLength       int  `json:"password_min_length"`
	PasswordRequireSpecial  bool `json:"password_require_special"`
	SessionTimeoutMinutes   int  `json:"session_timeout_minutes"`
	MaxFailedLoginAttempts  int  `json:"max_failed_login_attempts"`
	AccountLockoutMinutes   int  `json:"account_lockout_minutes"`
}

// UpdateOrganizationSettingsRequest represents the organization settings update request
type UpdateOrganizationSettingsRequest struct {
	MaxUsers                *int  `json:"max_users,omitempty" binding:"omitempty,min=1"`
	TwoFactorRequired       *bool `json:"two_factor_required,omitempty"`
	PasswordMinLength       *int  `json:"password_min_length,omitempty" binding:"omitempty,min=6,max=128"`
	PasswordRequireSpecial  *bool `json:"password_require_special,omitempty"`
	SessionTimeoutMinutes   *int  `json:"session_timeout_minutes,omitempty" binding:"omitempty,min=5,max=1440"`
	MaxFailedLoginAttempts  *int  `json:"max_failed_login_attempts,omitempty" binding:"omitempty,min=1,max=10"`
	AccountLockoutMinutes   *int  `json:"account_lockout_minutes,omitempty" binding:"omitempty,min=1,max=1440"`
}

// OrganizationStatsResponse represents organization statistics
type OrganizationStatsResponse struct {
	UserCount       int `json:"user_count"`
	ActiveUserCount int `json:"active_user_count"`
	RoleCount       int `json:"role_count"`
	PermissionCount int `json:"permission_count"`
}