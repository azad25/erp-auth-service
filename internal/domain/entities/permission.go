package entities

import (
	"time"

	"github.com/google/uuid"
)

// Permission represents the core permission entity with hierarchical support
type Permission struct {
	ID          uuid.UUID `json:"id"`
	Name        string    `json:"name"`
	Resource    string    `json:"resource"`
	Action      string    `json:"action"`
	Scope       string    `json:"scope"` // organization, global, etc.

	// Hierarchical permissions
	ParentID *uuid.UUID   `json:"parent_id"`
	Children []Permission `json:"children,omitempty"`

	// Metadata
	Description string `json:"description"`
	IsSystem    bool   `json:"is_system"`

	// Audit fields
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Role represents a collection of permissions
type Role struct {
	ID             uuid.UUID `json:"id"`
	OrganizationID uuid.UUID `json:"organization_id"`
	Name           string    `json:"name"`
	Description    string    `json:"description"`
	IsSystem       bool      `json:"is_system"`
	IsActive       bool      `json:"is_active"`

	// Audit fields
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// UserRole represents the many-to-many relationship between users and roles
type UserRole struct {
	ID     uuid.UUID `json:"id"`
	UserID uuid.UUID `json:"user_id"`
	RoleID uuid.UUID `json:"role_id"`

	// Assignment metadata
	AssignedBy uuid.UUID  `json:"assigned_by"`
	AssignedAt time.Time  `json:"assigned_at"`
	ExpiresAt  *time.Time `json:"expires_at"`
	IsActive   bool       `json:"is_active"`
}

// RolePermission represents the many-to-many relationship between roles and permissions
type RolePermission struct {
	ID           uuid.UUID `json:"id"`
	RoleID       uuid.UUID `json:"role_id"`
	PermissionID uuid.UUID `json:"permission_id"`

	// Assignment metadata
	GrantedBy uuid.UUID `json:"granted_by"`
	GrantedAt time.Time `json:"granted_at"`
	IsActive  bool      `json:"is_active"`
}

// IsExpired checks if a user role assignment has expired
func (ur *UserRole) IsExpired() bool {
	return ur.ExpiresAt != nil && ur.ExpiresAt.Before(time.Now())
}

// IsValid checks if a user role assignment is currently valid
func (ur *UserRole) IsValid() bool {
	return ur.IsActive && !ur.IsExpired()
}