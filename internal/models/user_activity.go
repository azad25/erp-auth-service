package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// UserActivity represents a user activity log entry
type UserActivity struct {
	ID             uuid.UUID `gorm:"type:uuid;primary_key;default:gen_random_uuid()" json:"id"`
	UserID         uuid.UUID `gorm:"type:uuid;not null;index" json:"user_id"`
	OrganizationID uuid.UUID `gorm:"type:uuid;not null;index" json:"organization_id"`
	Action         string    `gorm:"type:varchar(100);not null" json:"action"`
	Resource       string    `gorm:"type:varchar(100);not null" json:"resource"`
	Details        string    `gorm:"type:jsonb" json:"details"`
	IPAddress      string    `gorm:"type:varchar(45)" json:"ip_address"`
	UserAgent      string    `gorm:"type:text" json:"user_agent"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`

	// Associations
	User         User         `gorm:"foreignKey:UserID" json:"user,omitempty"`
	Organization Organization `gorm:"foreignKey:OrganizationID" json:"organization,omitempty"`
}

// BeforeCreate sets the ID if not already set
func (ua *UserActivity) BeforeCreate(tx *gorm.DB) error {
	if ua.ID == uuid.Nil {
		ua.ID = uuid.New()
	}
	return nil
}

// TableName returns the table name for UserActivity
func (UserActivity) TableName() string {
	return "user_activities"
}

// ActivityAction constants for common actions
const (
	ActionLogin            = "login"
	ActionLogout           = "logout"
	ActionLoginFailed      = "login_failed"
	ActionPasswordChanged = "password_changed"
	ActionProfileUpdated   = "profile_updated"
	ActionUserCreated      = "user_created"
	ActionUserUpdated      = "user_updated"
	ActionUserDeleted      = "user_deleted"
	ActionUserActivated    = "user_activated"
	ActionUserDeactivated  = "user_deactivated"
	ActionRoleAssigned     = "role_assigned"
	ActionRoleRemoved      = "role_removed"
	ActionPermissionGranted = "permission_granted"
	ActionPermissionRevoked = "permission_revoked"
	ActionOrganizationCreated = "organization_created"
	ActionOrganizationUpdated = "organization_updated"
	ActionOrganizationDeleted = "organization_deleted"
)

// ActivityResource constants for common resources
const (
	ResourceUser         = "user"
	ResourceRole         = "role"
	ResourcePermission   = "permission"
	ResourceOrganization = "organization"
	ResourceAuth         = "auth"
	ResourceProfile      = "profile"
)