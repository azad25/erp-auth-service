package errors

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// ErrorLogger provides structured error logging with alerting capabilities
type ErrorLogger struct {
	logger    *zap.Logger
	alerter   *ErrorAlerter
	metrics   *ErrorMetrics
	config    *ErrorLoggingConfig
	mu        sync.RWMutex
}

// ErrorLoggingConfig holds configuration for error logging
type ErrorLoggingConfig struct {
	// LogLevel defines the minimum log level for errors
	LogLevel zapcore.Level
	
	// EnableStackTrace enables stack trace logging for errors
	EnableStackTrace bool
	
	// EnableMetrics enables error metrics collection
	EnableMetrics bool
	
	// EnableAlerting enables error alerting
	EnableAlerting bool
	
	// SamplingConfig for high-frequency error sampling
	SamplingConfig *SamplingConfig
	
	// StructuredLogging enables structured JSON logging
	StructuredLogging bool
	
	// ErrorBufferSize defines the size of the error buffer for batch processing
	ErrorBufferSize int
	
	// FlushInterval defines how often to flush buffered errors
	FlushInterval time.Duration
}

// SamplingConfig holds configuration for error sampling
type SamplingConfig struct {
	// Initial sampling rate (errors per second before sampling kicks in)
	Initial int
	
	// Thereafter sampling rate (1 in N errors after initial threshold)
	Thereafter int
	
	// Tick interval for sampling window
	Tick time.Duration
}

// ErrorMetrics tracks error metrics
type ErrorMetrics struct {
	ErrorCounts      map[ErrorCode]int64
	ErrorRates       map[ErrorCode]float64
	SeverityCounts   map[ErrorSeverity]int64
	CategoryCounts   map[ErrorCategory]int64
	LastErrorTime    map[ErrorCode]time.Time
	TotalErrors      int64
	mu               sync.RWMutex
}

// ErrorAlerter handles error alerting
type ErrorAlerter struct {
	config    *AlertingConfig
	logger    *zap.Logger
	channels  []AlertChannel
	rules     []AlertRule
	mu        sync.RWMutex
}

// AlertingConfig holds configuration for error alerting
type AlertingConfig struct {
	// Enabled enables/disables alerting
	Enabled bool
	
	// DefaultChannel is the default alert channel
	DefaultChannel string
	
	// AlertThresholds define when to trigger alerts
	AlertThresholds map[ErrorSeverity]AlertThreshold
	
	// RateLimiting prevents alert spam
	RateLimiting *AlertRateLimiting
}

// AlertThreshold defines thresholds for triggering alerts
type AlertThreshold struct {
	// Count threshold (number of errors)
	Count int
	
	// Rate threshold (errors per minute)
	Rate float64
	
	// TimeWindow for threshold evaluation
	TimeWindow time.Duration
}

// AlertRateLimiting prevents alert spam
type AlertRateLimiting struct {
	// MaxAlertsPerMinute limits alerts per minute
	MaxAlertsPerMinute int
	
	// CooldownPeriod prevents duplicate alerts
	CooldownPeriod time.Duration
	
	// LastAlertTime tracks last alert time per error code
	LastAlertTime map[ErrorCode]time.Time
}

// AlertChannel represents an alert delivery channel
type AlertChannel interface {
	SendAlert(ctx context.Context, alert *Alert) error
	GetName() string
}

// AlertRule defines when and how to send alerts
type AlertRule struct {
	Name        string
	Condition   func(*AuthError) bool
	Severity    ErrorSeverity
	Channel     string
	Message     string
	Metadata    map[string]interface{}
}

// Alert represents an error alert
type Alert struct {
	ID          string                 `json:"id"`
	Timestamp   time.Time              `json:"timestamp"`
	Severity    ErrorSeverity          `json:"severity"`
	Title       string                 `json:"title"`
	Message     string                 `json:"message"`
	Error       *AuthError             `json:"error"`
	Metadata    map[string]interface{} `json:"metadata"`
	Channel     string                 `json:"channel"`
}

// NewErrorLogger creates a new error logger
func NewErrorLogger(logger *zap.Logger, config *ErrorLoggingConfig) *ErrorLogger {
	if config == nil {
		config = GetDefaultErrorLoggingConfig()
	}
	
	errorLogger := &ErrorLogger{
		logger:  logger,
		config:  config,
		metrics: NewErrorMetrics(),
	}
	
	// Initialize alerter if enabled
	if config.EnableAlerting {
		alertConfig := &AlertingConfig{
			Enabled:        true,
			DefaultChannel: "log",
			AlertThresholds: map[ErrorSeverity]AlertThreshold{
				SeverityCritical: {Count: 1, Rate: 1.0, TimeWindow: time.Minute},
				SeverityError:    {Count: 5, Rate: 5.0, TimeWindow: time.Minute},
				SeverityWarning:  {Count: 10, Rate: 10.0, TimeWindow: time.Minute},
			},
			RateLimiting: &AlertRateLimiting{
				MaxAlertsPerMinute: 10,
				CooldownPeriod:     5 * time.Minute,
				LastAlertTime:      make(map[ErrorCode]time.Time),
			},
		}
		errorLogger.alerter = NewErrorAlerter(alertConfig, logger)
	}
	
	return errorLogger
}

// LogError logs an error with structured logging and optional alerting
func (el *ErrorLogger) LogError(ctx context.Context, err error) {
	authErr := GetAuthError(err)
	
	// Extract context information
	authErr = ErrorFromContext(ctx, authErr)
	
	// Update metrics if enabled
	if el.config.EnableMetrics {
		el.metrics.RecordError(authErr)
	}
	
	// Log the error
	el.logStructuredError(authErr)
	
	// Send alert if enabled and conditions are met
	if el.config.EnableAlerting && el.alerter != nil {
		el.alerter.ProcessError(ctx, authErr)
	}
}

// LogErrorWithContext logs an error with additional context
func (el *ErrorLogger) LogErrorWithContext(ctx context.Context, err error, fields ...zap.Field) {
	authErr := GetAuthError(err)
	authErr = ErrorFromContext(ctx, authErr)
	
	// Update metrics
	if el.config.EnableMetrics {
		el.metrics.RecordError(authErr)
	}
	
	// Log with additional fields
	el.logStructuredErrorWithFields(authErr, fields...)
	
	// Send alert
	if el.config.EnableAlerting && el.alerter != nil {
		el.alerter.ProcessError(ctx, authErr)
	}
}

// logStructuredError logs an error with structured format
func (el *ErrorLogger) logStructuredError(authErr *AuthError) {
	fields := []zap.Field{
		zap.String("error_code", fmt.Sprintf("%d", authErr.Code)),
		zap.String("error_category", string(authErr.Category)),
		zap.String("error_severity", string(authErr.Severity)),
		zap.Bool("retryable", authErr.Retryable),
		zap.String("trace_id", authErr.TraceID),
		zap.String("request_id", authErr.RequestID),
		zap.String("user_id", authErr.UserID),
		zap.Time("timestamp", authErr.Timestamp),
	}
	
	// Add metadata fields
	if authErr.Metadata != nil {
		for key, value := range authErr.Metadata {
			fields = append(fields, zap.Any(fmt.Sprintf("meta_%s", key), value))
		}
	}
	
	// Add stack trace if enabled and available
	if el.config.EnableStackTrace && len(authErr.Stack) > 0 {
		stackJSON, _ := json.Marshal(authErr.Stack)
		fields = append(fields, zap.String("stack_trace", string(stackJSON)))
	}
	
	// Add underlying error if present
	if authErr.Cause != nil {
		fields = append(fields, zap.Error(authErr.Cause))
	}
	
	// Log based on severity
	switch authErr.Severity {
	case SeverityCritical:
		el.logger.Error(authErr.Message, fields...)
	case SeverityError:
		el.logger.Error(authErr.Message, fields...)
	case SeverityWarning:
		el.logger.Warn(authErr.Message, fields...)
	case SeverityInfo:
		el.logger.Info(authErr.Message, fields...)
	default:
		el.logger.Error(authErr.Message, fields...)
	}
}

// logStructuredErrorWithFields logs an error with additional fields
func (el *ErrorLogger) logStructuredErrorWithFields(authErr *AuthError, additionalFields ...zap.Field) {
	fields := []zap.Field{
		zap.String("error_code", fmt.Sprintf("%d", authErr.Code)),
		zap.String("error_category", string(authErr.Category)),
		zap.String("error_severity", string(authErr.Severity)),
		zap.Bool("retryable", authErr.Retryable),
		zap.String("trace_id", authErr.TraceID),
		zap.String("request_id", authErr.RequestID),
		zap.String("user_id", authErr.UserID),
		zap.Time("timestamp", authErr.Timestamp),
	}
	
	// Add metadata fields
	if authErr.Metadata != nil {
		for key, value := range authErr.Metadata {
			fields = append(fields, zap.Any(fmt.Sprintf("meta_%s", key), value))
		}
	}
	
	// Add additional fields
	fields = append(fields, additionalFields...)
	
	// Add stack trace if enabled
	if el.config.EnableStackTrace && len(authErr.Stack) > 0 {
		stackJSON, _ := json.Marshal(authErr.Stack)
		fields = append(fields, zap.String("stack_trace", string(stackJSON)))
	}
	
	// Add underlying error
	if authErr.Cause != nil {
		fields = append(fields, zap.Error(authErr.Cause))
	}
	
	// Log based on severity
	switch authErr.Severity {
	case SeverityCritical:
		el.logger.Error(authErr.Message, fields...)
	case SeverityError:
		el.logger.Error(authErr.Message, fields...)
	case SeverityWarning:
		el.logger.Warn(authErr.Message, fields...)
	case SeverityInfo:
		el.logger.Info(authErr.Message, fields...)
	default:
		el.logger.Error(authErr.Message, fields...)
	}
}

// GetMetrics returns current error metrics
func (el *ErrorLogger) GetMetrics() *ErrorMetrics {
	if !el.config.EnableMetrics {
		return nil
	}
	return el.metrics.GetSnapshot()
}

// NewErrorMetrics creates a new error metrics instance
func NewErrorMetrics() *ErrorMetrics {
	return &ErrorMetrics{
		ErrorCounts:    make(map[ErrorCode]int64),
		ErrorRates:     make(map[ErrorCode]float64),
		SeverityCounts: make(map[ErrorSeverity]int64),
		CategoryCounts: make(map[ErrorCategory]int64),
		LastErrorTime:  make(map[ErrorCode]time.Time),
	}
}

// RecordError records an error in metrics
func (em *ErrorMetrics) RecordError(authErr *AuthError) {
	em.mu.Lock()
	defer em.mu.Unlock()
	
	em.ErrorCounts[authErr.Code]++
	em.SeverityCounts[authErr.Severity]++
	em.CategoryCounts[authErr.Category]++
	em.LastErrorTime[authErr.Code] = time.Now()
	em.TotalErrors++
}

// GetSnapshot returns a snapshot of current metrics
func (em *ErrorMetrics) GetSnapshot() *ErrorMetrics {
	em.mu.RLock()
	defer em.mu.RUnlock()
	
	snapshot := &ErrorMetrics{
		ErrorCounts:    make(map[ErrorCode]int64),
		ErrorRates:     make(map[ErrorCode]float64),
		SeverityCounts: make(map[ErrorSeverity]int64),
		CategoryCounts: make(map[ErrorCategory]int64),
		LastErrorTime:  make(map[ErrorCode]time.Time),
		TotalErrors:    em.TotalErrors,
	}
	
	// Copy maps
	for k, v := range em.ErrorCounts {
		snapshot.ErrorCounts[k] = v
	}
	for k, v := range em.ErrorRates {
		snapshot.ErrorRates[k] = v
	}
	for k, v := range em.SeverityCounts {
		snapshot.SeverityCounts[k] = v
	}
	for k, v := range em.CategoryCounts {
		snapshot.CategoryCounts[k] = v
	}
	for k, v := range em.LastErrorTime {
		snapshot.LastErrorTime[k] = v
	}
	
	return snapshot
}

// NewErrorAlerter creates a new error alerter
func NewErrorAlerter(config *AlertingConfig, logger *zap.Logger) *ErrorAlerter {
	alerter := &ErrorAlerter{
		config:   config,
		logger:   logger,
		channels: make([]AlertChannel, 0),
		rules:    make([]AlertRule, 0),
	}
	
	// Add default log channel
	alerter.AddChannel(&LogAlertChannel{logger: logger})
	
	// Add default alert rules
	alerter.AddRule(AlertRule{
		Name: "critical_errors",
		Condition: func(err *AuthError) bool {
			return err.Severity == SeverityCritical
		},
		Severity: SeverityCritical,
		Channel:  "log",
		Message:  "Critical error occurred",
	})
	
	alerter.AddRule(AlertRule{
		Name: "authentication_failures",
		Condition: func(err *AuthError) bool {
			return err.Category == CategoryAuthentication && err.Severity == SeverityError
		},
		Severity: SeverityError,
		Channel:  "log",
		Message:  "Authentication failure detected",
	})
	
	return alerter
}

// AddChannel adds an alert channel
func (ea *ErrorAlerter) AddChannel(channel AlertChannel) {
	ea.mu.Lock()
	defer ea.mu.Unlock()
	ea.channels = append(ea.channels, channel)
}

// AddRule adds an alert rule
func (ea *ErrorAlerter) AddRule(rule AlertRule) {
	ea.mu.Lock()
	defer ea.mu.Unlock()
	ea.rules = append(ea.rules, rule)
}

// ProcessError processes an error and sends alerts if conditions are met
func (ea *ErrorAlerter) ProcessError(ctx context.Context, authErr *AuthError) {
	if !ea.config.Enabled {
		return
	}
	
	// Check rate limiting
	if ea.isRateLimited(authErr) {
		return
	}
	
	// Check alert rules
	ea.mu.RLock()
	rules := make([]AlertRule, len(ea.rules))
	copy(rules, ea.rules)
	ea.mu.RUnlock()
	
	for _, rule := range rules {
		if rule.Condition(authErr) {
			alert := &Alert{
				ID:        fmt.Sprintf("%d-%d", time.Now().UnixNano(), authErr.Code),
				Timestamp: time.Now(),
				Severity:  rule.Severity,
				Title:     rule.Name,
				Message:   rule.Message,
				Error:     authErr,
				Channel:   rule.Channel,
				Metadata:  rule.Metadata,
			}
			
			ea.sendAlert(ctx, alert)
			
			// Update rate limiting
			ea.updateRateLimit(authErr)
		}
	}
}

// isRateLimited checks if alerts for this error code are rate limited
func (ea *ErrorAlerter) isRateLimited(authErr *AuthError) bool {
	if ea.config.RateLimiting == nil {
		return false
	}
	
	lastAlert, exists := ea.config.RateLimiting.LastAlertTime[authErr.Code]
	if !exists {
		return false
	}
	
	return time.Since(lastAlert) < ea.config.RateLimiting.CooldownPeriod
}

// updateRateLimit updates the rate limiting state
func (ea *ErrorAlerter) updateRateLimit(authErr *AuthError) {
	if ea.config.RateLimiting != nil {
		ea.config.RateLimiting.LastAlertTime[authErr.Code] = time.Now()
	}
}

// sendAlert sends an alert through the appropriate channel
func (ea *ErrorAlerter) sendAlert(ctx context.Context, alert *Alert) {
	ea.mu.RLock()
	channels := make([]AlertChannel, len(ea.channels))
	copy(channels, ea.channels)
	ea.mu.RUnlock()
	
	// Find the appropriate channel
	var targetChannel AlertChannel
	for _, channel := range channels {
		if channel.GetName() == alert.Channel {
			targetChannel = channel
			break
		}
	}
	
	// Use default channel if not found
	if targetChannel == nil && len(channels) > 0 {
		targetChannel = channels[0]
	}
	
	if targetChannel != nil {
		if err := targetChannel.SendAlert(ctx, alert); err != nil {
			ea.logger.Error("Failed to send alert",
				zap.String("channel", targetChannel.GetName()),
				zap.String("alert_id", alert.ID),
				zap.Error(err))
		}
	}
}

// LogAlertChannel sends alerts to the log
type LogAlertChannel struct {
	logger *zap.Logger
}

// SendAlert sends an alert to the log
func (lac *LogAlertChannel) SendAlert(ctx context.Context, alert *Alert) error {
	lac.logger.Error("ALERT: "+alert.Title,
		zap.String("alert_id", alert.ID),
		zap.String("severity", string(alert.Severity)),
		zap.String("message", alert.Message),
		zap.String("error_code", fmt.Sprintf("%d", alert.Error.Code)),
		zap.String("error_category", string(alert.Error.Category)),
		zap.String("trace_id", alert.Error.TraceID),
		zap.Time("timestamp", alert.Timestamp))
	return nil
}

// GetName returns the channel name
func (lac *LogAlertChannel) GetName() string {
	return "log"
}

// GetDefaultErrorLoggingConfig returns default error logging configuration
func GetDefaultErrorLoggingConfig() *ErrorLoggingConfig {
	return &ErrorLoggingConfig{
		LogLevel:          zapcore.ErrorLevel,
		EnableStackTrace:  true,
		EnableMetrics:     true,
		EnableAlerting:    true,
		StructuredLogging: true,
		ErrorBufferSize:   1000,
		FlushInterval:     30 * time.Second,
		SamplingConfig: &SamplingConfig{
			Initial:    100,
			Thereafter: 100,
			Tick:       time.Second,
		},
	}
}