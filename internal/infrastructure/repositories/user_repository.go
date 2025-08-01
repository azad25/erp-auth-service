package repositories

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"erp-auth-service/internal/domain/interfaces"
	"erp-auth-service/internal/models"
)

// userRepository implements the UserRepository interface with performance optimizations
type userRepository struct {
	db       *gorm.DB
	readDB   *gorm.DB // Read replica for read operations
}

// NewUserRepository creates a new user repository with connection pooling
func NewUserRepository(db, readDB *gorm.DB) interfaces.UserRepository {
	return &userRepository{
		db:     db,
		readDB: readDB,
	}
}

// GetByID retrieves a user by ID using read replica
func (r *userRepository) GetByID(ctx context.Context, id uuid.UUID) (*models.User, error) {
	var user models.User
	err := r.readDB.WithContext(ctx).
		Where("id = ? AND is_active = ?", id, true).
		First(&user).Error
	
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, fmt.Errorf("user not found")
		}
		return nil, fmt.Errorf("failed to get user by ID: %w", err)
	}
	
	return &user, nil
}

// GetByEmail retrieves a user by email using optimized query
func (r *userRepository) GetByEmail(ctx context.Context, email string) (*models.User, error) {
	var user models.User
	err := r.readDB.WithContext(ctx).
		Where("email = ? AND is_active = ?", email, true).
		First(&user).Error
	
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, fmt.Errorf("user not found")
		}
		return nil, fmt.Errorf("failed to get user by email: %w", err)
	}
	
	return &user, nil
}

// GetByEmailWithOrganization retrieves user with organization data in single query
func (r *userRepository) GetByEmailWithOrganization(ctx context.Context, email string) (*models.User, error) {
	var user models.User
	err := r.readDB.WithContext(ctx).
		Preload("Organization").
		Where("email = ? AND is_active = ?", email, true).
		First(&user).Error
	
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, fmt.Errorf("user not found")
		}
		return nil, fmt.Errorf("failed to get user with organization: %w", err)
	}
	
	return &user, nil
}

// Create creates a new user using write database
func (r *userRepository) Create(ctx context.Context, user *models.User) error {
	err := r.db.WithContext(ctx).Create(user).Error
	if err != nil {
		return fmt.Errorf("failed to create user: %w", err)
	}
	return nil
}

// Update updates an existing user
func (r *userRepository) Update(ctx context.Context, user *models.User) error {
	err := r.db.WithContext(ctx).Save(user).Error
	if err != nil {
		return fmt.Errorf("failed to update user: %w", err)
	}
	return nil
}

// Delete soft deletes a user by setting is_active to false
func (r *userRepository) Delete(ctx context.Context, id uuid.UUID) error {
	err := r.db.WithContext(ctx).
		Model(&models.User{}).
		Where("id = ?", id).
		Update("is_active", false).Error
	
	if err != nil {
		return fmt.Errorf("failed to delete user: %w", err)
	}
	return nil
}

// GetUserPermissions retrieves all permissions for a user through roles
func (r *userRepository) GetUserPermissions(ctx context.Context, userID uuid.UUID) ([]models.Permission, error) {
	var permissions []models.Permission
	
	// Use raw SQL for optimal performance with materialized view
	query := `
		SELECT DISTINCT p.id, p.name, p.resource, p.action, p.scope, p.description, 
		       p.is_system, p.created_at, p.updated_at
		FROM permissions p
		JOIN role_permissions rp ON p.id = rp.permission_id
		JOIN user_roles ur ON rp.role_id = ur.role_id
		JOIN roles r ON ur.role_id = r.id
		WHERE ur.user_id = ? AND r.is_active = true AND p.is_system = false
		ORDER BY p.resource, p.action
	`
	
	err := r.readDB.WithContext(ctx).Raw(query, userID).Scan(&permissions).Error
	if err != nil {
		return nil, fmt.Errorf("failed to get user permissions: %w", err)
	}
	
	return permissions, nil
}

// GetUserRoles retrieves all roles for a user
func (r *userRepository) GetUserRoles(ctx context.Context, userID uuid.UUID) ([]models.Role, error) {
	var roles []models.Role
	
	err := r.readDB.WithContext(ctx).
		Joins("JOIN user_roles ur ON roles.id = ur.role_id").
		Where("ur.user_id = ? AND roles.is_active = ?", userID, true).
		Find(&roles).Error
	
	if err != nil {
		return nil, fmt.Errorf("failed to get user roles: %w", err)
	}
	
	return roles, nil
}

// GetActiveUsersByOrganization retrieves active users for an organization with pagination
func (r *userRepository) GetActiveUsersByOrganization(ctx context.Context, orgID uuid.UUID, limit, offset int) ([]models.User, error) {
	var users []models.User
	
	err := r.readDB.WithContext(ctx).
		Where("organization_id = ? AND is_active = ?", orgID, true).
		Order("created_at DESC").
		Limit(limit).
		Offset(offset).
		Find(&users).Error
	
	if err != nil {
		return nil, fmt.Errorf("failed to get active users by organization: %w", err)
	}
	
	return users, nil
}

// GetUsersByIDs retrieves multiple users by their IDs (bulk operation)
func (r *userRepository) GetUsersByIDs(ctx context.Context, ids []uuid.UUID) ([]models.User, error) {
	var users []models.User
	
	err := r.readDB.WithContext(ctx).
		Where("id IN ? AND is_active = ?", ids, true).
		Find(&users).Error
	
	if err != nil {
		return nil, fmt.Errorf("failed to get users by IDs: %w", err)
	}
	
	return users, nil
}

// UpdateLastLogin updates the last login timestamp for a user
func (r *userRepository) UpdateLastLogin(ctx context.Context, userID uuid.UUID, loginTime time.Time) error {
	err := r.db.WithContext(ctx).
		Model(&models.User{}).
		Where("id = ?", userID).
		Updates(map[string]interface{}{
			"last_login_at": loginTime,
			"updated_at":    time.Now(),
		}).Error
	
	if err != nil {
		return fmt.Errorf("failed to update last login: %w", err)
	}
	return nil
}

// IncrementFailedLoginAttempts increments the failed login attempts counter
func (r *userRepository) IncrementFailedLoginAttempts(ctx context.Context, userID uuid.UUID) error {
	err := r.db.WithContext(ctx).
		Model(&models.User{}).
		Where("id = ?", userID).
		UpdateColumn("failed_login_attempts", gorm.Expr("failed_login_attempts + 1")).Error
	
	if err != nil {
		return fmt.Errorf("failed to increment failed login attempts: %w", err)
	}
	return nil
}

// ResetFailedLoginAttempts resets the failed login attempts counter
func (r *userRepository) ResetFailedLoginAttempts(ctx context.Context, userID uuid.UUID) error {
	err := r.db.WithContext(ctx).
		Model(&models.User{}).
		Where("id = ?", userID).
		Updates(map[string]interface{}{
			"failed_login_attempts": 0,
			"locked_until":          nil,
			"updated_at":            time.Now(),
		}).Error
	
	if err != nil {
		return fmt.Errorf("failed to reset failed login attempts: %w", err)
	}
	return nil
}

// LockUser locks a user account until the specified time
func (r *userRepository) LockUser(ctx context.Context, userID uuid.UUID, lockUntil time.Time) error {
	err := r.db.WithContext(ctx).
		Model(&models.User{}).
		Where("id = ?", userID).
		Updates(map[string]interface{}{
			"locked_until": lockUntil,
			"updated_at":   time.Now(),
		}).Error
	
	if err != nil {
		return fmt.Errorf("failed to lock user: %w", err)
	}
	return nil
}