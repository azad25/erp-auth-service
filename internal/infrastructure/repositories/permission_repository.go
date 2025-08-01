package repositories

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"erp-auth-service/internal/domain/interfaces"
	"erp-auth-service/internal/models"
)

// permissionRepository implements the PermissionRepository interface
type permissionRepository struct {
	db     *gorm.DB
	readDB *gorm.DB // Read replica for read operations
}

// NewPermissionRepository creates a new permission repository
func NewPermissionRepository(db, readDB *gorm.DB) interfaces.PermissionRepository {
	return &permissionRepository{
		db:     db,
		readDB: readDB,
	}
}

// GetByID retrieves a permission by ID
func (r *permissionRepository) GetByID(ctx context.Context, id uuid.UUID) (*models.Permission, error) {
	var permission models.Permission
	err := r.readDB.WithContext(ctx).
		Where("id = ?", id).
		First(&permission).Error
	
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, fmt.Errorf("permission not found")
		}
		return nil, fmt.Errorf("failed to get permission by ID: %w", err)
	}
	
	return &permission, nil
}

// GetByName retrieves a permission by name
func (r *permissionRepository) GetByName(ctx context.Context, name string) (*models.Permission, error) {
	var permission models.Permission
	err := r.readDB.WithContext(ctx).
		Where("name = ?", name).
		First(&permission).Error
	
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, fmt.Errorf("permission not found")
		}
		return nil, fmt.Errorf("failed to get permission by name: %w", err)
	}
	
	return &permission, nil
}

// GetByResourceAndAction retrieves a permission by resource and action
func (r *permissionRepository) GetByResourceAndAction(ctx context.Context, resource, action string) (*models.Permission, error) {
	var permission models.Permission
	err := r.readDB.WithContext(ctx).
		Where("resource = ? AND action = ?", resource, action).
		First(&permission).Error
	
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, fmt.Errorf("permission not found")
		}
		return nil, fmt.Errorf("failed to get permission by resource and action: %w", err)
	}
	
	return &permission, nil
}

// Create creates a new permission
func (r *permissionRepository) Create(ctx context.Context, permission *models.Permission) error {
	err := r.db.WithContext(ctx).Create(permission).Error
	if err != nil {
		return fmt.Errorf("failed to create permission: %w", err)
	}
	return nil
}

// Update updates an existing permission
func (r *permissionRepository) Update(ctx context.Context, permission *models.Permission) error {
	err := r.db.WithContext(ctx).Save(permission).Error
	if err != nil {
		return fmt.Errorf("failed to update permission: %w", err)
	}
	return nil
}

// Delete deletes a permission
func (r *permissionRepository) Delete(ctx context.Context, id uuid.UUID) error {
	err := r.db.WithContext(ctx).
		Where("id = ?", id).
		Delete(&models.Permission{}).Error
	
	if err != nil {
		return fmt.Errorf("failed to delete permission: %w", err)
	}
	return nil
}

// GetPermissionsByResource retrieves all permissions for a specific resource
func (r *permissionRepository) GetPermissionsByResource(ctx context.Context, resource string) ([]models.Permission, error) {
	var permissions []models.Permission
	err := r.readDB.WithContext(ctx).
		Where("resource = ?", resource).
		Order("action ASC").
		Find(&permissions).Error
	
	if err != nil {
		return nil, fmt.Errorf("failed to get permissions by resource: %w", err)
	}
	
	return permissions, nil
}

// GetPermissionsByRole retrieves all permissions for a specific role
func (r *permissionRepository) GetPermissionsByRole(ctx context.Context, roleID uuid.UUID) ([]models.Permission, error) {
	var permissions []models.Permission
	
	err := r.readDB.WithContext(ctx).
		Joins("JOIN role_permissions rp ON permissions.id = rp.permission_id").
		Where("rp.role_id = ?", roleID).
		Order("resource ASC, action ASC").
		Find(&permissions).Error
	
	if err != nil {
		return nil, fmt.Errorf("failed to get permissions by role: %w", err)
	}
	
	return permissions, nil
}

// GetUserPermissions retrieves all permissions for a user through their roles
func (r *permissionRepository) GetUserPermissions(ctx context.Context, userID uuid.UUID) ([]models.Permission, error) {
	var permissions []models.Permission
	
	// Use optimized query with joins
	query := `
		SELECT DISTINCT p.id, p.name, p.resource, p.action, p.scope, p.parent_id,
		       p.description, p.is_system, p.created_at, p.updated_at
		FROM permissions p
		JOIN role_permissions rp ON p.id = rp.permission_id
		JOIN user_roles ur ON rp.role_id = ur.role_id
		JOIN roles r ON ur.role_id = r.id
		WHERE ur.user_id = ? AND r.is_active = true
		ORDER BY p.resource, p.action
	`
	
	err := r.readDB.WithContext(ctx).Raw(query, userID).Scan(&permissions).Error
	if err != nil {
		return nil, fmt.Errorf("failed to get user permissions: %w", err)
	}
	
	return permissions, nil
}

// GetChildPermissions retrieves all child permissions for a parent permission
func (r *permissionRepository) GetChildPermissions(ctx context.Context, parentID uuid.UUID) ([]models.Permission, error) {
	var permissions []models.Permission
	err := r.readDB.WithContext(ctx).
		Where("parent_id = ?", parentID).
		Order("name ASC").
		Find(&permissions).Error
	
	if err != nil {
		return nil, fmt.Errorf("failed to get child permissions: %w", err)
	}
	
	return permissions, nil
}

// GetPermissionHierarchy retrieves the full hierarchy for a permission (recursive)
func (r *permissionRepository) GetPermissionHierarchy(ctx context.Context, permissionID uuid.UUID) ([]models.Permission, error) {
	var permissions []models.Permission
	
	// Use recursive CTE to get the full hierarchy
	query := `
		WITH RECURSIVE permission_hierarchy AS (
			-- Base case: start with the given permission
			SELECT id, name, resource, action, scope, parent_id, description, is_system, created_at, updated_at, 0 as level
			FROM permissions
			WHERE id = ?
			
			UNION ALL
			
			-- Recursive case: get children
			SELECT p.id, p.name, p.resource, p.action, p.scope, p.parent_id, p.description, p.is_system, p.created_at, p.updated_at, ph.level + 1
			FROM permissions p
			JOIN permission_hierarchy ph ON p.parent_id = ph.id
		)
		SELECT id, name, resource, action, scope, parent_id, description, is_system, created_at, updated_at
		FROM permission_hierarchy
		ORDER BY level, name
	`
	
	err := r.readDB.WithContext(ctx).Raw(query, permissionID).Scan(&permissions).Error
	if err != nil {
		return nil, fmt.Errorf("failed to get permission hierarchy: %w", err)
	}
	
	return permissions, nil
}