package repositories

import (
	"context"
	"encoding/json"
	"time"

	"erp-auth-service/internal/models"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// UserActivityRepository handles user activity operations
type UserActivityRepository struct {
	db *gorm.DB
}

// NewUserActivityRepository creates a new user activity repository
func NewUserActivityRepository(db *gorm.DB) *UserActivityRepository {
	return &UserActivityRepository{
		db: db,
	}
}

// CreateActivity creates a new user activity log entry
func (r *UserActivityRepository) CreateActivity(ctx context.Context, activity *models.UserActivity) error {
	return r.db.WithContext(ctx).Create(activity).Error
}

// GetUserActivities retrieves user activities with pagination
// If organizationID is nil, it retrieves activities from all organizations (for super admin)
func (r *UserActivityRepository) GetUserActivities(ctx context.Context, userID *uuid.UUID, organizationID *uuid.UUID, limit, offset int) ([]*models.UserActivity, int64, error) {
	var activities []*models.UserActivity
	var total int64

	query := r.db.WithContext(ctx).Model(&models.UserActivity{}).
		Preload("User")

	// Filter by organization ID if provided (nil means all organizations for super admin)
	if organizationID != nil {
		query = query.Where("organization_id = ?", *organizationID)
	}

	// Filter by user ID if provided
	if userID != nil {
		query = query.Where("user_id = ?", *userID)
	}

	// Get total count
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	// Get activities with pagination
	if err := query.Order("created_at DESC").
		Limit(limit).
		Offset(offset).
		Find(&activities).Error; err != nil {
		return nil, 0, err
	}

	return activities, total, nil
}

// GetActivityByID retrieves a specific activity by ID
func (r *UserActivityRepository) GetActivityByID(ctx context.Context, id uuid.UUID) (*models.UserActivity, error) {
	var activity models.UserActivity
	if err := r.db.WithContext(ctx).
		Preload("User").
		Preload("Organization").
		First(&activity, "id = ?", id).Error; err != nil {
		return nil, err
	}
	return &activity, nil
}

// DeleteOldActivities deletes activities older than the specified duration
func (r *UserActivityRepository) DeleteOldActivities(ctx context.Context, olderThan time.Duration) error {
	cutoffTime := time.Now().Add(-olderThan)
	return r.db.WithContext(ctx).
		Where("created_at < ?", cutoffTime).
		Delete(&models.UserActivity{}).Error
}

// GetActivityStats retrieves activity statistics for an organization
func (r *UserActivityRepository) GetActivityStats(ctx context.Context, organizationID uuid.UUID, since time.Time) (map[string]int64, error) {
	var results []struct {
		Action string
		Count  int64
	}

	if err := r.db.WithContext(ctx).
		Model(&models.UserActivity{}).
		Select("action, COUNT(*) as count").
		Where("organization_id = ? AND created_at >= ?", organizationID, since).
		Group("action").
		Find(&results).Error; err != nil {
		return nil, err
	}

	stats := make(map[string]int64)
	for _, result := range results {
		stats[result.Action] = result.Count
	}

	return stats, nil
}

// LogActivity is a helper method to create and log an activity
func (r *UserActivityRepository) LogActivity(ctx context.Context, userID, organizationID uuid.UUID, action, resource string, details map[string]interface{}, ipAddress, userAgent string) error {
	var detailsJSON string
	if details != nil {
		if jsonBytes, err := json.Marshal(details); err == nil {
			detailsJSON = string(jsonBytes)
		}
	}

	activity := &models.UserActivity{
		UserID:         userID,
		OrganizationID: organizationID,
		Action:         action,
		Resource:       resource,
		Details:        detailsJSON,
		IPAddress:      ipAddress,
		UserAgent:      userAgent,
	}

	return r.CreateActivity(ctx, activity)
}

// GetRecentLoginCount gets the count of recent successful logins
func (r *UserActivityRepository) GetRecentLoginCount(ctx context.Context, organizationID uuid.UUID, since time.Time) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).
		Model(&models.UserActivity{}).
		Where("organization_id = ? AND action = ? AND created_at >= ?", organizationID, models.ActionLogin, since).
		Count(&count).Error
	return count, err
}

// GetFailedLoginCount gets the count of failed login attempts
func (r *UserActivityRepository) GetFailedLoginCount(ctx context.Context, organizationID uuid.UUID, since time.Time) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).
		Model(&models.UserActivity{}).
		Where("organization_id = ? AND action = ? AND created_at >= ?", organizationID, models.ActionLoginFailed, since).
		Count(&count).Error
	return count, err
}

// GetUserActivityCount gets the total activity count for a user
func (r *UserActivityRepository) GetUserActivityCount(ctx context.Context, userID uuid.UUID, since time.Time) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).
		Model(&models.UserActivity{}).
		Where("user_id = ? AND created_at >= ?", userID, since).
		Count(&count).Error
	return count, err
}