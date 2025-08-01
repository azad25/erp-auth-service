package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type Role struct {
	ID             uuid.UUID `gorm:"type:uuid;primary_key;index:idx_role_id" json:"id"`
	OrganizationID uuid.UUID `gorm:"type:uuid;not null;index:idx_role_org" json:"organization_id"`
	Name           string    `gorm:"not null;size:100;index:idx_role_name" json:"name" validate:"required,min=2,max=100"`
	Description    string    `gorm:"size:500" json:"description"`
	IsSystem       bool      `gorm:"default:false;index:idx_role_system" json:"is_system"`
	IsActive       bool      `gorm:"default:true;index:idx_role_active" json:"is_active"`
	CreatedAt      time.Time `gorm:"index:idx_role_created" json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`

	// Relationships (lazy loaded for performance)
	Organization    Organization     `gorm:"foreignKey:OrganizationID" json:"organization,omitempty"`
	UserRoles       []UserRole       `gorm:"foreignKey:RoleID" json:"user_roles,omitempty"`
	RolePermissions []RolePermission `gorm:"foreignKey:RoleID" json:"role_permissions,omitempty"`
}

// Permission model with hierarchical support and optimized indexes
type Permission struct {
	ID          uuid.UUID `gorm:"type:uuid;primary_key;index:idx_permission_id" json:"id"`
	Name        string    `gorm:"unique;not null;size:100;index:idx_permission_name" json:"name" validate:"required"`
	Resource    string    `gorm:"not null;size:100;index:idx_permission_resource" json:"resource" validate:"required"`
	Action      string    `gorm:"not null;size:50;index:idx_permission_action" json:"action" validate:"required"`
	Scope       string    `gorm:"size:100" json:"scope"` // organization, global, etc.
	
	// Hierarchical permissions
	ParentID    *uuid.UUID `gorm:"type:uuid;index:idx_permission_parent" json:"parent_id"`
	Children    []Permission `gorm:"foreignKey:ParentID" json:"children,omitempty"`
	
	// Metadata
	Description string    `gorm:"size:500" json:"description"`
	IsSystem    bool      `gorm:"default:false;index:idx_permission_system" json:"is_system"`
	CreatedAt   time.Time `gorm:"index:idx_permission_created" json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`

	// Relationships (lazy loaded for performance)
	RolePermissions []RolePermission `gorm:"foreignKey:PermissionID" json:"role_permissions,omitempty"`
}

type UserRole struct {
	ID        uuid.UUID `gorm:"type:uuid;primary_key;index:idx_user_role_id" json:"id"`
	UserID    uuid.UUID `gorm:"type:uuid;not null;index:idx_user_role_user_id;uniqueIndex:idx_user_role_unique" json:"user_id"`
	RoleID    uuid.UUID `gorm:"type:uuid;not null;index:idx_user_role_role_id;uniqueIndex:idx_user_role_unique" json:"role_id"`
	CreatedAt time.Time `gorm:"index:idx_user_role_created" json:"created_at"`

	// Relationships (lazy loaded for performance)
	User User `gorm:"foreignKey:UserID" json:"user,omitempty"`
	Role Role `gorm:"foreignKey:RoleID" json:"role,omitempty"`
}

type RolePermission struct {
	ID           uuid.UUID `gorm:"type:uuid;primary_key;index:idx_role_permission_id" json:"id"`
	RoleID       uuid.UUID `gorm:"type:uuid;not null;index:idx_role_permission_role_id;uniqueIndex:idx_role_permission_unique" json:"role_id"`
	PermissionID uuid.UUID `gorm:"type:uuid;not null;index:idx_role_permission_permission_id;uniqueIndex:idx_role_permission_unique" json:"permission_id"`
	CreatedAt    time.Time `gorm:"index:idx_role_permission_created" json:"created_at"`

	// Relationships (lazy loaded for performance)
	Role       Role       `gorm:"foreignKey:RoleID" json:"role,omitempty"`
	Permission Permission `gorm:"foreignKey:PermissionID" json:"permission,omitempty"`
}

func (r *Role) BeforeCreate(tx *gorm.DB) error {
	if r.ID == uuid.Nil {
		r.ID = uuid.New()
	}
	return nil
}

func (p *Permission) BeforeCreate(tx *gorm.DB) error {
	if p.ID == uuid.Nil {
		p.ID = uuid.New()
	}
	return nil
}

func (ur *UserRole) BeforeCreate(tx *gorm.DB) error {
	if ur.ID == uuid.Nil {
		ur.ID = uuid.New()
	}
	return nil
}

func (rp *RolePermission) BeforeCreate(tx *gorm.DB) error {
	if rp.ID == uuid.Nil {
		rp.ID = uuid.New()
	}
	return nil
}

func (r *Role) TableName() string {
	return "roles"
}

func (p *Permission) TableName() string {
	return "permissions"
}

func (ur *UserRole) TableName() string {
	return "user_roles"
}

func (rp *RolePermission) TableName() string {
	return "role_permissions"
}