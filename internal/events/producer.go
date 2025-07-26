package events

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/google/uuid"
	"github.com/segmentio/kafka-go"
)

type Producer struct {
	writer *kafka.Writer
}

type ProducerConfig struct {
	Brokers []string
	Topic   string
}

func NewProducer(config ProducerConfig) *Producer {
	writer := &kafka.Writer{
		Addr:         kafka.TCP(config.Brokers...),
		Topic:        config.Topic,
		Balancer:     &kafka.LeastBytes{},
		RequiredAcks: kafka.RequireOne,
		Async:        true,
		BatchSize:    100,
		BatchTimeout: 10 * time.Millisecond,
		ErrorLogger:  kafka.LoggerFunc(log.Printf),
	}

	return &Producer{
		writer: writer,
	}
}

func (p *Producer) Close() error {
	return p.writer.Close()
}

func (p *Producer) PublishEvent(ctx context.Context, event BaseEvent) error {
	eventBytes, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("failed to marshal event: %w", err)
	}

	message := kafka.Message{
		Key:   []byte(event.ID.String()),
		Value: eventBytes,
		Headers: []kafka.Header{
			{Key: "event-type", Value: []byte(event.Type)},
			{Key: "source", Value: []byte(event.Source)},
			{Key: "timestamp", Value: []byte(event.Timestamp.Format(time.RFC3339))},
		},
	}

	if event.UserID != nil {
		message.Headers = append(message.Headers, kafka.Header{
			Key:   "user-id",
			Value: []byte(event.UserID.String()),
		})
	}

	if event.OrganizationID != nil {
		message.Headers = append(message.Headers, kafka.Header{
			Key:   "organization-id",
			Value: []byte(event.OrganizationID.String()),
		})
	}

	return p.writer.WriteMessages(ctx, message)
}

// Convenience methods for publishing specific events
func (p *Producer) PublishUserRegistered(ctx context.Context, data UserRegisteredData, userID, orgID uuid.UUID, ipAddress, userAgent string) error {
	event := UserRegisteredEvent{
		BaseEvent: BaseEvent{
			ID:             uuid.New(),
			Type:           EventTypeUserRegistered,
			Source:         "auth-service",
			Timestamp:      time.Now(),
			UserID:         &userID,
			OrganizationID: &orgID,
			IPAddress:      ipAddress,
			UserAgent:      userAgent,
		},
		Data: data,
	}

	return p.PublishEvent(ctx, event.BaseEvent)
}

func (p *Producer) PublishUserLoggedIn(ctx context.Context, data UserLoggedInData, userID, orgID uuid.UUID, ipAddress, userAgent string) error {
	event := UserLoggedInEvent{
		BaseEvent: BaseEvent{
			ID:             uuid.New(),
			Type:           EventTypeUserLoggedIn,
			Source:         "auth-service",
			Timestamp:      time.Now(),
			UserID:         &userID,
			OrganizationID: &orgID,
			IPAddress:      ipAddress,
			UserAgent:      userAgent,
		},
		Data: data,
	}

	return p.PublishEvent(ctx, event.BaseEvent)
}

func (p *Producer) PublishUserLoggedOut(ctx context.Context, data UserLoggedOutData, userID, orgID uuid.UUID, ipAddress, userAgent string) error {
	event := UserLoggedOutEvent{
		BaseEvent: BaseEvent{
			ID:             uuid.New(),
			Type:           EventTypeUserLoggedOut,
			Source:         "auth-service",
			Timestamp:      time.Now(),
			UserID:         &userID,
			OrganizationID: &orgID,
			IPAddress:      ipAddress,
			UserAgent:      userAgent,
		},
		Data: data,
	}

	return p.PublishEvent(ctx, event.BaseEvent)
}

func (p *Producer) PublishPasswordChanged(ctx context.Context, data PasswordChangedData, userID, orgID uuid.UUID, ipAddress, userAgent string) error {
	event := PasswordChangedEvent{
		BaseEvent: BaseEvent{
			ID:             uuid.New(),
			Type:           EventTypePasswordChanged,
			Source:         "auth-service",
			Timestamp:      time.Now(),
			UserID:         &userID,
			OrganizationID: &orgID,
			IPAddress:      ipAddress,
			UserAgent:      userAgent,
		},
		Data: data,
	}

	return p.PublishEvent(ctx, event.BaseEvent)
}

func (p *Producer) PublishTokenRefreshed(ctx context.Context, data TokenRefreshedData, userID, orgID uuid.UUID, ipAddress, userAgent string) error {
	event := TokenRefreshedEvent{
		BaseEvent: BaseEvent{
			ID:             uuid.New(),
			Type:           EventTypeTokenRefreshed,
			Source:         "auth-service",
			Timestamp:      time.Now(),
			UserID:         &userID,
			OrganizationID: &orgID,
			IPAddress:      ipAddress,
			UserAgent:      userAgent,
		},
		Data: data,
	}

	return p.PublishEvent(ctx, event.BaseEvent)
}

func (p *Producer) PublishTokenRevoked(ctx context.Context, data TokenRevokedData, userID, orgID uuid.UUID, ipAddress, userAgent string) error {
	event := TokenRevokedEvent{
		BaseEvent: BaseEvent{
			ID:             uuid.New(),
			Type:           EventTypeTokenRevoked,
			Source:         "auth-service",
			Timestamp:      time.Now(),
			UserID:         &userID,
			OrganizationID: &orgID,
			IPAddress:      ipAddress,
			UserAgent:      userAgent,
		},
		Data: data,
	}

	return p.PublishEvent(ctx, event.BaseEvent)
}

func (p *Producer) PublishRoleAssigned(ctx context.Context, data RoleAssignedData, userID, orgID uuid.UUID, ipAddress, userAgent string) error {
	event := RoleAssignedEvent{
		BaseEvent: BaseEvent{
			ID:             uuid.New(),
			Type:           EventTypeRoleAssigned,
			Source:         "auth-service",
			Timestamp:      time.Now(),
			UserID:         &userID,
			OrganizationID: &orgID,
			IPAddress:      ipAddress,
			UserAgent:      userAgent,
		},
		Data: data,
	}

	return p.PublishEvent(ctx, event.BaseEvent)
}

func (p *Producer) PublishRoleRevoked(ctx context.Context, data RoleRevokedData, userID, orgID uuid.UUID, ipAddress, userAgent string) error {
	event := RoleRevokedEvent{
		BaseEvent: BaseEvent{
			ID:             uuid.New(),
			Type:           EventTypeRoleRevoked,
			Source:         "auth-service",
			Timestamp:      time.Now(),
			UserID:         &userID,
			OrganizationID: &orgID,
			IPAddress:      ipAddress,
			UserAgent:      userAgent,
		},
		Data: data,
	}

	return p.PublishEvent(ctx, event.BaseEvent)
}

func (p *Producer) PublishOrganizationCreated(ctx context.Context, data OrganizationCreatedData, userID, orgID uuid.UUID, ipAddress, userAgent string) error {
	event := OrganizationCreatedEvent{
		BaseEvent: BaseEvent{
			ID:             uuid.New(),
			Type:           EventTypeOrganizationCreated,
			Source:         "auth-service",
			Timestamp:      time.Now(),
			UserID:         &userID,
			OrganizationID: &orgID,
			IPAddress:      ipAddress,
			UserAgent:      userAgent,
		},
		Data: data,
	}

	return p.PublishEvent(ctx, event.BaseEvent)
}