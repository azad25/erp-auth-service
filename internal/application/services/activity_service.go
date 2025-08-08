package services

import (
	"context"
	"time"

	"erp-auth-service/internal/infrastructure/repositories"
	"erp-auth-service/internal/models"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

// ActivityService handles user activity logging and retrieval
type ActivityService struct {
	activityRepo *repositories.UserActivityRepository
	logger       *zap.Logger
}

// NewActivityService creates a new activity service
func NewActivityService(activityRepo *repositories.UserActivityRepository, logger *zap.Logger) *ActivityService {
	return &ActivityService{
		activityRepo: activityRepo,
		logger:       logger,
	}
}

// LogActivity logs a user activity
func (s *ActivityService) LogActivity(ctx context.Context, userID, organizationID uuid.UUID, action, resource string, details map[string]interface{}, ipAddress, userAgent string) error {
	s.logger.Debug("Logging user activity",
		zap.String("user_id", userID.String()),
		zap.String("organization_id", organizationID.String()),
		zap.String("action", action),
		zap.String("resource", resource),
		zap.String("ip_address", ipAddress))

	return s.activityRepo.LogActivity(ctx, userID, organizationID, action, resource, details, ipAddress, userAgent)
}

// GetUserActivities retrieves user activities with pagination
// If organizationID is nil, it retrieves activities from all organizations (for super admin)
func (s *ActivityService) GetUserActivities(ctx context.Context, userID *uuid.UUID, organizationID *uuid.UUID, limit, offset int) ([]*models.UserActivity, int64, error) {
	if organizationID != nil {
		s.logger.Debug("Getting user activities",
			zap.String("organization_id", organizationID.String()),
			zap.Int("limit", limit),
			zap.Int("offset", offset))
	} else {
		s.logger.Debug("Getting user activities for all organizations (super admin)",
			zap.Int("limit", limit),
			zap.Int("offset", offset))
	}

	return s.activityRepo.GetUserActivities(ctx, userID, organizationID, limit, offset)
}

// GetActivityStats retrieves activity statistics
func (s *ActivityService) GetActivityStats(ctx context.Context, organizationID uuid.UUID, since time.Time) (map[string]int64, error) {
	s.logger.Debug("Getting activity stats",
		zap.String("organization_id", organizationID.String()),
		zap.Time("since", since))

	return s.activityRepo.GetActivityStats(ctx, organizationID, since)
}

// LogLogin logs a successful login activity
func (s *ActivityService) LogLogin(ctx context.Context, userID, organizationID uuid.UUID, ipAddress, userAgent string) error {
	details := map[string]interface{}{
		"timestamp": time.Now().UTC(),
		"success":   true,
	}
	return s.LogActivity(ctx, userID, organizationID, models.ActionLogin, models.ResourceAuth, details, ipAddress, userAgent)
}

// LogLoginFailed logs a failed login attempt
func (s *ActivityService) LogLoginFailed(ctx context.Context, email, ipAddress, userAgent string, organizationID uuid.UUID) error {
	// For failed logins, we might not have a user ID, so we'll use a nil UUID
	// and store the email in details
	details := map[string]interface{}{
		"email":     email,
		"timestamp": time.Now().UTC(),
		"success":   false,
	}
	return s.LogActivity(ctx, uuid.Nil, organizationID, models.ActionLoginFailed, models.ResourceAuth, details, ipAddress, userAgent)
}

// LogLogout logs a logout activity
func (s *ActivityService) LogLogout(ctx context.Context, userID, organizationID uuid.UUID, ipAddress, userAgent string) error {
	details := map[string]interface{}{
		"timestamp": time.Now().UTC(),
	}
	return s.LogActivity(ctx, userID, organizationID, models.ActionLogout, models.ResourceAuth, details, ipAddress, userAgent)
}

// LogUserCreated logs user creation activity
func (s *ActivityService) LogUserCreated(ctx context.Context, creatorID, newUserID, organizationID uuid.UUID, ipAddress, userAgent string) error {
	details := map[string]interface{}{
		"new_user_id": newUserID.String(),
		"timestamp":   time.Now().UTC(),
	}
	return s.LogActivity(ctx, creatorID, organizationID, models.ActionUserCreated, models.ResourceUser, details, ipAddress, userAgent)
}

// LogUserUpdated logs user update activity
func (s *ActivityService) LogUserUpdated(ctx context.Context, updaterID, targetUserID, organizationID uuid.UUID, changes map[string]interface{}, ipAddress, userAgent string) error {
	details := map[string]interface{}{
		"target_user_id": targetUserID.String(),
		"changes":        changes,
		"timestamp":      time.Now().UTC(),
	}
	return s.LogActivity(ctx, updaterID, organizationID, models.ActionUserUpdated, models.ResourceUser, details, ipAddress, userAgent)
}

// LogUserDeleted logs user deletion activity
func (s *ActivityService) LogUserDeleted(ctx context.Context, deleterID, deletedUserID, organizationID uuid.UUID, ipAddress, userAgent string) error {
	details := map[string]interface{}{
		"deleted_user_id": deletedUserID.String(),
		"timestamp":       time.Now().UTC(),
	}
	return s.LogActivity(ctx, deleterID, organizationID, models.ActionUserDeleted, models.ResourceUser, details, ipAddress, userAgent)
}

// LogPasswordChanged logs password change activity
func (s *ActivityService) LogPasswordChanged(ctx context.Context, userID, organizationID uuid.UUID, ipAddress, userAgent string) error {
	details := map[string]interface{}{
		"timestamp": time.Now().UTC(),
	}
	return s.LogActivity(ctx, userID, organizationID, models.ActionPasswordChanged, models.ResourceProfile, details, ipAddress, userAgent)
}

// LogProfileUpdated logs profile update activity
func (s *ActivityService) LogProfileUpdated(ctx context.Context, userID, organizationID uuid.UUID, changes map[string]interface{}, ipAddress, userAgent string) error {
	details := map[string]interface{}{
		"changes":   changes,
		"timestamp": time.Now().UTC(),
	}
	return s.LogActivity(ctx, userID, organizationID, models.ActionProfileUpdated, models.ResourceProfile, details, ipAddress, userAgent)
}

// CleanupOldActivities removes old activity logs
func (s *ActivityService) CleanupOldActivities(ctx context.Context, olderThan time.Duration) error {
	s.logger.Info("Cleaning up old activities", zap.Duration("older_than", olderThan))
	return s.activityRepo.DeleteOldActivities(ctx, olderThan)
}

// GetSecurityStats retrieves security-related statistics
func (s *ActivityService) GetSecurityStats(ctx context.Context, organizationID uuid.UUID) (map[string]int64, error) {
	now := time.Now()
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	
	stats := make(map[string]int64)
	
	// Get failed logins today
	failedLogins, err := s.activityRepo.GetFailedLoginCount(ctx, organizationID, today)
	if err != nil {
		s.logger.Error("Failed to get failed login count", zap.Error(err))
		failedLogins = 0
	}
	stats["failed_logins_today"] = failedLogins
	
	// Get recent logins (last 24 hours)
	recentLogins, err := s.activityRepo.GetRecentLoginCount(ctx, organizationID, now.Add(-24*time.Hour))
	if err != nil {
		s.logger.Error("Failed to get recent login count", zap.Error(err))
		recentLogins = 0
	}
	stats["recent_logins"] = recentLogins
	
	return stats, nil
}