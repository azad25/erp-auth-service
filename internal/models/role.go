package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type Role struct {
	ID             uuid.UUID `gorm:"type:uuid;primary_key;default:gen_random_uuid()" json:"id"`
	OrganizationID uuid.UUID `gorm:"type:uuid;not null;index" json:"organization_id"`
	Name           string    `gorm:"not null;size:100" json:"name" validate:"required,min=2,max=100"`
	Description    string    `gorm:"size:500" json:"description"`
	IsSystem       bool      `gorm:"default:false" json:"is_system"`
	IsActive       bool      `gorm:"default:true" json:"is_active"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`

	// Relationships
	Organization    Organization     `gorm:"foreignKey:OrganizationID" json:"organization,omitempty"`
	UserRoles       []UserRole       `gorm:"foreignKey:RoleID" json:"user_roles,omitempty"`
	RolePermissions []RolePermission `gorm:"foreignKey:RoleID" json:"role_permissions,omitempty"`
}

type Permission struct {
	ID          uuid.UUID `gorm:"type:uuid;primary_key;default:gen_random_uuid()" json:"id"`
	Name        string    `gorm:"unique;not null;size:100" json:"name" validate:"required"`
	Resource    string    `gorm:"not null;size:100" json:"resource" validate:"required"`
	Action      string    `gorm:"not null;size:50" json:"action" validate:"required"`
	Description string    `gorm:"size:500" json:"description"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`

	// Relationships
	RolePermissions []RolePermission `gorm:"foreignKey:PermissionID" json:"role_permissions,omitempty"`
}

type UserRole struct {
	ID        uuid.UUID `gorm:"type:uuid;primary_key;default:gen_random_uuid()" json:"id"`
	UserID    uuid.UUID `gorm:"type:uuid;not null;index" json:"user_id"`
	RoleID    uuid.UUID `gorm:"type:uuid;not null;index" json:"role_id"`
	CreatedAt time.Time `json:"created_at"`

	// Relationships
	User User `gorm:"foreignKey:UserID" json:"user,omitempty"`
	Role Role `gorm:"foreignKey:RoleID" json:"role,omitempty"`
}

type RolePermission struct {
	ID           uuid.UUID `gorm:"type:uuid;primary_key;default:gen_random_uuid()" json:"id"`
	RoleID       uuid.UUID `gorm:"type:uuid;not null;index" json:"role_id"`
	PermissionID uuid.UUID `gorm:"type:uuid;not null;index" json:"permission_id"`
	CreatedAt    time.Time `json:"created_at"`

	// Relationships
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