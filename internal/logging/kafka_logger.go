package logging

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/segmentio/kafka-go"
)

// KafkaLogger sends logs to Kafka for processing by log aggregation services
type KafkaLogger struct {
	writer      *kafka.Writer
	buffer      chan LogMessage
	batchSize   int
	flushTimer  *time.Timer
	wg          sync.WaitGroup
	ctx         context.Context
	cancel      context.CancelFunc
	mu          sync.RWMutex
	closed      bool
}

// LogMessage represents a log message for Kafka
type LogMessage struct {
	Type      string      `json:"type"`
	Timestamp time.Time   `json:"@timestamp"`
	Service   string      `json:"service"`
	Data      interface{} `json:"data"`
}

// KafkaLoggerConfig holds configuration for Kafka logger
type KafkaLoggerConfig struct {
	Brokers       []string
	Topic         string
	BatchSize     int
	FlushInterval time.Duration
	BufferSize    int
}

// NewKafkaLogger creates a new Kafka logger
func NewKafkaLogger(config KafkaLoggerConfig) (*KafkaLogger, error) {
	if len(config.Brokers) == 0 {
		return nil, fmt.Errorf("kafka brokers cannot be empty")
	}
	
	if config.Topic == "" {
		config.Topic = "application-logs"
	}
	
	if config.BatchSize == 0 {
		config.BatchSize = 100
	}
	
	if config.FlushInterval == 0 {
		config.FlushInterval = 5 * time.Second
	}
	
	if config.BufferSize == 0 {
		config.BufferSize = 1000
	}
	
	writer := &kafka.Writer{
		Addr:         kafka.TCP(config.Brokers...),
		Topic:        config.Topic,
		Balancer:     &kafka.LeastBytes{},
		RequiredAcks: kafka.RequireOne,
		Async:        true,
		BatchSize:    config.BatchSize,
		BatchTimeout: config.FlushInterval,
		ErrorLogger:  kafka.LoggerFunc(func(msg string, args ...interface{}) {
			// Log Kafka errors to stderr to avoid infinite loop
			fmt.Printf("Kafka Logger Error: "+msg+"\n", args...)
		}),
	}
	
	ctx, cancel := context.WithCancel(context.Background())
	
	logger := &KafkaLogger{
		writer:     writer,
		buffer:     make(chan LogMessage, config.BufferSize),
		batchSize:  config.BatchSize,
		ctx:        ctx,
		cancel:     cancel,
	}
	
	// Start background processor
	logger.wg.Add(1)
	go logger.processLogs(config.FlushInterval)
	
	return logger, nil
}

// LogToKafka sends a log message to Kafka
func (k *KafkaLogger) LogToKafka(logType string, service string, data interface{}) error {
	k.mu.RLock()
	if k.closed {
		k.mu.RUnlock()
		return fmt.Errorf("kafka logger is closed")
	}
	k.mu.RUnlock()
	
	message := LogMessage{
		Type:      logType,
		Timestamp: time.Now().UTC(),
		Service:   service,
		Data:      data,
	}
	
	select {
	case k.buffer <- message:
		return nil
	case <-k.ctx.Done():
		return fmt.Errorf("kafka logger is shutting down")
	default:
		// Buffer is full, drop the message to prevent blocking
		return fmt.Errorf("kafka logger buffer is full, message dropped")
	}
}

// processLogs processes logs in batches
func (k *KafkaLogger) processLogs(flushInterval time.Duration) {
	defer k.wg.Done()
	
	ticker := time.NewTicker(flushInterval)
	defer ticker.Stop()
	
	batch := make([]LogMessage, 0, k.batchSize)
	
	for {
		select {
		case <-k.ctx.Done():
			// Flush remaining logs before exiting
			if len(batch) > 0 {
				k.sendBatch(batch)
			}
			return
			
		case <-ticker.C:
			// Flush batch on timer
			if len(batch) > 0 {
				k.sendBatch(batch)
				batch = batch[:0] // Reset batch
			}
			
		case message, ok := <-k.buffer:
			if !ok {
				// Channel closed, flush remaining logs
				if len(batch) > 0 {
					k.sendBatch(batch)
				}
				return
			}
			
			batch = append(batch, message)
			
			// Flush batch when it reaches the batch size
			if len(batch) >= k.batchSize {
				k.sendBatch(batch)
				batch = batch[:0] // Reset batch
			}
		}
	}
}

// sendBatch sends a batch of log messages to Kafka
func (k *KafkaLogger) sendBatch(batch []LogMessage) {
	if len(batch) == 0 {
		return
	}
	
	messages := make([]kafka.Message, len(batch))
	
	for i, logMsg := range batch {
		data, err := json.Marshal(logMsg)
		if err != nil {
			continue // Skip invalid messages
		}
		
		messages[i] = kafka.Message{
			Key:   []byte(fmt.Sprintf("%s-%s", logMsg.Service, logMsg.Type)),
			Value: data,
			Headers: []kafka.Header{
				{Key: "log-type", Value: []byte(logMsg.Type)},
				{Key: "service", Value: []byte(logMsg.Service)},
				{Key: "timestamp", Value: []byte(logMsg.Timestamp.Format(time.RFC3339))},
			},
		}
	}
	
	// Send messages to Kafka
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	
	if err := k.writer.WriteMessages(ctx, messages...); err != nil {
		// Log error to stderr to avoid infinite loop
		fmt.Printf("Failed to send log batch to Kafka: %v\n", err)
	}
}

// Close closes the Kafka logger
func (k *KafkaLogger) Close() error {
	k.mu.Lock()
	if k.closed {
		k.mu.Unlock()
		return nil
	}
	k.closed = true
	k.mu.Unlock()
	
	// Cancel context to stop background processor
	k.cancel()
	
	// Close buffer channel
	close(k.buffer)
	
	// Wait for background processor to finish
	k.wg.Wait()
	
	// Close Kafka writer
	return k.writer.Close()
}

// HealthCheck checks if Kafka logger is healthy
func (k *KafkaLogger) HealthCheck() error {
	k.mu.RLock()
	if k.closed {
		k.mu.RUnlock()
		return fmt.Errorf("kafka logger is closed")
	}
	k.mu.RUnlock()
	
	// Try to send a test message
	testMessage := LogMessage{
		Type:      "health-check",
		Timestamp: time.Now().UTC(),
		Service:   "kafka-logger",
		Data:      map[string]interface{}{"test": true},
	}
	
	data, err := json.Marshal(testMessage)
	if err != nil {
		return fmt.Errorf("failed to marshal test message: %w", err)
	}
	
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	
	message := kafka.Message{
		Key:   []byte("health-check"),
		Value: data,
	}
	
	return k.writer.WriteMessages(ctx, message)
}

// GetStats returns statistics about the Kafka logger
func (k *KafkaLogger) GetStats() map[string]interface{} {
	k.mu.RLock()
	defer k.mu.RUnlock()
	
	return map[string]interface{}{
		"buffer_size":     len(k.buffer),
		"buffer_capacity": cap(k.buffer),
		"is_closed":       k.closed,
	}
}