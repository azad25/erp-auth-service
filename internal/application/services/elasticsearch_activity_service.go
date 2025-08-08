package services

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"erp-auth-service/internal/elasticsearch"
	"erp-auth-service/internal/models"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

// ElasticsearchActivityService handles user activity logging and retrieval using Elasticsearch
type ElasticsearchActivityService struct {
	esClient    *elasticsearch.Client
	logger      *zap.Logger
	indexPrefix string
	redis       *redis.Client
}

// NewElasticsearchActivityService creates a new Elasticsearch-based activity service
func NewElasticsearchActivityService(esClient *elasticsearch.Client, logger *zap.Logger) *ElasticsearchActivityService {
	service := &ElasticsearchActivityService{
		esClient:    esClient,
		logger:      logger,
		indexPrefix: "user-activities",
	}

	// Initialize indices
	if err := service.initializeIndices(context.Background()); err != nil {
		logger.Error("Failed to initialize Elasticsearch indices", zap.Error(err))
	}

	return service
}

// WithRedis enables Redis publishing for real-time alerts
func (s *ElasticsearchActivityService) WithRedis(client *redis.Client) *ElasticsearchActivityService {
	s.redis = client
	return s
}

// initializeIndices creates the necessary Elasticsearch indices
func (s *ElasticsearchActivityService) initializeIndices(ctx context.Context) error {
	// Create the main activity index
	indexName := s.getCurrentIndexName()
	mapping := elasticsearch.GetActivityIndexMapping()

	if err := s.esClient.CreateIndex(ctx, indexName, mapping); err != nil {
		return fmt.Errorf("failed to create activity index: %w", err)
	}

	s.logger.Info("Elasticsearch activity indices initialized", zap.String("index", indexName))
	return nil
}

// getCurrentIndexName returns the current index name (time-based for better performance)
func (s *ElasticsearchActivityService) getCurrentIndexName() string {
	now := time.Now()
	return fmt.Sprintf("%s-%d-%02d", s.indexPrefix, now.Year(), now.Month())
}

// LogActivity logs a user activity to Elasticsearch
func (s *ElasticsearchActivityService) LogActivity(ctx context.Context, userID, organizationID uuid.UUID, action, resource string, details map[string]interface{}, ipAddress, userAgent string) error {
	s.logger.Debug("Logging user activity to Elasticsearch",
		zap.String("user_id", userID.String()),
		zap.String("organization_id", organizationID.String()),
		zap.String("action", action),
		zap.String("resource", resource),
		zap.String("ip_address", ipAddress))

	// Create activity model
	activity := &models.UserActivity{
		ID:             uuid.New(),
		UserID:         userID,
		OrganizationID: organizationID,
		Action:         action,
		Resource:       resource,
		IPAddress:      ipAddress,
		UserAgent:      userAgent,
		CreatedAt:      time.Now(),
	}

	// Convert details to JSON string
	if details != nil {
		detailsJSON, err := json.Marshal(details)
		if err != nil {
			s.logger.Error("Failed to marshal activity details", zap.Error(err))
		} else {
			activity.Details = string(detailsJSON)
		}
	}

	// Create Elasticsearch document
	doc := elasticsearch.NewActivityDocument(activity)

	// Add additional context if available
	if details != nil {
		doc.Details = details
	}

	// Index the document
	indexName := s.getCurrentIndexName()
	if err := s.esClient.IndexDocument(ctx, indexName, doc.ID, doc); err != nil {
		s.logger.Error("Failed to index activity document", zap.Error(err))
		return fmt.Errorf("failed to log activity: %w", err)
	}

	// If this is a security event, also check for patterns and create alerts
	if doc.IsSecurityEvent {
		go s.checkSecurityPatterns(context.Background(), doc)
	}

	return nil
}

// publishWebSocketEvent publishes a security event to Redis so API GW can push via WebSockets
func (s *ElasticsearchActivityService) publishWebSocketEvent(ctx context.Context, eventType string, data map[string]interface{}) {
	if s.redis == nil {
		return
	}
	payload := map[string]interface{}{
		"type":       "event",
		"channel":    fmt.Sprintf("security:%s", eventType),
		"data":       data,
		"timestamp":  time.Now().Format(time.RFC3339),
		"message_id": uuid.New().String(),
	}
	bytes, _ := json.Marshal(payload)
	channel := fmt.Sprintf("events:%s", fmt.Sprintf("security:%s", eventType))
	if err := s.redis.Publish(ctx, channel, string(bytes)).Err(); err != nil {
		s.logger.Warn("Failed to publish security event to Redis", zap.Error(err), zap.String("channel", channel))
	}
}

// GetUserActivities retrieves user activities from Elasticsearch with pagination
func (s *ElasticsearchActivityService) GetUserActivities(ctx context.Context, userID *uuid.UUID, organizationID *uuid.UUID, limit, offset int) ([]*models.UserActivity, int64, error) {
	query := s.buildActivityQuery(userID, organizationID, limit, offset)

	// Search across all activity indices (use wildcard pattern)
	indexPattern := s.indexPrefix + "-*"

	searchResp, err := s.esClient.SearchDocuments(ctx, indexPattern, query)
	if err != nil {
		s.logger.Error("Failed to search activities", zap.Error(err))
		return nil, 0, fmt.Errorf("failed to search activities: %w", err)
	}

	// Convert Elasticsearch response to models
	activities := make([]*models.UserActivity, 0, len(searchResp.Hits.Hits))
	for _, hit := range searchResp.Hits.Hits {
		activity, err := s.convertHitToActivity(hit.Source)
		if err != nil {
			s.logger.Error("Failed to convert hit to activity", zap.Error(err))
			continue
		}
		activities = append(activities, activity)
	}

	total := int64(searchResp.Hits.Total.Value)
	return activities, total, nil
}

// buildActivityQuery builds an Elasticsearch query for activities
func (s *ElasticsearchActivityService) buildActivityQuery(userID *uuid.UUID, organizationID *uuid.UUID, limit, offset int) map[string]interface{} {
	query := map[string]interface{}{
		"query": map[string]interface{}{
			"bool": map[string]interface{}{
				"must": []map[string]interface{}{},
			},
		},
		"sort": []map[string]interface{}{
			{
				"timestamp": map[string]interface{}{
					"order": "desc",
				},
			},
		},
		"size": limit,
		"from": offset,
	}

	mustClauses := query["query"].(map[string]interface{})["bool"].(map[string]interface{})["must"].([]map[string]interface{})

	// Filter by organization ID if provided (nil means all organizations for super admin)
	if organizationID != nil {
		mustClauses = append(mustClauses, map[string]interface{}{
			"term": map[string]interface{}{
				"organization_id": organizationID.String(),
			},
		})
	}

	// Filter by user ID if provided
	if userID != nil {
		mustClauses = append(mustClauses, map[string]interface{}{
			"term": map[string]interface{}{
				"user_id": userID.String(),
			},
		})
	}

	query["query"].(map[string]interface{})["bool"].(map[string]interface{})["must"] = mustClauses
	return query
}

// convertHitToActivity converts an Elasticsearch hit to a UserActivity model
func (s *ElasticsearchActivityService) convertHitToActivity(source map[string]interface{}) (*models.UserActivity, error) {
	activity := &models.UserActivity{}

	// Parse ID
	if id, ok := source["id"].(string); ok {
		if parsedID, err := uuid.Parse(id); err == nil {
			activity.ID = parsedID
		}
	}

	// Parse UserID
	if userID, ok := source["user_id"].(string); ok {
		if parsedUserID, err := uuid.Parse(userID); err == nil {
			activity.UserID = parsedUserID
		}
	}

	// Parse OrganizationID
	if orgID, ok := source["organization_id"].(string); ok {
		if parsedOrgID, err := uuid.Parse(orgID); err == nil {
			activity.OrganizationID = parsedOrgID
		}
	}

	// Basic fields
	if action, ok := source["action"].(string); ok {
		activity.Action = action
	}
	if resource, ok := source["resource"].(string); ok {
		activity.Resource = resource
	}
	if ipAddress, ok := source["ip_address"].(string); ok {
		activity.IPAddress = ipAddress
	}
	if userAgent, ok := source["user_agent"].(string); ok {
		activity.UserAgent = userAgent
	}

	// Parse timestamp
	if timestamp, ok := source["timestamp"].(string); ok {
		if parsedTime, err := time.Parse(time.RFC3339, timestamp); err == nil {
			activity.CreatedAt = parsedTime
		}
	}

	// Parse details
	if details, ok := source["details"]; ok {
		if detailsJSON, err := json.Marshal(details); err == nil {
			activity.Details = string(detailsJSON)
		}
	}

	// Create user info from enriched fields
	if userEmail, ok := source["user_email"].(string); ok {
		activity.User = models.User{
			Email: userEmail,
		}
		if firstName, ok := source["user_first_name"].(string); ok {
			activity.User.FirstName = firstName
		}
		if lastName, ok := source["user_last_name"].(string); ok {
			activity.User.LastName = lastName
		}
		// Set a dummy ID to indicate user info is available
		activity.User.ID = uuid.New()
	}

	return activity, nil
}

// GetSecurityEvents retrieves recent security events
func (s *ElasticsearchActivityService) GetSecurityEvents(ctx context.Context, organizationID *uuid.UUID, limit int) ([]*SecurityEvent, error) {
	query := map[string]interface{}{
		"query": map[string]interface{}{
			"bool": map[string]interface{}{
				"must": []map[string]interface{}{
					{
						"term": map[string]interface{}{
							"is_security_event": true,
						},
					},
					{
						"range": map[string]interface{}{
							"timestamp": map[string]interface{}{
								"gte": "now-24h",
							},
						},
					},
				},
			},
		},
		"sort": []map[string]interface{}{
			{
				"timestamp": map[string]interface{}{
					"order": "desc",
				},
			},
		},
		"size": limit,
	}

	// Filter by organization if provided
	mustClauses := query["query"].(map[string]interface{})["bool"].(map[string]interface{})["must"].([]map[string]interface{})
	if organizationID != nil {
		mustClauses = append(mustClauses, map[string]interface{}{
			"term": map[string]interface{}{
				"organization_id": organizationID.String(),
			},
		})
		query["query"].(map[string]interface{})["bool"].(map[string]interface{})["must"] = mustClauses
	}

	indexPattern := s.indexPrefix + "-*"
	searchResp, err := s.esClient.SearchDocuments(ctx, indexPattern, query)
	if err != nil {
		return nil, fmt.Errorf("failed to search security events: %w", err)
	}

	events := make([]*SecurityEvent, 0, len(searchResp.Hits.Hits))
	for _, hit := range searchResp.Hits.Hits {
		event := s.convertHitToSecurityEvent(hit.Source)
		if event != nil {
			events = append(events, event)
		}
	}

	return events, nil
}

// SecurityEvent represents a security event
type SecurityEvent struct {
	ID        string                 `json:"id"`
	Type      string                 `json:"type"`
	Severity  string                 `json:"severity"`
	Message   string                 `json:"message"`
	IPAddress string                 `json:"ip_address"`
	UserAgent string                 `json:"user_agent"`
	UserEmail string                 `json:"user_email"`
	Timestamp time.Time              `json:"timestamp"`
	Details   map[string]interface{} `json:"details"`
	Count     int                    `json:"count"`
}

// convertHitToSecurityEvent converts an Elasticsearch hit to a SecurityEvent
func (s *ElasticsearchActivityService) convertHitToSecurityEvent(source map[string]interface{}) *SecurityEvent {
	event := &SecurityEvent{}

	if id, ok := source["id"].(string); ok {
		event.ID = id
	}
	if action, ok := source["action"].(string); ok {
		event.Type = action
	}
	if severity, ok := source["severity"].(string); ok {
		event.Severity = severity
	}
	if ipAddress, ok := source["ip_address"].(string); ok {
		event.IPAddress = ipAddress
	}
	if userAgent, ok := source["user_agent"].(string); ok {
		event.UserAgent = userAgent
	}
	if userEmail, ok := source["user_email"].(string); ok {
		event.UserEmail = userEmail
	}
	if timestamp, ok := source["timestamp"].(string); ok {
		if parsedTime, err := time.Parse(time.RFC3339, timestamp); err == nil {
			event.Timestamp = parsedTime
		}
	}
	if details, ok := source["details"]; ok {
		if detailsMap, ok := details.(map[string]interface{}); ok {
			event.Details = detailsMap
		}
	}

	// Generate message based on event type
	event.Message = s.generateSecurityEventMessage(event)
	event.Count = 1 // Default count, can be aggregated later

	return event
}

// generateSecurityEventMessage generates a human-readable message for security events
func (s *ElasticsearchActivityService) generateSecurityEventMessage(event *SecurityEvent) string {
	switch event.Type {
	case models.ActionLoginFailed:
		if event.UserEmail != "" {
			return fmt.Sprintf("Failed login attempt for user %s from IP %s", event.UserEmail, event.IPAddress)
		}
		return fmt.Sprintf("Failed login attempt from IP %s", event.IPAddress)
	case models.ActionPasswordChanged:
		return fmt.Sprintf("Password changed for user %s from IP %s", event.UserEmail, event.IPAddress)
	case models.ActionUserDeleted:
		return fmt.Sprintf("User account deleted: %s from IP %s", event.UserEmail, event.IPAddress)
	default:
		return fmt.Sprintf("Security event: %s from IP %s", event.Type, event.IPAddress)
	}
}

// checkSecurityPatterns checks for security patterns and creates alerts
func (s *ElasticsearchActivityService) checkSecurityPatterns(ctx context.Context, doc *elasticsearch.ActivityDocument) {
	// Check for multiple failed login attempts from same IP
	if doc.Action == models.ActionLoginFailed {
		s.checkFailedLoginPattern(ctx, doc)
	}
}

// checkFailedLoginPattern checks for multiple failed login attempts
func (s *ElasticsearchActivityService) checkFailedLoginPattern(ctx context.Context, doc *elasticsearch.ActivityDocument) {
	// Query for failed logins from the same IP in the last hour
	query := map[string]interface{}{
		"query": map[string]interface{}{
			"bool": map[string]interface{}{
				"must": []map[string]interface{}{
					{
						"term": map[string]interface{}{
							"action": models.ActionLoginFailed,
						},
					},
					{
						"term": map[string]interface{}{
							"ip_address": doc.IPAddress,
						},
					},
					{
						"range": map[string]interface{}{
							"timestamp": map[string]interface{}{
								"gte": "now-1h",
							},
						},
					},
				},
			},
		},
		"size": 0,
	}

	// Add organization filter if not super admin context
	if doc.OrganizationID != "" {
		mustClauses := query["query"].(map[string]interface{})["bool"].(map[string]interface{})["must"].([]map[string]interface{})
		mustClauses = append(mustClauses, map[string]interface{}{
			"term": map[string]interface{}{
				"organization_id": doc.OrganizationID,
			},
		})
		query["query"].(map[string]interface{})["bool"].(map[string]interface{})["must"] = mustClauses
	}

	indexPattern := s.indexPrefix + "-*"
	searchResp, err := s.esClient.SearchDocuments(ctx, indexPattern, query)
	if err != nil {
		s.logger.Error("Failed to check failed login pattern", zap.Error(err))
		return
	}

	failedAttempts := searchResp.Hits.Total.Value
	if failedAttempts >= 5 { // Threshold for security alert
		s.logger.Warn("Multiple failed login attempts detected",
			zap.String("ip_address", doc.IPAddress),
			zap.Int("attempts", failedAttempts),
			zap.String("organization_id", doc.OrganizationID))

		// Publish to Redis for immediate WebSocket push
		data := map[string]interface{}{
			"type":       "failed_login_threshold",
			"attempts":   failedAttempts,
			"ip_address": doc.IPAddress,
			"org_id":     doc.OrganizationID,
			"timestamp":  time.Now().Format(time.RFC3339),
		}
		s.publishWebSocketEvent(ctx, "failed_login", data)
	}
}

// GetSecurityStats retrieves security statistics from Elasticsearch
func (s *ElasticsearchActivityService) GetSecurityStats(ctx context.Context, organizationID uuid.UUID) (map[string]int64, error) {
	now := time.Now()
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())

	stats := make(map[string]int64)

	// Query for failed logins today
	failedLoginsQuery := s.buildSecurityStatsQuery(organizationID, models.ActionLoginFailed, today)
	failedLoginsResp, err := s.esClient.SearchDocuments(ctx, s.indexPrefix+"-*", failedLoginsQuery)
	if err != nil {
		s.logger.Error("Failed to get failed login stats", zap.Error(err))
		stats["failed_logins_today"] = 0
	} else {
		stats["failed_logins_today"] = int64(failedLoginsResp.Hits.Total.Value)
	}

	// Query for recent successful logins (last 24 hours)
	recentLoginsQuery := s.buildSecurityStatsQuery(organizationID, models.ActionLogin, now.Add(-24*time.Hour))
	recentLoginsResp, err := s.esClient.SearchDocuments(ctx, s.indexPrefix+"-*", recentLoginsQuery)
	if err != nil {
		s.logger.Error("Failed to get recent login stats", zap.Error(err))
		stats["recent_logins"] = 0
	} else {
		stats["recent_logins"] = int64(recentLoginsResp.Hits.Total.Value)
	}

	// Query for security events today
	securityEventsQuery := s.buildSecurityEventsStatsQuery(organizationID, today)
	securityEventsResp, err := s.esClient.SearchDocuments(ctx, s.indexPrefix+"-*", securityEventsQuery)
	if err != nil {
		s.logger.Error("Failed to get security events stats", zap.Error(err))
		stats["security_alerts"] = 0
	} else {
		stats["security_alerts"] = int64(securityEventsResp.Hits.Total.Value)
	}

	return stats, nil
}

// buildSecurityStatsQuery builds a query for security statistics
func (s *ElasticsearchActivityService) buildSecurityStatsQuery(organizationID uuid.UUID, action string, since time.Time) map[string]interface{} {
	return map[string]interface{}{
		"query": map[string]interface{}{
			"bool": map[string]interface{}{
				"must": []map[string]interface{}{
					{
						"term": map[string]interface{}{
							"organization_id": organizationID.String(),
						},
					},
					{
						"term": map[string]interface{}{
							"action": action,
						},
					},
					{
						"range": map[string]interface{}{
							"timestamp": map[string]interface{}{
								"gte": since.Format(time.RFC3339),
							},
						},
					},
				},
			},
		},
		"size": 0,
	}
}

// buildSecurityEventsStatsQuery builds a query for security events statistics
func (s *ElasticsearchActivityService) buildSecurityEventsStatsQuery(organizationID uuid.UUID, since time.Time) map[string]interface{} {
	return map[string]interface{}{
		"query": map[string]interface{}{
			"bool": map[string]interface{}{
				"must": []map[string]interface{}{
					{
						"term": map[string]interface{}{
							"organization_id": organizationID.String(),
						},
					},
					{
						"term": map[string]interface{}{
							"is_security_event": true,
						},
					},
					{
						"range": map[string]interface{}{
							"timestamp": map[string]interface{}{
								"gte": since.Format(time.RFC3339),
							},
						},
					},
				},
			},
		},
		"size": 0,
	}
}

// CleanupOldActivities removes old activity logs (Elasticsearch will handle this via ILM policies)
func (s *ElasticsearchActivityService) CleanupOldActivities(ctx context.Context, olderThan time.Duration) error {
	s.logger.Info("Cleanup for Elasticsearch is handled via Index Lifecycle Management policies",
		zap.Duration("older_than", olderThan))
	return nil
}

// Helper methods for logging specific activities
func (s *ElasticsearchActivityService) LogLogin(ctx context.Context, userID, organizationID uuid.UUID, ipAddress, userAgent string) error {
	details := map[string]interface{}{
		"timestamp": time.Now().UTC(),
		"success":   true,
	}
	return s.LogActivity(ctx, userID, organizationID, models.ActionLogin, models.ResourceAuth, details, ipAddress, userAgent)
}

func (s *ElasticsearchActivityService) LogLoginFailed(ctx context.Context, email, ipAddress, userAgent string, organizationID uuid.UUID) error {
	details := map[string]interface{}{
		"email":     email,
		"timestamp": time.Now().UTC(),
		"success":   false,
	}
	return s.LogActivity(ctx, uuid.Nil, organizationID, models.ActionLoginFailed, models.ResourceAuth, details, ipAddress, userAgent)
}

func (s *ElasticsearchActivityService) LogLogout(ctx context.Context, userID, organizationID uuid.UUID, ipAddress, userAgent string) error {
	details := map[string]interface{}{
		"timestamp": time.Now().UTC(),
	}
	return s.LogActivity(ctx, userID, organizationID, models.ActionLogout, models.ResourceAuth, details, ipAddress, userAgent)
}
