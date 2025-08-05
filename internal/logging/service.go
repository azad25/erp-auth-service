package logging

import (
	"context"
	"fmt"
	"os"
	"sync"
	"time"
)

// LoggingService provides centralized logging functionality
type LoggingService struct {
	structuredLogger Logger
	kafkaLogger      *KafkaLogger
	service          string
	environment      string
	mu               sync.RWMutex
	closed           bool
}

// LoggingServiceConfig holds configuration for the logging service
type LoggingServiceConfig struct {
	Service     string
	Component   string
	Environment string
	
	// Kafka configuration
	KafkaEnabled bool
	KafkaConfig  KafkaLoggerConfig
}

// NewLoggingService creates a new logging service
func NewLoggingService(config LoggingServiceConfig) (*LoggingService, error) {
	// Create structured logger
	structuredLogger := NewStructuredLogger(config.Service, config.Component)
	
	service := &LoggingService{
		structuredLogger: structuredLogger,
		service:          config.Service,
		environment:      config.Environment,
	}
	
	// Initialize Kafka logger if enabled
	if config.KafkaEnabled {
		kafkaLogger, err := NewKafkaLogger(config.KafkaConfig)
		if err != nil {
			// Log warning but don't fail service initialization
			structuredLogger.Warn(context.Background(), "Failed to initialize Kafka logger", map[string]interface{}{
				"error": err.Error(),
			})
		} else {
			service.kafkaLogger = kafkaLogger
		}
	}
	
	return service, nil
}

// Debug logs a debug message
func (s *LoggingService) Debug(ctx context.Context, message string, fields map[string]interface{}) {
	s.mu.RLock()
	if s.closed {
		s.mu.RUnlock()
		return
	}
	s.mu.RUnlock()
	
	s.structuredLogger.Debug(ctx, message, fields)
	
	// Send to Kafka if available
	if s.kafkaLogger != nil {
		logData := map[string]interface{}{
			"level":   "debug",
			"message": message,
			"fields":  fields,
		}
		s.kafkaLogger.LogToKafka("debug", s.service, logData)
	}
}

// Info logs an info message
func (s *LoggingService) Info(ctx context.Context, message string, fields map[string]interface{}) {
	s.mu.RLock()
	if s.closed {
		s.mu.RUnlock()
		return
	}
	s.mu.RUnlock()
	
	s.structuredLogger.Info(ctx, message, fields)
	
	// Send to Kafka if available
	if s.kafkaLogger != nil {
		logData := map[string]interface{}{
			"level":   "info",
			"message": message,
			"fields":  fields,
		}
		s.kafkaLogger.LogToKafka("info", s.service, logData)
	}
}

// Warn logs a warning message
func (s *LoggingService) Warn(ctx context.Context, message string, fields map[string]interface{}) {
	s.mu.RLock()
	if s.closed {
		s.mu.RUnlock()
		return
	}
	s.mu.RUnlock()
	
	s.structuredLogger.Warn(ctx, message, fields)
	
	// Send to Kafka if available
	if s.kafkaLogger != nil {
		logData := map[string]interface{}{
			"level":   "warn",
			"message": message,
			"fields":  fields,
		}
		s.kafkaLogger.LogToKafka("warn", s.service, logData)
	}
}

// Error logs an error message
func (s *LoggingService) Error(ctx context.Context, message string, fields map[string]interface{}) {
	s.mu.RLock()
	if s.closed {
		s.mu.RUnlock()
		return
	}
	s.mu.RUnlock()
	
	s.structuredLogger.Error(ctx, message, fields)
	
	// Send to Kafka if available (high priority)
	if s.kafkaLogger != nil {
		logData := map[string]interface{}{
			"level":   "error",
			"message": message,
			"fields":  fields,
		}
		s.kafkaLogger.LogToKafka("error", s.service, logData)
	}
}

// Fatal logs a fatal message
func (s *LoggingService) Fatal(ctx context.Context, message string, fields map[string]interface{}) {
	s.mu.RLock()
	if s.closed {
		s.mu.RUnlock()
		return
	}
	s.mu.RUnlock()
	
	// Send to Kafka first (before potential exit)
	if s.kafkaLogger != nil {
		logData := map[string]interface{}{
			"level":   "fatal",
			"message": message,
			"fields":  fields,
		}
		s.kafkaLogger.LogToKafka("fatal", s.service, logData)
		
		// Give Kafka a moment to send the message
		time.Sleep(100 * time.Millisecond)
	}
	
	s.structuredLogger.Fatal(ctx, message, fields)
}

// LogRequest logs an HTTP request
func (s *LoggingService) LogRequest(ctx context.Context, entry RequestLogEntry) {
	s.mu.RLock()
	if s.closed {
		s.mu.RUnlock()
		return
	}
	s.mu.RUnlock()
	
	s.structuredLogger.LogRequest(ctx, entry)
	
	// Send to Kafka if available
	if s.kafkaLogger != nil {
		s.kafkaLogger.LogToKafka("request", s.service, entry)
	}
}

// LogError logs an error entry
func (s *LoggingService) LogError(ctx context.Context, entry ErrorLogEntry) {
	s.mu.RLock()
	if s.closed {
		s.mu.RUnlock()
		return
	}
	s.mu.RUnlock()
	
	s.structuredLogger.LogError(ctx, entry)
	
	// Send to Kafka if available
	if s.kafkaLogger != nil {
		s.kafkaLogger.LogToKafka("error", s.service, entry)
	}
}

// LogEvent logs a business event
func (s *LoggingService) LogEvent(ctx context.Context, entry EventLogEntry) {
	s.mu.RLock()
	if s.closed {
		s.mu.RUnlock()
		return
	}
	s.mu.RUnlock()
	
	s.structuredLogger.LogEvent(ctx, entry)
	
	// Send to Kafka if available
	if s.kafkaLogger != nil {
		s.kafkaLogger.LogToKafka("event", s.service, entry)
	}
}

// LogMetric logs a metric entry
func (s *LoggingService) LogMetric(ctx context.Context, entry MetricLogEntry) {
	s.mu.RLock()
	if s.closed {
		s.mu.RUnlock()
		return
	}
	s.mu.RUnlock()
	
	s.structuredLogger.LogMetric(ctx, entry)
	
	// Send to Kafka if available
	if s.kafkaLogger != nil {
		s.kafkaLogger.LogToKafka("metric", s.service, entry)
	}
}

// WithFields returns a new logger with additional fields
func (s *LoggingService) WithFields(fields map[string]interface{}) Logger {
	s.mu.RLock()
	defer s.mu.RUnlock()
	
	if s.closed {
		return s.structuredLogger
	}
	
	return s.structuredLogger.WithFields(fields)
}

// WithContext returns a new logger with context
func (s *LoggingService) WithContext(ctx context.Context) Logger {
	s.mu.RLock()
	defer s.mu.RUnlock()
	
	if s.closed {
		return s.structuredLogger
	}
	
	return s.structuredLogger.WithContext(ctx)
}

// Close closes the logging service
func (s *LoggingService) Close() error {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return nil
	}
	s.closed = true
	s.mu.Unlock()
	
	var err error
	
	// Close Kafka logger if available
	if s.kafkaLogger != nil {
		if kafkaErr := s.kafkaLogger.Close(); kafkaErr != nil {
			err = fmt.Errorf("failed to close Kafka logger: %w", kafkaErr)
		}
	}
	
	return err
}

// HealthCheck performs a health check on all logging components
func (s *LoggingService) HealthCheck() error {
	s.mu.RLock()
	if s.closed {
		s.mu.RUnlock()
		return fmt.Errorf("logging service is closed")
	}
	s.mu.RUnlock()
	
	// Check Kafka logger if available
	if s.kafkaLogger != nil {
		if err := s.kafkaLogger.HealthCheck(); err != nil {
			return fmt.Errorf("kafka logger health check failed: %w", err)
		}
	}
	
	return nil
}

// GetStats returns statistics about the logging service
func (s *LoggingService) GetStats() map[string]interface{} {
	s.mu.RLock()
	defer s.mu.RUnlock()
	
	stats := map[string]interface{}{
		"service":     s.service,
		"environment": s.environment,
		"is_closed":   s.closed,
	}
	
	// Add Kafka stats if available
	if s.kafkaLogger != nil {
		stats["kafka"] = s.kafkaLogger.GetStats()
	}
	
	return stats
}

// Global logging service instance
var globalLoggingService *LoggingService
var globalLoggingServiceOnce sync.Once

// InitializeGlobalLogger initializes the global logging service
func InitializeGlobalLogger(config LoggingServiceConfig) error {
	var err error
	globalLoggingServiceOnce.Do(func() {
		globalLoggingService, err = NewLoggingService(config)
	})
	return err
}

// GetGlobalLogger returns the global logging service
func GetGlobalLogger() Logger {
	if globalLoggingService == nil {
		// Fallback to basic structured logger
		return NewStructuredLogger("unknown", "unknown")
	}
	return globalLoggingService
}

// CloseGlobalLogger closes the global logging service
func CloseGlobalLogger() error {
	if globalLoggingService != nil {
		return globalLoggingService.Close()
	}
	return nil
}

// DefaultLoggingServiceConfig returns a default configuration
func DefaultLoggingServiceConfig() LoggingServiceConfig {
	return LoggingServiceConfig{
		Service:     "auth-service",
		Component:   "main",
		Environment: getEnvOrDefault("ENV", "development"),
		KafkaEnabled: getEnvOrDefault("KAFKA_LOGGING_ENABLED", "true") == "true",
		KafkaConfig: KafkaLoggerConfig{
			Brokers:       []string{getEnvOrDefault("KAFKA_BROKERS", "localhost:9092")},
			Topic:         getEnvOrDefault("KAFKA_LOG_TOPIC", "application-logs"),
			BatchSize:     100,
			FlushInterval: 5 * time.Second,
			BufferSize:    1000,
		},
	}
}

// getEnvOrDefault returns environment variable value or default
func getEnvOrDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}