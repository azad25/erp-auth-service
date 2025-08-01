package interfaces

import (
	"context"
	"time"

	"github.com/google/uuid"
	"erp-auth-service/internal/models"
)

// UserRepository defines the interface for user data operations with performance optimizations
type UserRepository interface {
	// Basic CRUD operations
	GetByID(ctx context.Context, id uuid.UUID) (*models.User, error)
	GetByEmail(ctx context.Context, email string) (*models.User, error)
	Create(ctx context.Context, user *models.User) error
	Update(ctx context.Context, user *models.User) error
	Delete(ctx context.Context, id uuid.UUID) error
	
	// Performance optimized queries
	GetByEmailWithOrganization(ctx context.Context, email string) (*models.User, error)
	GetUserPermissions(ctx context.Context, userID uuid.UUID) ([]models.Permission, error)
	GetUserRoles(ctx context.Context, userID uuid.UUID) ([]models.Role, error)
	GetActiveUsersByOrganization(ctx context.Context, orgID uuid.UUID, limit, offset int) ([]models.User, error)
	
	// Bulk operations for high-throughput scenarios
	GetUsersByIDs(ctx context.Context, ids []uuid.UUID) ([]models.User, error)
	UpdateLastLogin(ctx context.Context, userID uuid.UUID, loginTime time.Time) error
	
	// Security operations
	IncrementFailedLoginAttempts(ctx context.Context, userID uuid.UUID) error
	ResetFailedLoginAttempts(ctx context.Context, userID uuid.UUID) error
	LockUser(ctx context.Context, userID uuid.UUID, lockUntil time.Time) error
}

// OrganizationRepository defines the interface for organization data operations
type OrganizationRepository interface {
	// Basic CRUD operations
	GetByID(ctx context.Context, id uuid.UUID) (*models.Organization, error)
	GetByDomain(ctx context.Context, domain string) (*models.Organization, error)
	Create(ctx context.Context, org *models.Organization) error
	Update(ctx context.Context, org *models.Organization) error
	Delete(ctx context.Context, id uuid.UUID) error
	
	// Performance queries
	GetActiveOrganizations(ctx context.Context, limit, offset int) ([]models.Organization, error)
	GetOrganizationWithUsers(ctx context.Context, id uuid.UUID) (*models.Organization, error)
	GetUserCount(ctx context.Context, orgID uuid.UUID) (int, error)
}

// RoleRepository defines the interface for role data operations
type RoleRepository interface {
	// Basic CRUD operations
	GetByID(ctx context.Context, id uuid.UUID) (*models.Role, error)
	GetByName(ctx context.Context, name string, orgID uuid.UUID) (*models.Role, error)
	Create(ctx context.Context, role *models.Role) error
	Update(ctx context.Context, role *models.Role) error
	Delete(ctx context.Context, id uuid.UUID) error
	
	// Performance queries
	GetRolesByOrganization(ctx context.Context, orgID uuid.UUID) ([]models.Role, error)
	GetRoleWithPermissions(ctx context.Context, roleID uuid.UUID) (*models.Role, error)
	GetRolePermissions(ctx context.Context, roleID uuid.UUID) ([]models.Permission, error)
	
	// Role assignment operations
	AssignPermissionToRole(ctx context.Context, roleID, permissionID uuid.UUID) error
	RevokePermissionFromRole(ctx context.Context, roleID, permissionID uuid.UUID) error
	AssignRoleToUser(ctx context.Context, userID, roleID uuid.UUID) error
	RevokeRoleFromUser(ctx context.Context, userID, roleID uuid.UUID) error
}

// PermissionRepository defines the interface for permission data operations
type PermissionRepository interface {
	// Basic CRUD operations
	GetByID(ctx context.Context, id uuid.UUID) (*models.Permission, error)
	GetByName(ctx context.Context, name string) (*models.Permission, error)
	GetByResourceAndAction(ctx context.Context, resource, action string) (*models.Permission, error)
	Create(ctx context.Context, permission *models.Permission) error
	Update(ctx context.Context, permission *models.Permission) error
	Delete(ctx context.Context, id uuid.UUID) error
	
	// Performance queries
	GetPermissionsByResource(ctx context.Context, resource string) ([]models.Permission, error)
	GetPermissionsByRole(ctx context.Context, roleID uuid.UUID) ([]models.Permission, error)
	GetUserPermissions(ctx context.Context, userID uuid.UUID) ([]models.Permission, error)
	
	// Hierarchical permissions
	GetChildPermissions(ctx context.Context, parentID uuid.UUID) ([]models.Permission, error)
	GetPermissionHierarchy(ctx context.Context, permissionID uuid.UUID) ([]models.Permission, error)
}

// TokenRepository defines the interface for token data operations
type TokenRepository interface {
	// Basic CRUD operations
	GetByID(ctx context.Context, id uuid.UUID) (*models.Token, error)
	GetByTokenHash(ctx context.Context, tokenHash string) (*models.Token, error)
	Create(ctx context.Context, token *models.Token) error
	Update(ctx context.Context, token *models.Token) error
	Delete(ctx context.Context, id uuid.UUID) error
	
	// Token management
	RevokeToken(ctx context.Context, tokenHash string) error
	RevokeAllUserTokens(ctx context.Context, userID uuid.UUID, tokenType models.TokenType) error
	GetActiveTokensByUser(ctx context.Context, userID uuid.UUID, tokenType models.TokenType) ([]models.Token, error)
	
	// Cleanup operations
	CleanupExpiredTokens(ctx context.Context) error
	DeleteRevokedTokens(ctx context.Context, olderThan time.Time) error
}

// UserRoleRepository defines the interface for user-role relationship operations
type UserRoleRepository interface {
	// Basic operations
	Create(ctx context.Context, userRole *models.UserRole) error
	Delete(ctx context.Context, userID, roleID uuid.UUID) error
	GetByUserID(ctx context.Context, userID uuid.UUID) ([]models.UserRole, error)
	GetByRoleID(ctx context.Context, roleID uuid.UUID) ([]models.UserRole, error)
	
	// Bulk operations
	AssignRolesToUser(ctx context.Context, userID uuid.UUID, roleIDs []uuid.UUID) error
	RemoveRolesFromUser(ctx context.Context, userID uuid.UUID, roleIDs []uuid.UUID) error
}

// RolePermissionRepository defines the interface for role-permission relationship operations
type RolePermissionRepository interface {
	// Basic operations
	Create(ctx context.Context, rolePermission *models.RolePermission) error
	Delete(ctx context.Context, roleID, permissionID uuid.UUID) error
	GetByRoleID(ctx context.Context, roleID uuid.UUID) ([]models.RolePermission, error)
	GetByPermissionID(ctx context.Context, permissionID uuid.UUID) ([]models.RolePermission, error)
	
	// Bulk operations
	AssignPermissionsToRole(ctx context.Context, roleID uuid.UUID, permissionIDs []uuid.UUID) error
	RemovePermissionsFromRole(ctx context.Context, roleID uuid.UUID, permissionIDs []uuid.UUID) error
}