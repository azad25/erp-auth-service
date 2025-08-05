package logging

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"runtime"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/sirupsen/logrus"
)

// LogLevel represents the severity level of a log entry
type LogLevel string

const (
	LogLevelDebug LogLevel = "debug"
	LogLevelInfo  LogLevel = "info"
	LogLevelWarn  LogLevel = "warn"
	LogLevelError LogLevel = "error"
	LogLevelFatal LogLevel = "fatal"
)

// Logger interface defines the logging contract
type Logger interface {
	Debug(ctx context.Context, message string, fields map[string]interface{})
	Info(ctx context.Context, message string, fields map[string]interface{})
	Warn(ctx context.Context, message string, fields map[string]interface{})
	Error(ctx context.Context, message string, fields map[string]interface{})
	Fatal(ctx context.Context, message string, fields map[string]interface{})
	
	// Structured logging methods
	LogRequest(ctx context.Context, entry RequestLogEntry)
	LogError(ctx context.Context, entry ErrorLogEntry)
	LogEvent(ctx context.Context, entry EventLogEntry)
	LogMetric(ctx context.Context, entry MetricLogEntry)
	
	// Utility methods
	WithFields(fields map[string]interface{}) Logger
	WithContext(ctx context.Context) Logger
}

// LogEntry represents a structured log entry
type LogEntry struct {
	Timestamp     time.Time              `json:"@timestamp"`
	Level         LogLevel               `json:"level"`
	Message       string                 `json:"message"`
	Service       string                 `json:"service"`
	Component     string                 `json:"component"`
	RequestID     string                 `json:"request_id,omitempty"`
	UserID        string                 `json:"user_id,omitempty"`
	CorrelationID string                 `json:"correlation_id,omitempty"`
	Fields        map[string]interface{} `json:"fields,omitempty"`
	StackTrace    string                 `json:"stack_trace,omitempty"`
	Environment   string                 `json:"environment"`
	Version       string                 `json:"version"`
}

// RequestLogEntry represents an HTTP request log entry
type RequestLogEntry struct {
	Timestamp     time.Time              `json:"@timestamp"`
	RequestID     string                 `json:"request_id"`
	UserID        string                 `json:"user_id,omitempty"`
	Method        string                 `json:"method"`
	Path          string                 `json:"path"`
	Query         string                 `json:"query,omitempty"`
	StatusCode    int                    `json:"status_code"`
	Duration      time.Duration          `json:"duration_ms"`
	UserAgent     string                 `json:"user_agent,omitempty"`
	RemoteIP      string                 `json:"remote_ip"`
	RequestSize   int64                  `json:"request_size_bytes"`
	ResponseSize  int64                  `json:"response_size_bytes"`
	Headers       map[string]string      `json:"headers,omitempty"`
	Service       string                 `json:"service"`
	Environment   string                 `json:"environment"`
}

// ErrorLogEntry represents an error log entry
type ErrorLogEntry struct {
	Timestamp     time.Time              `json:"@timestamp"`
	RequestID     string                 `json:"request_id,omitempty"`
	UserID        string                 `json:"user_id,omitempty"`
	Level         LogLevel               `json:"level"`
	Message       string                 `json:"message"`
	Error         string                 `json:"error,omitempty"`
	StackTrace    string                 `json:"stack_trace,omitempty"`
	Context       map[string]interface{} `json:"context,omitempty"`
	Service       string                 `json:"service"`
	Component     string                 `json:"component"`
	Environment   string                 `json:"environment"`
}

// EventLogEntry represents a business event log entry
type EventLogEntry struct {
	Timestamp     time.Time              `json:"@timestamp"`
	EventID       string                 `json:"event_id"`
	EventType     string                 `json:"event_type"`
	RequestID     string                 `json:"request_id,omitempty"`
	UserID        string                 `json:"user_id,omitempty"`
	CorrelationID string                 `json:"correlation_id,omitempty"`
	Source        string                 `json:"source"`
	Data          map[string]interface{} `json:"data,omitempty"`
	Success       bool                   `json:"success"`
	Service       string                 `json:"service"`
	Environment   string                 `json:"environment"`
}

// MetricLogEntry represents a metric log entry
type MetricLogEntry struct {
	Timestamp   time.Time              `json:"@timestamp"`
	MetricName  string                 `json:"metric_name"`
	MetricType  string                 `json:"metric_type"` // counter, gauge, histogram, timer
	Value       float64                `json:"value"`
	Labels      map[string]string      `json:"labels,omitempty"`
	Unit        string                 `json:"unit,omitempty"`
	Service     string                 `json:"service"`
	Environment string                 `json:"environment"`
}

// StructuredLogger implements the Logger interface using logrus
type StructuredLogger struct {
	logger      *logrus.Logger
	service     string
	component   string
	environment string
	version     string
	fields      map[string]interface{}
}

// NewStructuredLogger creates a new structured logger
func NewStructuredLogger(service, component string) Logger {
	logger := logrus.New()
	
	// Set JSON formatter
	logger.SetFormatter(&logrus.JSONFormatter{
		TimestampFormat: time.RFC3339Nano,
		FieldMap: logrus.FieldMap{
			logrus.FieldKeyTime:  "@timestamp",
			logrus.FieldKeyLevel: "level",
			logrus.FieldKeyMsg:   "message",
		},
	})
	
	// Set log level from environment
	level := strings.ToLower(os.Getenv("LOG_LEVEL"))
	switch level {
	case "debug":
		logger.SetLevel(logrus.DebugLevel)
	case "info":
		logger.SetLevel(logrus.InfoLevel)
	case "warn":
		logger.SetLevel(logrus.WarnLevel)
	case "error":
		logger.SetLevel(logrus.ErrorLevel)
	case "fatal":
		logger.SetLevel(logrus.FatalLevel)
	default:
		logger.SetLevel(logrus.InfoLevel)
	}
	
	// Set output to stdout
	logger.SetOutput(os.Stdout)
	
	environment := os.Getenv("ENV")
	if environment == "" {
		environment = "development"
	}
	
	version := os.Getenv("SERVICE_VERSION")
	if version == "" {
		version = "1.0.0"
	}
	
	return &StructuredLogger{
		logger:      logger,
		service:     service,
		component:   component,
		environment: environment,
		version:     version,
		fields:      make(map[string]interface{}),
	}
}

// Debug logs a debug message
func (l *StructuredLogger) Debug(ctx context.Context, message string, fields map[string]interface{}) {
	l.log(ctx, LogLevelDebug, message, fields)
}

// Info logs an info message
func (l *StructuredLogger) Info(ctx context.Context, message string, fields map[string]interface{}) {
	l.log(ctx, LogLevelInfo, message, fields)
}

// Warn logs a warning message
func (l *StructuredLogger) Warn(ctx context.Context, message string, fields map[string]interface{}) {
	l.log(ctx, LogLevelWarn, message, fields)
}

// Error logs an error message
func (l *StructuredLogger) Error(ctx context.Context, message string, fields map[string]interface{}) {
	l.log(ctx, LogLevelError, message, fields)
}

// Fatal logs a fatal message and exits
func (l *StructuredLogger) Fatal(ctx context.Context, message string, fields map[string]interface{}) {
	l.log(ctx, LogLevelFatal, message, fields)
	os.Exit(1)
}

// log is the internal logging method
func (l *StructuredLogger) log(ctx context.Context, level LogLevel, message string, fields map[string]interface{}) {
	entry := l.logger.WithFields(logrus.Fields{
		"service":     l.service,
		"component":   l.component,
		"environment": l.environment,
		"version":     l.version,
	})
	
	// Add context fields
	if ctx != nil {
		if requestID := getRequestIDFromContext(ctx); requestID != "" {
			entry = entry.WithField("request_id", requestID)
		}
		if userID := getUserIDFromContext(ctx); userID != "" {
			entry = entry.WithField("user_id", userID)
		}
		if correlationID := getCorrelationIDFromContext(ctx); correlationID != "" {
			entry = entry.WithField("correlation_id", correlationID)
		}
	}
	
	// Add custom fields
	for k, v := range l.fields {
		entry = entry.WithField(k, v)
	}
	
	// Add provided fields
	for k, v := range fields {
		entry = entry.WithField(k, v)
	}
	
	// Add stack trace for errors
	if level == LogLevelError || level == LogLevelFatal {
		entry = entry.WithField("stack_trace", getStackTrace())
	}
	
	// Log the message
	switch level {
	case LogLevelDebug:
		entry.Debug(message)
	case LogLevelInfo:
		entry.Info(message)
	case LogLevelWarn:
		entry.Warn(message)
	case LogLevelError:
		entry.Error(message)
	case LogLevelFatal:
		entry.Fatal(message)
	}
}

// LogRequest logs an HTTP request
func (l *StructuredLogger) LogRequest(ctx context.Context, entry RequestLogEntry) {
	entry.Service = l.service
	entry.Environment = l.environment
	
	logData, _ := json.Marshal(entry)
	l.logger.WithField("log_type", "request").Info(string(logData))
}

// LogError logs an error entry
func (l *StructuredLogger) LogError(ctx context.Context, entry ErrorLogEntry) {
	entry.Service = l.service
	entry.Environment = l.environment
	
	if entry.StackTrace == "" {
		entry.StackTrace = getStackTrace()
	}
	
	logData, _ := json.Marshal(entry)
	l.logger.WithField("log_type", "error").Error(string(logData))
}

// LogEvent logs a business event
func (l *StructuredLogger) LogEvent(ctx context.Context, entry EventLogEntry) {
	entry.Service = l.service
	entry.Environment = l.environment
	
	logData, _ := json.Marshal(entry)
	l.logger.WithField("log_type", "event").Info(string(logData))
}

// LogMetric logs a metric entry
func (l *StructuredLogger) LogMetric(ctx context.Context, entry MetricLogEntry) {
	entry.Service = l.service
	entry.Environment = l.environment
	
	logData, _ := json.Marshal(entry)
	l.logger.WithField("log_type", "metric").Info(string(logData))
}

// WithFields returns a new logger with additional fields
func (l *StructuredLogger) WithFields(fields map[string]interface{}) Logger {
	newFields := make(map[string]interface{})
	for k, v := range l.fields {
		newFields[k] = v
	}
	for k, v := range fields {
		newFields[k] = v
	}
	
	return &StructuredLogger{
		logger:      l.logger,
		service:     l.service,
		component:   l.component,
		environment: l.environment,
		version:     l.version,
		fields:      newFields,
	}
}

// WithContext returns a new logger with context
func (l *StructuredLogger) WithContext(ctx context.Context) Logger {
	fields := make(map[string]interface{})
	for k, v := range l.fields {
		fields[k] = v
	}
	
	if ctx != nil {
		if requestID := getRequestIDFromContext(ctx); requestID != "" {
			fields["request_id"] = requestID
		}
		if userID := getUserIDFromContext(ctx); userID != "" {
			fields["user_id"] = userID
		}
		if correlationID := getCorrelationIDFromContext(ctx); correlationID != "" {
			fields["correlation_id"] = correlationID
		}
	}
	
	return &StructuredLogger{
		logger:      l.logger,
		service:     l.service,
		component:   l.component,
		environment: l.environment,
		version:     l.version,
		fields:      fields,
	}
}

// Helper functions

// getRequestIDFromContext extracts request ID from context
func getRequestIDFromContext(ctx context.Context) string {
	if ginCtx, ok := ctx.(*gin.Context); ok {
		if requestID, exists := ginCtx.Get("request_id"); exists && requestID != nil {
			return fmt.Sprintf("%v", requestID)
		}
	}
	
	if requestID := ctx.Value("request_id"); requestID != nil {
		return fmt.Sprintf("%v", requestID)
	}
	
	return ""
}

// getUserIDFromContext extracts user ID from context
func getUserIDFromContext(ctx context.Context) string {
	if ginCtx, ok := ctx.(*gin.Context); ok {
		if userID, exists := ginCtx.Get("user_id"); exists && userID != nil {
			return fmt.Sprintf("%v", userID)
		}
	}
	
	if userID := ctx.Value("user_id"); userID != nil {
		return fmt.Sprintf("%v", userID)
	}
	
	return ""
}

// getCorrelationIDFromContext extracts correlation ID from context
func getCorrelationIDFromContext(ctx context.Context) string {
	if ginCtx, ok := ctx.(*gin.Context); ok {
		if correlationID, exists := ginCtx.Get("correlation_id"); exists && correlationID != nil {
			return fmt.Sprintf("%v", correlationID)
		}
	}
	
	if correlationID := ctx.Value("correlation_id"); correlationID != nil {
		return fmt.Sprintf("%v", correlationID)
	}
	
	return ""
}

// getStackTrace returns the current stack trace
func getStackTrace() string {
	const depth = 32
	var pcs [depth]uintptr
	n := runtime.Callers(3, pcs[:])
	frames := runtime.CallersFrames(pcs[:n])
	
	var stackTrace strings.Builder
	for {
		frame, more := frames.Next()
		if !more {
			break
		}
		
		stackTrace.WriteString(fmt.Sprintf("%s:%d %s\n", frame.File, frame.Line, frame.Function))
	}
	
	return stackTrace.String()
}

// generateRequestID generates a new request ID
func GenerateRequestID() string {
	return uuid.New().String()
}

// generateCorrelationID generates a new correlation ID
func GenerateCorrelationID() string {
	return uuid.New().String()
}