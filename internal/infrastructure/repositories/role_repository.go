package repositories

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"erp-auth-service/internal/domain/interfaces"
	"erp-auth-service/internal/models"
)

// roleRepository implements the RoleRepository interface
type roleRepository struct {
	db     *gorm.DB
	readDB *gorm.DB // Read replica for read operations
}

// NewRoleRepository creates a new role repository
func NewRoleRepository(db, readDB *gorm.DB) interfaces.RoleRepository {
	return &roleRepository{
		db:     db,
		readDB: readDB,
	}
}

// GetByID retrieves a role by ID
func (r *roleRepository) GetByID(ctx context.Context, id uuid.UUID) (*models.Role, error) {
	var role models.Role
	err := r.readDB.WithContext(ctx).
		Where("id = ? AND is_active = ?", id, true).
		First(&role).Error
	
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, fmt.Errorf("role not found")
		}
		return nil, fmt.Errorf("failed to get role by ID: %w", err)
	}
	
	return &role, nil
}

// GetByName retrieves a role by name within an organization
func (r *roleRepository) GetByName(ctx context.Context, name string, orgID uuid.UUID) (*models.Role, error) {
	var role models.Role
	err := r.readDB.WithContext(ctx).
		Where("name = ? AND organization_id = ? AND is_active = ?", name, orgID, true).
		First(&role).Error
	
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, fmt.Errorf("role not found")
		}
		return nil, fmt.Errorf("failed to get role by name: %w", err)
	}
	
	return &role, nil
}

// Create creates a new role
func (r *roleRepository) Create(ctx context.Context, role *models.Role) error {
	err := r.db.WithContext(ctx).Create(role).Error
	if err != nil {
		return fmt.Errorf("failed to create role: %w", err)
	}
	return nil
}

// Update updates an existing role
func (r *roleRepository) Update(ctx context.Context, role *models.Role) error {
	err := r.db.WithContext(ctx).Save(role).Error
	if err != nil {
		return fmt.Errorf("failed to update role: %w", err)
	}
	return nil
}

// Delete soft deletes a role
func (r *roleRepository) Delete(ctx context.Context, id uuid.UUID) error {
	err := r.db.WithContext(ctx).
		Model(&models.Role{}).
		Where("id = ?", id).
		Update("is_active", false).Error
	
	if err != nil {
		return fmt.Errorf("failed to delete role: %w", err)
	}
	return nil
}

// GetRolesByOrganization retrieves all roles for an organization
func (r *roleRepository) GetRolesByOrganization(ctx context.Context, orgID uuid.UUID) ([]models.Role, error) {
	var roles []models.Role
	err := r.readDB.WithContext(ctx).
		Where("organization_id = ? AND is_active = ?", orgID, true).
		Order("name ASC").
		Find(&roles).Error
	
	if err != nil {
		return nil, fmt.Errorf("failed to get roles by organization: %w", err)
	}
	
	return roles, nil
}

// GetRoleWithPermissions retrieves a role with its permissions
func (r *roleRepository) GetRoleWithPermissions(ctx context.Context, roleID uuid.UUID) (*models.Role, error) {
	var role models.Role
	err := r.readDB.WithContext(ctx).
		Preload("RolePermissions.Permission").
		Where("id = ? AND is_active = ?", roleID, true).
		First(&role).Error
	
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, fmt.Errorf("role not found")
		}
		return nil, fmt.Errorf("failed to get role with permissions: %w", err)
	}
	
	return &role, nil
}

// GetRolePermissions retrieves all permissions for a role
func (r *roleRepository) GetRolePermissions(ctx context.Context, roleID uuid.UUID) ([]models.Permission, error) {
	var permissions []models.Permission
	
	err := r.readDB.WithContext(ctx).
		Joins("JOIN role_permissions rp ON permissions.id = rp.permission_id").
		Where("rp.role_id = ?", roleID).
		Find(&permissions).Error
	
	if err != nil {
		return nil, fmt.Errorf("failed to get role permissions: %w", err)
	}
	
	return permissions, nil
}

// AssignPermissionToRole assigns a permission to a role
func (r *roleRepository) AssignPermissionToRole(ctx context.Context, roleID, permissionID uuid.UUID) error {
	rolePermission := &models.RolePermission{
		RoleID:       roleID,
		PermissionID: permissionID,
	}
	
	err := r.db.WithContext(ctx).Create(rolePermission).Error
	if err != nil {
		return fmt.Errorf("failed to assign permission to role: %w", err)
	}
	return nil
}

// RevokePermissionFromRole removes a permission from a role
func (r *roleRepository) RevokePermissionFromRole(ctx context.Context, roleID, permissionID uuid.UUID) error {
	err := r.db.WithContext(ctx).
		Where("role_id = ? AND permission_id = ?", roleID, permissionID).
		Delete(&models.RolePermission{}).Error
	
	if err != nil {
		return fmt.Errorf("failed to revoke permission from role: %w", err)
	}
	return nil
}

// AssignRoleToUser assigns a role to a user
func (r *roleRepository) AssignRoleToUser(ctx context.Context, userID, roleID uuid.UUID) error {
	userRole := &models.UserRole{
		UserID: userID,
		RoleID: roleID,
	}
	
	err := r.db.WithContext(ctx).Create(userRole).Error
	if err != nil {
		return fmt.Errorf("failed to assign role to user: %w", err)
	}
	return nil
}

// RevokeRoleFromUser removes a role from a user
func (r *roleRepository) RevokeRoleFromUser(ctx context.Context, userID, roleID uuid.UUID) error {
	err := r.db.WithContext(ctx).
		Where("user_id = ? AND role_id = ?", userID, roleID).
		Delete(&models.UserRole{}).Error
	
	if err != nil {
		return fmt.Errorf("failed to revoke role from user: %w", err)
	}
	return nil
}