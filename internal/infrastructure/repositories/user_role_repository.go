package repositories

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"erp-auth-service/internal/domain/interfaces"
	"erp-auth-service/internal/models"
)

// userRoleRepository implements the UserRoleRepository interface
type userRoleRepository struct {
	db     *gorm.DB
	readDB *gorm.DB // Read replica for read operations
}

// NewUserRoleRepository creates a new user role repository
func NewUserRoleRepository(db, readDB *gorm.DB) interfaces.UserRoleRepository {
	return &userRoleRepository{
		db:     db,
		readDB: readDB,
	}
}

// Create creates a new user-role relationship
func (r *userRoleRepository) Create(ctx context.Context, userRole *models.UserRole) error {
	err := r.db.WithContext(ctx).Create(userRole).Error
	if err != nil {
		return fmt.Errorf("failed to create user role: %w", err)
	}
	return nil
}

// Delete removes a user-role relationship
func (r *userRoleRepository) Delete(ctx context.Context, userID, roleID uuid.UUID) error {
	err := r.db.WithContext(ctx).
		Where("user_id = ? AND role_id = ?", userID, roleID).
		Delete(&models.UserRole{}).Error
	
	if err != nil {
		return fmt.Errorf("failed to delete user role: %w", err)
	}
	return nil
}

// GetByUserID retrieves all user-role relationships for a user
func (r *userRoleRepository) GetByUserID(ctx context.Context, userID uuid.UUID) ([]models.UserRole, error) {
	var userRoles []models.UserRole
	err := r.readDB.WithContext(ctx).
		Preload("Role").
		Where("user_id = ?", userID).
		Find(&userRoles).Error
	
	if err != nil {
		return nil, fmt.Errorf("failed to get user roles by user ID: %w", err)
	}
	
	return userRoles, nil
}

// GetByRoleID retrieves all user-role relationships for a role
func (r *userRoleRepository) GetByRoleID(ctx context.Context, roleID uuid.UUID) ([]models.UserRole, error) {
	var userRoles []models.UserRole
	err := r.readDB.WithContext(ctx).
		Preload("User").
		Where("role_id = ?", roleID).
		Find(&userRoles).Error
	
	if err != nil {
		return nil, fmt.Errorf("failed to get user roles by role ID: %w", err)
	}
	
	return userRoles, nil
}

// AssignRolesToUser assigns multiple roles to a user in a single transaction
func (r *userRoleRepository) AssignRolesToUser(ctx context.Context, userID uuid.UUID, roleIDs []uuid.UUID) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for _, roleID := range roleIDs {
			userRole := &models.UserRole{
				UserID: userID,
				RoleID: roleID,
			}
			
			// Use ON CONFLICT to handle duplicates gracefully
			err := tx.Create(userRole).Error
			if err != nil {
				// Check if it's a duplicate key error, if so continue
				if isDuplicateKeyError(err) {
					continue
				}
				return fmt.Errorf("failed to assign role %s to user: %w", roleID, err)
			}
		}
		return nil
	})
}

// RemoveRolesFromUser removes multiple roles from a user in a single transaction
func (r *userRoleRepository) RemoveRolesFromUser(ctx context.Context, userID uuid.UUID, roleIDs []uuid.UUID) error {
	err := r.db.WithContext(ctx).
		Where("user_id = ? AND role_id IN ?", userID, roleIDs).
		Delete(&models.UserRole{}).Error
	
	if err != nil {
		return fmt.Errorf("failed to remove roles from user: %w", err)
	}
	return nil
}

// isDuplicateKeyError checks if the error is a duplicate key constraint violation
func isDuplicateKeyError(err error) bool {
	// This is a simplified check - in production, you'd want more robust error checking
	return err != nil && (
		err.Error() == "UNIQUE constraint failed" ||
		err.Error() == "duplicate key value violates unique constraint")
}