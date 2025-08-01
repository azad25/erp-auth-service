package interfaces

import (
	"github.com/gin-gonic/gin"
)

// AuthHandler defines the interface for authentication HTTP handlers
type AuthHandler interface {
	Login(c *gin.Context)
	Logout(c *gin.Context)
	RefreshToken(c *gin.Context)
	ResetPassword(c *gin.Context)
	ConfirmPasswordReset(c *gin.Context)
	ChangePassword(c *gin.Context)
	EnableTwoFactor(c *gin.Context)
	VerifyTwoFactor(c *gin.Context)
	DisableTwoFactor(c *gin.Context)
	ValidateToken(c *gin.Context)
}

// UserHandler defines the interface for user management HTTP handlers
type UserHandler interface {
	CreateUser(c *gin.Context)
	GetUser(c *gin.Context)
	GetCurrentUser(c *gin.Context)
	UpdateUser(c *gin.Context)
	DeleteUser(c *gin.Context)
	ActivateUser(c *gin.Context)
	DeactivateUser(c *gin.Context)
	GetUserPermissions(c *gin.Context)
	GetUserRoles(c *gin.Context)
}

// OrganizationHandler defines the interface for organization management HTTP handlers
type OrganizationHandler interface {
	CreateOrganization(c *gin.Context)
	GetOrganization(c *gin.Context)
	UpdateOrganization(c *gin.Context)
	DeleteOrganization(c *gin.Context)
	UpdateSettings(c *gin.Context)
	GetStats(c *gin.Context)
}

// RoleHandler defines the interface for role management HTTP handlers
type RoleHandler interface {
	CreateRole(c *gin.Context)
	GetRole(c *gin.Context)
	UpdateRole(c *gin.Context)
	DeleteRole(c *gin.Context)
	AssignPermission(c *gin.Context)
	RevokePermission(c *gin.Context)
	AssignToUser(c *gin.Context)
	RevokeFromUser(c *gin.Context)
	GetRolePermissions(c *gin.Context)
}

// PermissionHandler defines the interface for permission management HTTP handlers
type PermissionHandler interface {
	CreatePermission(c *gin.Context)
	GetPermission(c *gin.Context)
	UpdatePermission(c *gin.Context)
	DeletePermission(c *gin.Context)
	GetHierarchy(c *gin.Context)
	CheckUserPermission(c *gin.Context)
}

// HealthHandler defines the interface for health check handlers
type HealthHandler interface {
	Health(c *gin.Context)
	Ready(c *gin.Context)
	Live(c *gin.Context)
}