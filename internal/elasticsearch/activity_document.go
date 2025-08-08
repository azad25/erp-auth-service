package elasticsearch

import (
	"time"

	"erp-auth-service/internal/models"

	"github.com/google/uuid"
)

// ActivityDocument represents a user activity document in Elasticsearch
type ActivityDocument struct {
	ID             string                 `json:"id"`
	UserID         string                 `json:"user_id"`
	OrganizationID string                 `json:"organization_id"`
	Action         string                 `json:"action"`
	Resource       string                 `json:"resource"`
	Details        map[string]interface{} `json:"details"`
	IPAddress      string                 `json:"ip_address"`
	UserAgent      string                 `json:"user_agent"`
	Timestamp      time.Time              `json:"timestamp"`
	CreatedAt      time.Time              `json:"created_at"`

	// Enriched user information
	UserEmail     string `json:"user_email,omitempty"`
	UserFirstName string `json:"user_first_name,omitempty"`
	UserLastName  string `json:"user_last_name,omitempty"`
	UserFullName  string `json:"user_full_name,omitempty"`

	// Enriched organization information
	OrgName   string `json:"org_name,omitempty"`
	OrgDomain string `json:"org_domain,omitempty"`

	// Additional metadata for analytics
	SessionID string `json:"session_id,omitempty"`
	RequestID string `json:"request_id,omitempty"`

	// Derived fields for analytics
	ActionCategory  string `json:"action_category"`
	Severity        string `json:"severity"`
	IsSecurityEvent bool   `json:"is_security_event"`

	// Time-based fields for time-series analysis
	Year   int `json:"year"`
	Month  int `json:"month"`
	Day    int `json:"day"`
	Hour   int `json:"hour"`
	Minute int `json:"minute"`
}

// NewActivityDocument creates a new activity document from a UserActivity model
func NewActivityDocument(activity *models.UserActivity) *ActivityDocument {
	doc := &ActivityDocument{
		ID:             activity.ID.String(),
		UserID:         activity.UserID.String(),
		OrganizationID: activity.OrganizationID.String(),
		Action:         activity.Action,
		Resource:       activity.Resource,
		IPAddress:      activity.IPAddress,
		UserAgent:      activity.UserAgent,
		Timestamp:      activity.CreatedAt,
		CreatedAt:      activity.CreatedAt,

		// Time-based fields
		Year:   activity.CreatedAt.Year(),
		Month:  int(activity.CreatedAt.Month()),
		Day:    activity.CreatedAt.Day(),
		Hour:   activity.CreatedAt.Hour(),
		Minute: activity.CreatedAt.Minute(),
	}

	// Parse details JSON
	if activity.Details != "" {
		// Store details as raw JSON string field under details.raw to avoid parsing errors
		doc.Details = map[string]interface{}{
			"raw": activity.Details,
		}
	}

	// Enrich with user information if available
	if activity.User.ID != uuid.Nil {
		doc.UserEmail = activity.User.Email
		doc.UserFirstName = activity.User.FirstName
		doc.UserLastName = activity.User.LastName
		doc.UserFullName = activity.User.FirstName + " " + activity.User.LastName
	}

	// Set derived fields
	doc.ActionCategory = categorizeAction(activity.Action)
	doc.Severity = determineSeverity(activity.Action)
	doc.IsSecurityEvent = isSecurityEvent(activity.Action)

	return doc
}

// categorizeAction categorizes the action for analytics
func categorizeAction(action string) string {
	switch {
	case action == models.ActionLogin || action == models.ActionLogout:
		return "authentication"
	case action == models.ActionLoginFailed:
		return "security"
	case action == models.ActionUserCreated || action == models.ActionUserUpdated || action == models.ActionUserDeleted:
		return "user_management"
	case action == models.ActionPasswordChanged:
		return "security"
	case action == models.ActionProfileUpdated:
		return "profile"
	default:
		return "general"
	}
}

// determineSeverity determines the severity level of the action
func determineSeverity(action string) string {
	switch action {
	case models.ActionLoginFailed:
		return "high"
	case models.ActionUserDeleted, models.ActionPasswordChanged:
		return "medium"
	case models.ActionLogin, models.ActionLogout:
		return "low"
	case models.ActionUserCreated, models.ActionUserUpdated:
		return "medium"
	case models.ActionProfileUpdated:
		return "low"
	default:
		return "low"
	}
}

// isSecurityEvent determines if the action is a security-related event
func isSecurityEvent(action string) bool {
	securityActions := []string{
		models.ActionLoginFailed,
		models.ActionPasswordChanged,
		models.ActionUserDeleted,
	}

	for _, secAction := range securityActions {
		if action == secAction {
			return true
		}
	}
	return false
}

// GetActivityIndexMapping returns the Elasticsearch mapping for activity documents
func GetActivityIndexMapping() map[string]interface{} {
	return map[string]interface{}{
		"mappings": map[string]interface{}{
			"properties": map[string]interface{}{
				"id": map[string]interface{}{
					"type": "keyword",
				},
				"user_id": map[string]interface{}{
					"type": "keyword",
				},
				"organization_id": map[string]interface{}{
					"type": "keyword",
				},
				"action": map[string]interface{}{
					"type": "keyword",
				},
				"resource": map[string]interface{}{
					"type": "keyword",
				},
				"details": map[string]interface{}{
					"type":    "object",
					"enabled": true,
				},
				"ip_address": map[string]interface{}{
					"type": "ip",
				},
				"user_agent": map[string]interface{}{
					"type": "text",
					"fields": map[string]interface{}{
						"keyword": map[string]interface{}{
							"type": "keyword",
						},
					},
				},
				"timestamp": map[string]interface{}{
					"type": "date",
				},
				"created_at": map[string]interface{}{
					"type": "date",
				},
				"user_email": map[string]interface{}{
					"type": "keyword",
				},
				"user_first_name": map[string]interface{}{
					"type": "text",
				},
				"user_last_name": map[string]interface{}{
					"type": "text",
				},
				"user_full_name": map[string]interface{}{
					"type": "text",
				},
				"org_name": map[string]interface{}{
					"type": "text",
				},
				"org_domain": map[string]interface{}{
					"type": "keyword",
				},
				"session_id": map[string]interface{}{
					"type": "keyword",
				},
				"request_id": map[string]interface{}{
					"type": "keyword",
				},
				"action_category": map[string]interface{}{
					"type": "keyword",
				},
				"severity": map[string]interface{}{
					"type": "keyword",
				},
				"is_security_event": map[string]interface{}{
					"type": "boolean",
				},
				"year": map[string]interface{}{
					"type": "integer",
				},
				"month": map[string]interface{}{
					"type": "integer",
				},
				"day": map[string]interface{}{
					"type": "integer",
				},
				"hour": map[string]interface{}{
					"type": "integer",
				},
				"minute": map[string]interface{}{
					"type": "integer",
				},
			},
		},
		"settings": map[string]interface{}{
			"number_of_shards":   1,
			"number_of_replicas": 0,
			"refresh_interval":   "1s",
		},
	}
}
