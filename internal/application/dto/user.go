package dto

import (
	"time"

	"github.com/google/uuid"
)

// CreateUserRequest represents the user creation request payload
type CreateUserRequest struct {
	Email          string    `json:"email" binding:"required,email"`
	Password       string    `json:"password" binding:"required,min=8"`
	FirstName      string    `json:"first_name" binding:"required,min=1"`
	LastName       string    `json:"last_name" binding:"required,min=1"`
	OrganizationID uuid.UUID `json:"organization_id" binding:"required"`
}

// UpdateUserRequest represents the user update request payload
type UpdateUserRequest struct {
	FirstName string `json:"first_name,omitempty" binding:"omitempty,min=1"`
	LastName  string `json:"last_name,omitempty" binding:"omitempty,min=1"`
	Email     string `json:"email,omitempty" binding:"omitempty,email"`
}

// UserResponse represents the user response payload
type UserResponse struct {
	ID               uuid.UUID  `json:"id"`
	Email            string     `json:"email"`
	FirstName        string     `json:"first_name"`
	LastName         string     `json:"last_name"`
	FullName         string     `json:"full_name"`
	OrganizationID   uuid.UUID  `json:"organization_id"`
	IsActive         bool       `json:"is_active"`
	IsVerified       bool       `json:"is_verified"`
	TwoFactorEnabled bool       `json:"two_factor_enabled"`
	LastLoginAt      *time.Time `json:"last_login_at"`
	CreatedAt        time.Time  `json:"created_at"`
	UpdatedAt        time.Time  `json:"updated_at"`
}

// UserListResponse represents a paginated list of users
type UserListResponse struct {
	Users      []UserResponse `json:"users"`
	Total      int            `json:"total"`
	Page       int            `json:"page"`
	PageSize   int            `json:"page_size"`
	TotalPages int            `json:"total_pages"`
}

// AssignRoleRequest represents the role assignment request payload
type AssignRoleRequest struct {
	RoleID    uuid.UUID  `json:"role_id" binding:"required"`
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
}

// UserPermissionResponse represents user permissions in responses
type UserPermissionResponse struct {
	ID          uuid.UUID `json:"id"`
	Name        string    `json:"name"`
	Resource    string    `json:"resource"`
	Action      string    `json:"action"`
	Scope       string    `json:"scope"`
	Description string    `json:"description"`
}

// UserRoleResponse represents user roles in responses
type UserRoleResponse struct {
	ID          uuid.UUID  `json:"id"`
	Name        string     `json:"name"`
	Description string     `json:"description"`
	AssignedAt  time.Time  `json:"assigned_at"`
	ExpiresAt   *time.Time `json:"expires_at,omitempty"`
	IsActive    bool       `json:"is_active"`
}