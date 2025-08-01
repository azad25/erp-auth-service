package dto

import (
	"time"

	"github.com/google/uuid"
)

// LoginRequest represents the login request payload
type LoginRequest struct {
	Email    string `json:"email" binding:"required,email"`
	Password string `json:"password" binding:"required,min=1"`
}

// LoginResponse represents the login response payload
type LoginResponse struct {
	AccessToken      string    `json:"access_token"`
	RefreshToken     string    `json:"refresh_token"`
	TokenType        string    `json:"token_type"`
	ExpiresAt        time.Time `json:"expires_at"`
	RefreshExpiresAt time.Time `json:"refresh_expires_at"`
	User             UserInfo  `json:"user"`
}

// RefreshTokenRequest represents the refresh token request payload
type RefreshTokenRequest struct {
	RefreshToken string `json:"refresh_token" binding:"required"`
}

// ResetPasswordRequest represents the password reset request payload
type ResetPasswordRequest struct {
	Email string `json:"email" binding:"required,email"`
}

// ConfirmPasswordResetRequest represents the password reset confirmation payload
type ConfirmPasswordResetRequest struct {
	Token       string `json:"token" binding:"required"`
	NewPassword string `json:"new_password" binding:"required,min=8"`
}

// ChangePasswordRequest represents the password change request payload
type ChangePasswordRequest struct {
	OldPassword string `json:"old_password" binding:"required"`
	NewPassword string `json:"new_password" binding:"required,min=8"`
}

// EnableTwoFactorResponse represents the two-factor authentication setup response
type EnableTwoFactorResponse struct {
	Secret      string   `json:"secret"`
	QRCode      string   `json:"qr_code"`
	BackupCodes []string `json:"backup_codes"`
}

// VerifyTwoFactorRequest represents the two-factor authentication verification request
type VerifyTwoFactorRequest struct {
	Code string `json:"code" binding:"required,len=6"`
}

// DisableTwoFactorRequest represents the two-factor authentication disable request
type DisableTwoFactorRequest struct {
	Password string `json:"password" binding:"required"`
}

// UserInfo represents basic user information in responses
type UserInfo struct {
	ID             uuid.UUID `json:"id"`
	Email          string    `json:"email"`
	FirstName      string    `json:"first_name"`
	LastName       string    `json:"last_name"`
	FullName       string    `json:"full_name"`
	OrganizationID uuid.UUID `json:"organization_id"`
	IsActive       bool      `json:"is_active"`
	IsVerified     bool      `json:"is_verified"`
	TwoFactorEnabled bool    `json:"two_factor_enabled"`
	LastLoginAt    *time.Time `json:"last_login_at"`
	CreatedAt      time.Time `json:"created_at"`
}