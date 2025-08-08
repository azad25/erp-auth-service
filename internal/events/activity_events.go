package events

import (
	"time"

	"github.com/google/uuid"
)

// Activity event types
const (
	EventTypeUserActivity = "user.activity"
)

// UserActivityEvent represents a user activity event for Kafka streaming
type UserActivityEvent struct {
	BaseEvent
	Data UserActivityData `json:"data"`
}

// UserActivityData contains the activity details
type UserActivityData struct {
	ActivityID     uuid.UUID              `json:"activity_id"`
	UserID         uuid.UUID              `json:"user_id"`
	OrganizationID uuid.UUID              `json:"organization_id"`
	Action         string                 `json:"action"`
	Resource       string                 `json:"resource"`
	Details        map[string]interface{} `json:"details"`
	IPAddress      string                 `json:"ip_address"`
	UserAgent      string                 `json:"user_agent"`
	Timestamp      time.Time              `json:"timestamp"`
	
	// Additional metadata for analytics
	SessionID      string                 `json:"session_id,omitempty"`
	RequestID      string                 `json:"request_id,omitempty"`
	
	// User context for enriched analytics
	UserEmail      string                 `json:"user_email,omitempty"`
	UserName       string                 `json:"user_name,omitempty"`
	OrgName        string                 `json:"org_name,omitempty"`
	OrgDomain      string                 `json:"org_domain,omitempty"`
}

// NewUserActivityEvent creates a new user activity event
func NewUserActivityEvent(
	activityID, userID, organizationID uuid.UUID,
	action, resource string,
	details map[string]interface{},
	ipAddress, userAgent string,
) *UserActivityEvent {
	return &UserActivityEvent{
		BaseEvent: BaseEvent{
			ID:             uuid.New(),
			Type:           EventTypeUserActivity,
			Source:         "auth-service",
			Timestamp:      time.Now().UTC(),
			UserID:         &userID,
			OrganizationID: &organizationID,
			IPAddress:      ipAddress,
			UserAgent:      userAgent,
		},
		Data: UserActivityData{
			ActivityID:     activityID,
			UserID:         userID,
			OrganizationID: organizationID,
			Action:         action,
			Resource:       resource,
			Details:        details,
			IPAddress:      ipAddress,
			UserAgent:      userAgent,
			Timestamp:      time.Now().UTC(),
		},
	}
}

// WithUserContext enriches the event with user context information
func (e *UserActivityEvent) WithUserContext(email, firstName, lastName string) *UserActivityEvent {
	e.Data.UserEmail = email
	e.Data.UserName = firstName + " " + lastName
	return e
}

// WithOrganizationContext enriches the event with organization context
func (e *UserActivityEvent) WithOrganizationContext(orgName, orgDomain string) *UserActivityEvent {
	e.Data.OrgName = orgName
	e.Data.OrgDomain = orgDomain
	return e
}

// WithSessionContext enriches the event with session information
func (e *UserActivityEvent) WithSessionContext(sessionID, requestID string) *UserActivityEvent {
	e.Data.SessionID = sessionID
	e.Data.RequestID = requestID
	return e
}