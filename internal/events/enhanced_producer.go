package events

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"erp-auth-service/internal/config"
	"github.com/google/uuid"
	"github.com/segmentio/kafka-go"
	"go.uber.org/zap"
)

// EventStatus represents the status of an event
type EventStatus int

const (
	EventStatusPending EventStatus = iota
	EventStatusProcessing
	EventStatusSent
	EventStatusFailed
	EventStatusDeadLetter
)

// QueuedEvent represents an event in the local queue
type QueuedEvent struct {
	ID          uuid.UUID   `json:"id"`
	Event       BaseEvent   `json:"event"`
	Attempts    int         `json:"attempts"`
	Status      EventStatus `json:"status"`
	CreatedAt   time.Time   `json:"created_at"`
	LastAttempt time.Time   `json:"last_attempt"`
	NextRetry   time.Time   `json:"next_retry"`
	Error       string      `json:"error,omitempty"`
}

// EnhancedProducer provides high-performance event publishing with reliability features
type EnhancedProducer struct {
	config          config.KafkaConfig
	logger          *zap.Logger
	writers         []*kafka.Writer
	writerPool      chan *kafka.Writer
	localQueue      chan *QueuedEvent
	persistentQueue *PersistentQueue
	deadLetterQueue chan *QueuedEvent
	
	// Metrics and monitoring
	metrics *ProducerMetrics
	
	// Control channels
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
	
	// State management
	isRunning bool
	mu        sync.RWMutex
}



// ProducerMetrics tracks producer performance
type ProducerMetrics struct {
	EventsPublished    int64
	EventsFailed       int64
	EventsRetried      int64
	EventsDeadLettered int64
	QueueSize          int64
	AvgLatency         time.Duration
	mu                 sync.RWMutex
}

// NewEnhancedProducer creates a new enhanced Kafka producer
func NewEnhancedProducer(config config.KafkaConfig, logger *zap.Logger) (*EnhancedProducer, error) {
	ctx, cancel := context.WithCancel(context.Background())
	
	producer := &EnhancedProducer{
		config:          config,
		logger:          logger,
		writers:         make([]*kafka.Writer, 0, config.ConnectionPoolSize),
		writerPool:      make(chan *kafka.Writer, config.ConnectionPoolSize),
		localQueue:      make(chan *QueuedEvent, config.LocalQueueSize),
		deadLetterQueue: make(chan *QueuedEvent, 1000), // Fixed size for DLQ
		metrics:         &ProducerMetrics{},
		ctx:             ctx,
		cancel:          cancel,
		isRunning:       false,
	}
	
	// Initialize connection pool
	if err := producer.initializeConnectionPool(); err != nil {
		cancel()
		return nil, fmt.Errorf("failed to initialize connection pool: %w", err)
	}
	
	// Initialize persistent queue if enabled
	if config.EnableLocalPersistence {
		var err error
		producer.persistentQueue, err = NewPersistentQueue(config.LocalStoragePath, logger)
		if err != nil {
			cancel()
			return nil, fmt.Errorf("failed to initialize persistent queue: %w", err)
		}
	}
	
	return producer, nil
}

// initializeConnectionPool creates a pool of Kafka writers
func (p *EnhancedProducer) initializeConnectionPool() error {
	for i := 0; i < p.config.ConnectionPoolSize; i++ {
		writer := &kafka.Writer{
			Addr:         kafka.TCP(p.config.Brokers...),
			Topic:        p.config.Topic,
			Balancer:     &kafka.LeastBytes{},
			RequiredAcks: kafka.RequireOne,
			Async:        false, // We handle async ourselves
			BatchSize:    p.config.BatchSize,
			BatchTimeout: time.Duration(p.config.BatchTimeoutMs) * time.Millisecond,
			ErrorLogger:  kafka.LoggerFunc(p.logger.Sugar().Errorf),
		}
		
		p.writers = append(p.writers, writer)
		p.writerPool <- writer
	}
	
	return nil
}

// Start begins the producer's background processing
func (p *EnhancedProducer) Start() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	
	if p.isRunning {
		return fmt.Errorf("producer is already running")
	}
	
	p.isRunning = true
	
	// Start worker goroutines
	p.wg.Add(3)
	go p.processLocalQueue()
	go p.processDeadLetterQueue()
	go p.retryFailedEvents()
	
	// Load persisted events if enabled
	if p.persistentQueue != nil {
		go p.loadPersistedEvents()
	}
	
	p.logger.Info("Enhanced Kafka producer started",
		zap.Int("connection_pool_size", p.config.ConnectionPoolSize),
		zap.Int("local_queue_size", p.config.LocalQueueSize),
		zap.Bool("persistence_enabled", p.config.EnableLocalPersistence))
	
	return nil
}

// Stop gracefully shuts down the producer
func (p *EnhancedProducer) Stop() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	
	if !p.isRunning {
		return nil
	}
	
	p.logger.Info("Stopping enhanced Kafka producer...")
	
	// Cancel context to signal shutdown
	p.cancel()
	
	// Wait for all goroutines to finish
	p.wg.Wait()
	
	// Close all writers
	for _, writer := range p.writers {
		if err := writer.Close(); err != nil {
			p.logger.Error("Error closing Kafka writer", zap.Error(err))
		}
	}
	
	// Close persistent queue
	if p.persistentQueue != nil {
		if err := p.persistentQueue.Close(); err != nil {
			p.logger.Error("Error closing persistent queue", zap.Error(err))
		}
	}
	
	p.isRunning = false
	p.logger.Info("Enhanced Kafka producer stopped")
	
	return nil
}

// PublishEvent publishes an event asynchronously
func (p *EnhancedProducer) PublishEvent(ctx context.Context, event BaseEvent) error {
	if !p.isRunning {
		return fmt.Errorf("producer is not running")
	}
	
	queuedEvent := &QueuedEvent{
		ID:        uuid.New(),
		Event:     event,
		Attempts:  0,
		Status:    EventStatusPending,
		CreatedAt: time.Now(),
	}
	
	// Try to add to local queue
	select {
	case p.localQueue <- queuedEvent:
		p.updateMetrics(func(m *ProducerMetrics) {
			m.QueueSize++
		})
		return nil
	case <-ctx.Done():
		return ctx.Err()
	default:
		// Queue is full, persist if enabled
		if p.persistentQueue != nil {
			return p.persistentQueue.Enqueue(queuedEvent)
		}
		return fmt.Errorf("local queue is full and persistence is disabled")
	}
}

// processLocalQueue processes events from the local queue
func (p *EnhancedProducer) processLocalQueue() {
	defer p.wg.Done()
	
	for {
		select {
		case <-p.ctx.Done():
			return
		case queuedEvent := <-p.localQueue:
			p.updateMetrics(func(m *ProducerMetrics) {
				m.QueueSize--
			})
			
			if err := p.sendEvent(queuedEvent); err != nil {
				p.handleFailedEvent(queuedEvent, err)
			}
		}
	}
}

// sendEvent sends an event to Kafka
func (p *EnhancedProducer) sendEvent(queuedEvent *QueuedEvent) error {
	start := time.Now()
	
	// Get writer from pool
	var writer *kafka.Writer
	select {
	case writer = <-p.writerPool:
		defer func() { p.writerPool <- writer }()
	case <-p.ctx.Done():
		return p.ctx.Err()
	}
	
	// Prepare Kafka message
	eventBytes, err := json.Marshal(queuedEvent.Event)
	if err != nil {
		return fmt.Errorf("failed to marshal event: %w", err)
	}
	
	message := kafka.Message{
		Key:   []byte(queuedEvent.Event.ID.String()),
		Value: eventBytes,
		Headers: []kafka.Header{
			{Key: "event-type", Value: []byte(queuedEvent.Event.Type)},
			{Key: "source", Value: []byte(queuedEvent.Event.Source)},
			{Key: "timestamp", Value: []byte(queuedEvent.Event.Timestamp.Format(time.RFC3339))},
			{Key: "attempt", Value: []byte(fmt.Sprintf("%d", queuedEvent.Attempts+1))},
		},
	}
	
	if queuedEvent.Event.UserID != nil {
		message.Headers = append(message.Headers, kafka.Header{
			Key:   "user-id",
			Value: []byte(queuedEvent.Event.UserID.String()),
		})
	}
	
	if queuedEvent.Event.OrganizationID != nil {
		message.Headers = append(message.Headers, kafka.Header{
			Key:   "organization-id",
			Value: []byte(queuedEvent.Event.OrganizationID.String()),
		})
	}
	
	// Send message
	queuedEvent.Status = EventStatusProcessing
	queuedEvent.Attempts++
	queuedEvent.LastAttempt = time.Now()
	
	if err := writer.WriteMessages(p.ctx, message); err != nil {
		return fmt.Errorf("failed to write message to Kafka: %w", err)
	}
	
	// Update metrics
	latency := time.Since(start)
	p.updateMetrics(func(m *ProducerMetrics) {
		m.EventsPublished++
		m.AvgLatency = (m.AvgLatency + latency) / 2
	})
	
	queuedEvent.Status = EventStatusSent
	
	p.logger.Debug("Event published successfully",
		zap.String("event_id", queuedEvent.ID.String()),
		zap.String("event_type", queuedEvent.Event.Type),
		zap.Duration("latency", latency),
		zap.Int("attempt", queuedEvent.Attempts))
	
	return nil
}

// handleFailedEvent handles events that failed to send
func (p *EnhancedProducer) handleFailedEvent(queuedEvent *QueuedEvent, err error) {
	queuedEvent.Status = EventStatusFailed
	queuedEvent.Error = err.Error()
	
	p.logger.Warn("Event failed to publish",
		zap.String("event_id", queuedEvent.ID.String()),
		zap.String("event_type", queuedEvent.Event.Type),
		zap.Int("attempt", queuedEvent.Attempts),
		zap.Error(err))
	
	// Check if we should retry
	if queuedEvent.Attempts < p.config.RetryAttempts {
		// Calculate next retry time with exponential backoff
		backoffMs := p.config.RetryBackoffMs * (1 << (queuedEvent.Attempts - 1))
		queuedEvent.NextRetry = time.Now().Add(time.Duration(backoffMs) * time.Millisecond)
		
		// Persist for retry if persistence is enabled
		if p.persistentQueue != nil {
			if err := p.persistentQueue.Enqueue(queuedEvent); err != nil {
				p.logger.Error("Failed to persist event for retry", zap.Error(err))
			}
		}
		
		p.updateMetrics(func(m *ProducerMetrics) {
			m.EventsRetried++
		})
	} else {
		// Send to dead letter queue
		queuedEvent.Status = EventStatusDeadLetter
		select {
		case p.deadLetterQueue <- queuedEvent:
		default:
			p.logger.Error("Dead letter queue is full, dropping event",
				zap.String("event_id", queuedEvent.ID.String()))
		}
		
		p.updateMetrics(func(m *ProducerMetrics) {
			m.EventsFailed++
		})
	}
}

// processDeadLetterQueue processes events that couldn't be delivered
func (p *EnhancedProducer) processDeadLetterQueue() {
	defer p.wg.Done()
	
	// Create dead letter writer
	dlqWriter := &kafka.Writer{
		Addr:         kafka.TCP(p.config.Brokers...),
		Topic:        p.config.DeadLetterTopic,
		Balancer:     &kafka.LeastBytes{},
		RequiredAcks: kafka.RequireOne,
		ErrorLogger:  kafka.LoggerFunc(p.logger.Sugar().Errorf),
	}
	defer dlqWriter.Close()
	
	for {
		select {
		case <-p.ctx.Done():
			return
		case queuedEvent := <-p.deadLetterQueue:
			if err := p.sendToDeadLetterQueue(dlqWriter, queuedEvent); err != nil {
				p.logger.Error("Failed to send event to dead letter queue",
					zap.String("event_id", queuedEvent.ID.String()),
					zap.Error(err))
			} else {
				p.updateMetrics(func(m *ProducerMetrics) {
					m.EventsDeadLettered++
				})
			}
		}
	}
}

// sendToDeadLetterQueue sends an event to the dead letter queue
func (p *EnhancedProducer) sendToDeadLetterQueue(writer *kafka.Writer, queuedEvent *QueuedEvent) error {
	dlqEvent := map[string]interface{}{
		"original_event": queuedEvent.Event,
		"failure_info": map[string]interface{}{
			"attempts":     queuedEvent.Attempts,
			"last_error":   queuedEvent.Error,
			"failed_at":    queuedEvent.LastAttempt,
			"created_at":   queuedEvent.CreatedAt,
		},
	}
	
	eventBytes, err := json.Marshal(dlqEvent)
	if err != nil {
		return fmt.Errorf("failed to marshal dead letter event: %w", err)
	}
	
	message := kafka.Message{
		Key:   []byte(queuedEvent.Event.ID.String()),
		Value: eventBytes,
		Headers: []kafka.Header{
			{Key: "original-event-type", Value: []byte(queuedEvent.Event.Type)},
			{Key: "failure-reason", Value: []byte("max-retries-exceeded")},
			{Key: "dlq-timestamp", Value: []byte(time.Now().Format(time.RFC3339))},
		},
	}
	
	return writer.WriteMessages(p.ctx, message)
}

// retryFailedEvents processes events that need to be retried
func (p *EnhancedProducer) retryFailedEvents() {
	defer p.wg.Done()
	
	if p.persistentQueue == nil {
		return // No persistence, no retries
	}
	
	ticker := time.NewTicker(time.Second * 10) // Check every 10 seconds
	defer ticker.Stop()
	
	for {
		select {
		case <-p.ctx.Done():
			return
		case <-ticker.C:
			events, err := p.persistentQueue.GetRetryableEvents()
			if err != nil {
				p.logger.Error("Failed to get retryable events", zap.Error(err))
				continue
			}
			
			for _, event := range events {
				if time.Now().After(event.NextRetry) {
					// Try to add back to local queue
					select {
					case p.localQueue <- event:
						p.updateMetrics(func(m *ProducerMetrics) {
							m.QueueSize++
						})
						// Remove from persistent storage
						if err := p.persistentQueue.Remove(event.ID); err != nil {
							p.logger.Error("Failed to remove retried event from persistent queue", zap.Error(err))
						}
					default:
						// Local queue is full, leave in persistent storage
						break
					}
				}
			}
		}
	}
}

// loadPersistedEvents loads events from persistent storage on startup
func (p *EnhancedProducer) loadPersistedEvents() {
	if p.persistentQueue == nil {
		return
	}
	
	events, err := p.persistentQueue.GetAll()
	if err != nil {
		p.logger.Error("Failed to load persisted events", zap.Error(err))
		return
	}
	
	p.logger.Info("Loading persisted events", zap.Int("count", len(events)))
	
	for _, event := range events {
		select {
		case p.localQueue <- event:
			p.updateMetrics(func(m *ProducerMetrics) {
				m.QueueSize++
			})
			// Remove from persistent storage
			if err := p.persistentQueue.Remove(event.ID); err != nil {
				p.logger.Error("Failed to remove loaded event from persistent queue", zap.Error(err))
			}
		case <-p.ctx.Done():
			return
		default:
			// Local queue is full, leave in persistent storage
			break
		}
	}
}

// updateMetrics safely updates producer metrics
func (p *EnhancedProducer) updateMetrics(fn func(*ProducerMetrics)) {
	p.metrics.mu.Lock()
	defer p.metrics.mu.Unlock()
	fn(p.metrics)
}

// GetMetrics returns current producer metrics
func (p *EnhancedProducer) GetMetrics() ProducerMetrics {
	p.metrics.mu.RLock()
	defer p.metrics.mu.RUnlock()
	return *p.metrics
}

// Health check for the producer
func (p *EnhancedProducer) IsHealthy() bool {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.isRunning
}

// Convenience methods for publishing specific events (matching existing API)

func (p *EnhancedProducer) PublishUserRegistered(ctx context.Context, data UserRegisteredData, userID, orgID uuid.UUID, ipAddress, userAgent string) error {
	event := BaseEvent{
		ID:             uuid.New(),
		Type:           EventTypeUserRegistered,
		Source:         "auth-service",
		Timestamp:      time.Now(),
		UserID:         &userID,
		OrganizationID: &orgID,
		IPAddress:      ipAddress,
		UserAgent:      userAgent,
		Data:           data,
	}
	
	return p.PublishEvent(ctx, event)
}

func (p *EnhancedProducer) PublishUserLoggedIn(ctx context.Context, data UserLoggedInData, userID, orgID uuid.UUID, ipAddress, userAgent string) error {
	event := BaseEvent{
		ID:             uuid.New(),
		Type:           EventTypeUserLoggedIn,
		Source:         "auth-service",
		Timestamp:      time.Now(),
		UserID:         &userID,
		OrganizationID: &orgID,
		IPAddress:      ipAddress,
		UserAgent:      userAgent,
		Data:           data,
	}
	
	return p.PublishEvent(ctx, event)
}

func (p *EnhancedProducer) PublishUserLoggedOut(ctx context.Context, data UserLoggedOutData, userID, orgID uuid.UUID, ipAddress, userAgent string) error {
	event := BaseEvent{
		ID:             uuid.New(),
		Type:           EventTypeUserLoggedOut,
		Source:         "auth-service",
		Timestamp:      time.Now(),
		UserID:         &userID,
		OrganizationID: &orgID,
		IPAddress:      ipAddress,
		UserAgent:      userAgent,
		Data:           data,
	}
	
	return p.PublishEvent(ctx, event)
}

func (p *EnhancedProducer) PublishPasswordChanged(ctx context.Context, data PasswordChangedData, userID, orgID uuid.UUID, ipAddress, userAgent string) error {
	event := BaseEvent{
		ID:             uuid.New(),
		Type:           EventTypePasswordChanged,
		Source:         "auth-service",
		Timestamp:      time.Now(),
		UserID:         &userID,
		OrganizationID: &orgID,
		IPAddress:      ipAddress,
		UserAgent:      userAgent,
		Data:           data,
	}
	
	return p.PublishEvent(ctx, event)
}

func (p *EnhancedProducer) PublishTokenRefreshed(ctx context.Context, data TokenRefreshedData, userID, orgID uuid.UUID, ipAddress, userAgent string) error {
	event := BaseEvent{
		ID:             uuid.New(),
		Type:           EventTypeTokenRefreshed,
		Source:         "auth-service",
		Timestamp:      time.Now(),
		UserID:         &userID,
		OrganizationID: &orgID,
		IPAddress:      ipAddress,
		UserAgent:      userAgent,
		Data:           data,
	}
	
	return p.PublishEvent(ctx, event)
}

func (p *EnhancedProducer) PublishTokenRevoked(ctx context.Context, data TokenRevokedData, userID, orgID uuid.UUID, ipAddress, userAgent string) error {
	event := BaseEvent{
		ID:             uuid.New(),
		Type:           EventTypeTokenRevoked,
		Source:         "auth-service",
		Timestamp:      time.Now(),
		UserID:         &userID,
		OrganizationID: &orgID,
		IPAddress:      ipAddress,
		UserAgent:      userAgent,
		Data:           data,
	}
	
	return p.PublishEvent(ctx, event)
}

func (p *EnhancedProducer) PublishRoleAssigned(ctx context.Context, data RoleAssignedData, userID, orgID uuid.UUID, ipAddress, userAgent string) error {
	event := BaseEvent{
		ID:             uuid.New(),
		Type:           EventTypeRoleAssigned,
		Source:         "auth-service",
		Timestamp:      time.Now(),
		UserID:         &userID,
		OrganizationID: &orgID,
		IPAddress:      ipAddress,
		UserAgent:      userAgent,
		Data:           data,
	}
	
	return p.PublishEvent(ctx, event)
}

func (p *EnhancedProducer) PublishRoleRevoked(ctx context.Context, data RoleRevokedData, userID, orgID uuid.UUID, ipAddress, userAgent string) error {
	event := BaseEvent{
		ID:             uuid.New(),
		Type:           EventTypeRoleRevoked,
		Source:         "auth-service",
		Timestamp:      time.Now(),
		UserID:         &userID,
		OrganizationID: &orgID,
		IPAddress:      ipAddress,
		UserAgent:      userAgent,
		Data:           data,
	}
	
	return p.PublishEvent(ctx, event)
}

func (p *EnhancedProducer) PublishOrganizationCreated(ctx context.Context, data OrganizationCreatedData, userID, orgID uuid.UUID, ipAddress, userAgent string) error {
	event := BaseEvent{
		ID:             uuid.New(),
		Type:           EventTypeOrganizationCreated,
		Source:         "auth-service",
		Timestamp:      time.Now(),
		UserID:         &userID,
		OrganizationID: &orgID,
		IPAddress:      ipAddress,
		UserAgent:      userAgent,
		Data:           data,
	}
	
	return p.PublishEvent(ctx, event)
}

func (p *EnhancedProducer) Publish2FAEnabled(ctx context.Context, data TwoFAEnabledData, userID, orgID uuid.UUID, ipAddress, userAgent string) error {
	event := BaseEvent{
		ID:             uuid.New(),
		Type:           EventType2FAEnabled,
		Source:         "auth-service",
		Timestamp:      time.Now(),
		UserID:         &userID,
		OrganizationID: &orgID,
		IPAddress:      ipAddress,
		UserAgent:      userAgent,
		Data:           data,
	}
	
	return p.PublishEvent(ctx, event)
}

func (p *EnhancedProducer) Publish2FADisabled(ctx context.Context, data TwoFADisabledData, userID, orgID uuid.UUID, ipAddress, userAgent string) error {
	event := BaseEvent{
		ID:             uuid.New(),
		Type:           EventType2FADisabled,
		Source:         "auth-service",
		Timestamp:      time.Now(),
		UserID:         &userID,
		OrganizationID: &orgID,
		IPAddress:      ipAddress,
		UserAgent:      userAgent,
		Data:           data,
	}
	
	return p.PublishEvent(ctx, event)
}

func (p *EnhancedProducer) PublishOrganizationUpdated(ctx context.Context, data OrganizationUpdatedData, userID, orgID uuid.UUID, ipAddress, userAgent string) error {
	event := BaseEvent{
		ID:             uuid.New(),
		Type:           EventTypeOrganizationUpdated,
		Source:         "auth-service",
		Timestamp:      time.Now(),
		UserID:         &userID,
		OrganizationID: &orgID,
		IPAddress:      ipAddress,
		UserAgent:      userAgent,
		Data:           data,
	}
	
	return p.PublishEvent(ctx, event)
}