package events

import (
	"time"

	"github.com/google/uuid"
)

// Event types
const (
	EventTypeUserRegistered    = "user.registered"
	EventTypeUserLoggedIn      = "user.logged_in"
	EventTypeUserLoggedOut     = "user.logged_out"
	EventTypePasswordChanged   = "user.password_changed"
	EventTypeTokenRefreshed    = "token.refreshed"
	EventTypeTokenRevoked      = "token.revoked"
	EventTypeRoleAssigned      = "role.assigned"
	EventTypeRoleRevoked       = "role.revoked"
	EventTypePermissionChanged = "permission.changed"
	EventType2FAEnabled        = "user.2fa_enabled"
	EventType2FADisabled       = "user.2fa_disabled"
	EventTypeOrganizationCreated = "organization.created"
	EventTypeOrganizationUpdated = "organization.updated"
)

// Base event structure
type BaseEvent struct {
	ID             uuid.UUID   `json:"id"`
	Type           string      `json:"type"`
	Source         string      `json:"source"`
	Timestamp      time.Time   `json:"timestamp"`
	UserID         *uuid.UUID  `json:"user_id,omitempty"`
	OrganizationID *uuid.UUID  `json:"organization_id,omitempty"`
	IPAddress      string      `json:"ip_address,omitempty"`
	UserAgent      string      `json:"user_agent,omitempty"`
	Data           interface{} `json:"data"`
}

// User events
type UserRegisteredEvent struct {
	BaseEvent
	Data UserRegisteredData `json:"data"`
}

type UserRegisteredData struct {
	UserID         uuid.UUID `json:"user_id"`
	Email          string    `json:"email"`
	FirstName      string    `json:"first_name"`
	LastName       string    `json:"last_name"`
	OrganizationID uuid.UUID `json:"organization_id"`
}

type UserLoggedInEvent struct {
	BaseEvent
	Data UserLoggedInData `json:"data"`
}

type UserLoggedInData struct {
	UserID         uuid.UUID `json:"user_id"`
	Email          string    `json:"email"`
	OrganizationID uuid.UUID `json:"organization_id"`
	LoginMethod    string    `json:"login_method"` // "password", "2fa", etc.
}

type UserLoggedOutEvent struct {
	BaseEvent
	Data UserLoggedOutData `json:"data"`
}

type UserLoggedOutData struct {
	UserID         uuid.UUID `json:"user_id"`
	OrganizationID uuid.UUID `json:"organization_id"`
	LogoutReason   string    `json:"logout_reason"` // "manual", "timeout", "revoked"
}

type PasswordChangedEvent struct {
	BaseEvent
	Data PasswordChangedData `json:"data"`
}

type PasswordChangedData struct {
	UserID         uuid.UUID `json:"user_id"`
	OrganizationID uuid.UUID `json:"organization_id"`
	ChangedBy      uuid.UUID `json:"changed_by"` // Who changed it (could be admin)
}

// Token events
type TokenRefreshedEvent struct {
	BaseEvent
	Data TokenRefreshedData `json:"data"`
}

type TokenRefreshedData struct {
	UserID         uuid.UUID `json:"user_id"`
	OrganizationID uuid.UUID `json:"organization_id"`
	TokenID        string    `json:"token_id"`
}

type TokenRevokedEvent struct {
	BaseEvent
	Data TokenRevokedData `json:"data"`
}

type TokenRevokedData struct {
	UserID         uuid.UUID `json:"user_id"`
	OrganizationID uuid.UUID `json:"organization_id"`
	TokenID        string    `json:"token_id"`
	TokenType      string    `json:"token_type"` // "access", "refresh"
	RevokedBy      uuid.UUID `json:"revoked_by"`
	Reason         string    `json:"reason"`
}

// Role and permission events
type RoleAssignedEvent struct {
	BaseEvent
	Data RoleAssignedData `json:"data"`
}

type RoleAssignedData struct {
	UserID         uuid.UUID `json:"user_id"`
	RoleID         uuid.UUID `json:"role_id"`
	RoleName       string    `json:"role_name"`
	OrganizationID uuid.UUID `json:"organization_id"`
	AssignedBy     uuid.UUID `json:"assigned_by"`
}

type RoleRevokedEvent struct {
	BaseEvent
	Data RoleRevokedData `json:"data"`
}

type RoleRevokedData struct {
	UserID         uuid.UUID `json:"user_id"`
	RoleID         uuid.UUID `json:"role_id"`
	RoleName       string    `json:"role_name"`
	OrganizationID uuid.UUID `json:"organization_id"`
	RevokedBy      uuid.UUID `json:"revoked_by"`
}

// 2FA events
type TwoFAEnabledEvent struct {
	BaseEvent
	Data TwoFAEnabledData `json:"data"`
}

type TwoFAEnabledData struct {
	UserID         uuid.UUID `json:"user_id"`
	OrganizationID uuid.UUID `json:"organization_id"`
	Method         string    `json:"method"` // "totp", "sms", etc.
}

type TwoFADisabledEvent struct {
	BaseEvent
	Data TwoFADisabledData `json:"data"`
}

type TwoFADisabledData struct {
	UserID         uuid.UUID `json:"user_id"`
	OrganizationID uuid.UUID `json:"organization_id"`
	DisabledBy     uuid.UUID `json:"disabled_by"`
}

// Organization events
type OrganizationCreatedEvent struct {
	BaseEvent
	Data OrganizationCreatedData `json:"data"`
}

type OrganizationCreatedData struct {
	OrganizationID uuid.UUID `json:"organization_id"`
	Name           string    `json:"name"`
	Domain         string    `json:"domain"`
	CreatedBy      uuid.UUID `json:"created_by"`
}

type OrganizationUpdatedEvent struct {
	BaseEvent
	Data OrganizationUpdatedData `json:"data"`
}

type OrganizationUpdatedData struct {
	OrganizationID uuid.UUID `json:"organization_id"`
	UpdatedBy      uuid.UUID `json:"updated_by"`
	Changes        []string  `json:"changes"` // List of changed fields
}