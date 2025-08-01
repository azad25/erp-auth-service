package repositories

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"erp-auth-service/internal/domain/interfaces"
	"erp-auth-service/internal/models"
)

// rolePermissionRepository implements the RolePermissionRepository interface
type rolePermissionRepository struct {
	db     *gorm.DB
	readDB *gorm.DB // Read replica for read operations
}

// NewRolePermissionRepository creates a new role permission repository
func NewRolePermissionRepository(db, readDB *gorm.DB) interfaces.RolePermissionRepository {
	return &rolePermissionRepository{
		db:     db,
		readDB: readDB,
	}
}

// Create creates a new role-permission relationship
func (r *rolePermissionRepository) Create(ctx context.Context, rolePermission *models.RolePermission) error {
	err := r.db.WithContext(ctx).Create(rolePermission).Error
	if err != nil {
		return fmt.Errorf("failed to create role permission: %w", err)
	}
	return nil
}

// Delete removes a role-permission relationship
func (r *rolePermissionRepository) Delete(ctx context.Context, roleID, permissionID uuid.UUID) error {
	err := r.db.WithContext(ctx).
		Where("role_id = ? AND permission_id = ?", roleID, permissionID).
		Delete(&models.RolePermission{}).Error
	
	if err != nil {
		return fmt.Errorf("failed to delete role permission: %w", err)
	}
	return nil
}

// GetByRoleID retrieves all role-permission relationships for a role
func (r *rolePermissionRepository) GetByRoleID(ctx context.Context, roleID uuid.UUID) ([]models.RolePermission, error) {
	var rolePermissions []models.RolePermission
	err := r.readDB.WithContext(ctx).
		Preload("Permission").
		Where("role_id = ?", roleID).
		Find(&rolePermissions).Error
	
	if err != nil {
		return nil, fmt.Errorf("failed to get role permissions by role ID: %w", err)
	}
	
	return rolePermissions, nil
}

// GetByPermissionID retrieves all role-permission relationships for a permission
func (r *rolePermissionRepository) GetByPermissionID(ctx context.Context, permissionID uuid.UUID) ([]models.RolePermission, error) {
	var rolePermissions []models.RolePermission
	err := r.readDB.WithContext(ctx).
		Preload("Role").
		Where("permission_id = ?", permissionID).
		Find(&rolePermissions).Error
	
	if err != nil {
		return nil, fmt.Errorf("failed to get role permissions by permission ID: %w", err)
	}
	
	return rolePermissions, nil
}

// AssignPermissionsToRole assigns multiple permissions to a role in a single transaction
func (r *rolePermissionRepository) AssignPermissionsToRole(ctx context.Context, roleID uuid.UUID, permissionIDs []uuid.UUID) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for _, permissionID := range permissionIDs {
			rolePermission := &models.RolePermission{
				RoleID:       roleID,
				PermissionID: permissionID,
			}
			
			// Use ON CONFLICT to handle duplicates gracefully
			err := tx.Create(rolePermission).Error
			if err != nil {
				// Check if it's a duplicate key error, if so continue
				if isDuplicateKeyError(err) {
					continue
				}
				return fmt.Errorf("failed to assign permission %s to role: %w", permissionID, err)
			}
		}
		return nil
	})
}

// RemovePermissionsFromRole removes multiple permissions from a role in a single transaction
func (r *rolePermissionRepository) RemovePermissionsFromRole(ctx context.Context, roleID uuid.UUID, permissionIDs []uuid.UUID) error {
	err := r.db.WithContext(ctx).
		Where("role_id = ? AND permission_id IN ?", roleID, permissionIDs).
		Delete(&models.RolePermission{}).Error
	
	if err != nil {
		return fmt.Errorf("failed to remove permissions from role: %w", err)
	}
	return nil
}