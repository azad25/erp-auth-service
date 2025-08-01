package interfaces

import (
	"context"
	"time"

	"github.com/google/uuid"
	"erp-auth-service/internal/domain/entities"
)

// AuthService defines the core authentication business logic interface
type AuthService interface {
	Login(ctx context.Context, email, password string, ipAddress, userAgent string) (*entities.TokenPair, error)
	Logout(ctx context.Context, tokenHash string) error
	RefreshToken(ctx context.Context, refreshToken string, ipAddress, userAgent string) (*entities.TokenPair, error)
	ValidateToken(ctx context.Context, tokenHash string) (*entities.User, error)
	ResetPassword(ctx context.Context, email string) error
	ConfirmPasswordReset(ctx context.Context, token, newPassword string) error
	ChangePassword(ctx context.Context, userID uuid.UUID, oldPassword, newPassword string) error
	EnableTwoFactor(ctx context.Context, userID uuid.UUID) (string, []string, error) // secret, backup codes
	VerifyTwoFactor(ctx context.Context, userID uuid.UUID, code string) error
	DisableTwoFactor(ctx context.Context, userID uuid.UUID, password string) error
}

// UserService defines the user management business logic interface
type UserService interface {
	CreateUser(ctx context.Context, user *entities.User, password string) error
	GetUser(ctx context.Context, userID uuid.UUID) (*entities.User, error)
	GetUserByEmail(ctx context.Context, email string) (*entities.User, error)
	UpdateUser(ctx context.Context, user *entities.User) error
	DeleteUser(ctx context.Context, userID uuid.UUID) error
	ActivateUser(ctx context.Context, userID uuid.UUID) error
	DeactivateUser(ctx context.Context, userID uuid.UUID) error
	VerifyUser(ctx context.Context, userID uuid.UUID) error
	GetUserPermissions(ctx context.Context, userID uuid.UUID) ([]entities.Permission, error)
	GetUserRoles(ctx context.Context, userID uuid.UUID) ([]entities.Role, error)
}

// OrganizationService defines the organization management business logic interface
type OrganizationService interface {
	CreateOrganization(ctx context.Context, org *entities.Organization) error
	GetOrganization(ctx context.Context, orgID uuid.UUID) (*entities.Organization, error)
	GetOrganizationByDomain(ctx context.Context, domain string) (*entities.Organization, error)
	UpdateOrganization(ctx context.Context, org *entities.Organization) error
	DeleteOrganization(ctx context.Context, orgID uuid.UUID) error
	UpdateSettings(ctx context.Context, orgID uuid.UUID, settings entities.OrganizationSettings) error
	GetUserCount(ctx context.Context, orgID uuid.UUID) (int, error)
	CanAddUser(ctx context.Context, orgID uuid.UUID) (bool, error)
}

// PermissionService defines the permission management business logic interface
type PermissionService interface {
	CreatePermission(ctx context.Context, permission *entities.Permission) error
	GetPermission(ctx context.Context, permissionID uuid.UUID) (*entities.Permission, error)
	UpdatePermission(ctx context.Context, permission *entities.Permission) error
	DeletePermission(ctx context.Context, permissionID uuid.UUID) error
	GetPermissionHierarchy(ctx context.Context, parentID *uuid.UUID) ([]entities.Permission, error)
	CheckUserPermission(ctx context.Context, userID uuid.UUID, resource, action string) (bool, error)
	GetUserEffectivePermissions(ctx context.Context, userID uuid.UUID) ([]entities.Permission, error)
}

// RoleService defines the role management business logic interface
type RoleService interface {
	CreateRole(ctx context.Context, role *entities.Role) error
	GetRole(ctx context.Context, roleID uuid.UUID) (*entities.Role, error)
	UpdateRole(ctx context.Context, role *entities.Role) error
	DeleteRole(ctx context.Context, roleID uuid.UUID) error
	AssignPermissionToRole(ctx context.Context, roleID, permissionID, grantedBy uuid.UUID) error
	RevokePermissionFromRole(ctx context.Context, roleID, permissionID uuid.UUID) error
	AssignRoleToUser(ctx context.Context, userID, roleID, assignedBy uuid.UUID, expiresAt *time.Time) error
	RevokeRoleFromUser(ctx context.Context, userID, roleID uuid.UUID) error
	GetRolePermissions(ctx context.Context, roleID uuid.UUID) ([]entities.Permission, error)
}

// TokenService defines the token management business logic interface
type TokenService interface {
	GenerateTokenPair(ctx context.Context, userID uuid.UUID, ipAddress, userAgent string) (*entities.TokenPair, error)
	ValidateAccessToken(ctx context.Context, tokenString string) (*entities.User, error)
	ValidateRefreshToken(ctx context.Context, tokenString string) (*entities.Token, error)
	RevokeToken(ctx context.Context, tokenHash string) error
	RevokeAllUserTokens(ctx context.Context, userID uuid.UUID, tokenType entities.TokenType) error
	GenerateResetToken(ctx context.Context, userID uuid.UUID) (string, error)
	ValidateResetToken(ctx context.Context, tokenString string) (*entities.User, error)
	CleanupExpiredTokens(ctx context.Context) error
}

// CacheService defines the caching interface for performance optimization
type CacheService interface {
	Get(ctx context.Context, key string) (string, error)
	Set(ctx context.Context, key string, value string, expiration time.Duration) error
	Delete(ctx context.Context, key string) error
	SetUserPermissions(ctx context.Context, userID uuid.UUID, permissions []entities.Permission, expiration time.Duration) error
	GetUserPermissions(ctx context.Context, userID uuid.UUID) ([]entities.Permission, error)
	InvalidateUserCache(ctx context.Context, userID uuid.UUID) error
}

// EventService defines the interface for publishing domain events
type EventService interface {
	PublishUserCreated(ctx context.Context, user *entities.User) error
	PublishUserUpdated(ctx context.Context, user *entities.User) error
	PublishUserDeleted(ctx context.Context, userID uuid.UUID) error
	PublishUserLoggedIn(ctx context.Context, userID uuid.UUID, ipAddress, userAgent string) error
	PublishUserLoggedOut(ctx context.Context, userID uuid.UUID) error
	PublishPasswordChanged(ctx context.Context, userID uuid.UUID) error
	PublishTwoFactorEnabled(ctx context.Context, userID uuid.UUID) error
	PublishTwoFactorDisabled(ctx context.Context, userID uuid.UUID) error
}