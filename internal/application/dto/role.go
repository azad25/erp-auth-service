package dto

import (
	"time"

	"github.com/google/uuid"
)

// CreateRoleRequest represents the role creation request payload
type CreateRoleRequest struct {
	Name           string    `json:"name" binding:"required,min=1"`
	Description    string    `json:"description,omitempty"`
	OrganizationID uuid.UUID `json:"organization_id" binding:"required"`
}

// UpdateRoleRequest represents the role update request payload
type UpdateRoleRequest struct {
	Name        string `json:"name,omitempty" binding:"omitempty,min=1"`
	Description string `json:"description,omitempty"`
}

// RoleResponse represents the role response payload
type RoleResponse struct {
	ID             uuid.UUID `json:"id"`
	Name           string    `json:"name"`
	Description    string    `json:"description"`
	OrganizationID uuid.UUID `json:"organization_id"`
	IsSystem       bool      `json:"is_system"`
	IsActive       bool      `json:"is_active"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

// AssignPermissionToRoleRequest represents the permission assignment request
type AssignPermissionToRoleRequest struct {
	PermissionID uuid.UUID `json:"permission_id" binding:"required"`
}

// CreatePermissionRequest represents the permission creation request payload
type CreatePermissionRequest struct {
	Name        string     `json:"name" binding:"required,min=1"`
	Resource    string     `json:"resource" binding:"required,min=1"`
	Action      string     `json:"action" binding:"required,min=1"`
	Scope       string     `json:"scope" binding:"required,min=1"`
	Description string     `json:"description,omitempty"`
	ParentID    *uuid.UUID `json:"parent_id,omitempty"`
}

// UpdatePermissionRequest represents the permission update request payload
type UpdatePermissionRequest struct {
	Name        string     `json:"name,omitempty" binding:"omitempty,min=1"`
	Resource    string     `json:"resource,omitempty" binding:"omitempty,min=1"`
	Action      string     `json:"action,omitempty" binding:"omitempty,min=1"`
	Scope       string     `json:"scope,omitempty" binding:"omitempty,min=1"`
	Description string     `json:"description,omitempty"`
	ParentID    *uuid.UUID `json:"parent_id,omitempty"`
}

// PermissionResponse represents the permission response payload
type PermissionResponse struct {
	ID          uuid.UUID              `json:"id"`
	Name        string                 `json:"name"`
	Resource    string                 `json:"resource"`
	Action      string                 `json:"action"`
	Scope       string                 `json:"scope"`
	Description string                 `json:"description"`
	ParentID    *uuid.UUID             `json:"parent_id,omitempty"`
	Children    []PermissionResponse   `json:"children,omitempty"`
	IsSystem    bool                   `json:"is_system"`
	CreatedAt   time.Time              `json:"created_at"`
	UpdatedAt   time.Time              `json:"updated_at"`
}

// CheckPermissionRequest represents the permission check request
type CheckPermissionRequest struct {
	Resource string `json:"resource" binding:"required"`
	Action   string `json:"action" binding:"required"`
}

// CheckPermissionResponse represents the permission check response
type CheckPermissionResponse struct {
	HasPermission bool   `json:"has_permission"`
	Resource      string `json:"resource"`
	Action        string `json:"action"`
}