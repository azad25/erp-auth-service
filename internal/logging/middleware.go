package logging

import (
	"bytes"
	"context"
	"io"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
)

// LoggingMiddleware provides request logging functionality
type LoggingMiddleware struct {
	logger Logger
}

// NewLoggingMiddleware creates a new logging middleware
func NewLoggingMiddleware(logger Logger) *LoggingMiddleware {
	return &LoggingMiddleware{
		logger: logger,
	}
}

// RequestLogger returns a Gin middleware that logs HTTP requests
func (m *LoggingMiddleware) RequestLogger() gin.HandlerFunc {
	return func(c *gin.Context) {
		// Generate request ID if not present
		requestID := c.GetHeader("X-Request-ID")
		if requestID == "" {
			requestID = GenerateRequestID()
		}
		c.Set("request_id", requestID)
		c.Header("X-Request-ID", requestID)
		
		// Generate correlation ID if not present
		correlationID := c.GetHeader("X-Correlation-ID")
		if correlationID == "" {
			correlationID = GenerateCorrelationID()
		}
		c.Set("correlation_id", correlationID)
		c.Header("X-Correlation-ID", correlationID)
		
		// Capture request body for logging (if needed)
		var requestBody []byte
		if c.Request.Body != nil {
			requestBody, _ = io.ReadAll(c.Request.Body)
			c.Request.Body = io.NopCloser(bytes.NewBuffer(requestBody))
		}
		
		// Create response writer wrapper to capture response
		writer := &responseWriter{
			ResponseWriter: c.Writer,
			body:          &bytes.Buffer{},
		}
		c.Writer = writer
		
		// Record start time
		start := time.Now()
		
		// Process request
		c.Next()
		
		// Calculate duration
		duration := time.Since(start)
		
		// Get user ID from context (if authenticated)
		userID := ""
		if uid, exists := c.Get("user_id"); exists && uid != nil {
			userID = uid.(string)
		}
		
		// Create request log entry
		entry := RequestLogEntry{
			Timestamp:    start,
			RequestID:    requestID,
			UserID:       userID,
			Method:       c.Request.Method,
			Path:         c.Request.URL.Path,
			Query:        c.Request.URL.RawQuery,
			StatusCode:   c.Writer.Status(),
			Duration:     duration,
			UserAgent:    c.Request.UserAgent(),
			RemoteIP:     c.ClientIP(),
			RequestSize:  int64(len(requestBody)),
			ResponseSize: int64(writer.body.Len()),
			Headers:      extractHeaders(c.Request.Header),
		}
		
		// Log the request
		m.logger.LogRequest(c.Request.Context(), entry)
		
		// Log additional info for errors
		if c.Writer.Status() >= 400 {
			errorEntry := ErrorLogEntry{
				Timestamp: time.Now(),
				RequestID: requestID,
				UserID:    userID,
				Level:     LogLevelError,
				Message:   "HTTP Error Response",
				Context: map[string]interface{}{
					"method":        c.Request.Method,
					"path":          c.Request.URL.Path,
					"status_code":   c.Writer.Status(),
					"response_body": writer.body.String(),
				},
				Component: "http",
			}
			
			m.logger.LogError(c.Request.Context(), errorEntry)
		}
	}
}

// ErrorLogger returns a Gin middleware that logs errors
func (m *LoggingMiddleware) ErrorLogger() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Next()
		
		// Check for errors
		if len(c.Errors) > 0 {
			requestID := ""
			if rid, exists := c.Get("request_id"); exists && rid != nil {
				requestID = rid.(string)
			}
			
			userID := ""
			if uid, exists := c.Get("user_id"); exists && uid != nil {
				userID = uid.(string)
			}
			
			for _, err := range c.Errors {
				errorEntry := ErrorLogEntry{
					Timestamp: time.Now(),
					RequestID: requestID,
					UserID:    userID,
					Level:     LogLevelError,
					Message:   "Gin Error",
					Error:     err.Error(),
					Context: map[string]interface{}{
						"error_type": err.Type,
						"method":     c.Request.Method,
						"path":       c.Request.URL.Path,
					},
					Component: "gin",
				}
				
				m.logger.LogError(c.Request.Context(), errorEntry)
			}
		}
	}
}

// PanicRecovery returns a Gin middleware that recovers from panics and logs them
func (m *LoggingMiddleware) PanicRecovery() gin.HandlerFunc {
	return gin.CustomRecovery(func(c *gin.Context, recovered interface{}) {
		requestID := ""
		if rid, exists := c.Get("request_id"); exists && rid != nil {
			requestID = rid.(string)
		}
		
		userID := ""
		if uid, exists := c.Get("user_id"); exists && uid != nil {
			userID = uid.(string)
		}
		
		errorEntry := ErrorLogEntry{
			Timestamp: time.Now(),
			RequestID: requestID,
			UserID:    userID,
			Level:     LogLevelFatal,
			Message:   "Panic Recovered",
			Error:     recovered.(string),
			Context: map[string]interface{}{
				"method": c.Request.Method,
				"path":   c.Request.URL.Path,
			},
			Component: "panic_recovery",
		}
		
		m.logger.LogError(c.Request.Context(), errorEntry)
		
		c.AbortWithStatus(500)
	})
}

// responseWriter wraps gin.ResponseWriter to capture response body
type responseWriter struct {
	gin.ResponseWriter
	body *bytes.Buffer
}

func (w *responseWriter) Write(b []byte) (int, error) {
	w.body.Write(b)
	return w.ResponseWriter.Write(b)
}

// extractHeaders extracts relevant headers for logging
func extractHeaders(headers map[string][]string) map[string]string {
	result := make(map[string]string)
	
	// Only log specific headers for security
	relevantHeaders := []string{
		"Content-Type",
		"Accept",
		"Authorization", // Will be masked
		"X-Forwarded-For",
		"X-Real-IP",
		"User-Agent",
	}
	
	for _, header := range relevantHeaders {
		if values, exists := headers[header]; exists && len(values) > 0 {
			if header == "Authorization" {
				// Mask authorization header
				result[header] = "Bearer ***"
			} else {
				result[header] = values[0]
			}
		}
	}
	
	return result
}

// ContextWithLogger adds logger to context
func ContextWithLogger(ctx context.Context, logger Logger) context.Context {
	return context.WithValue(ctx, "logger", logger)
}

// LoggerFromContext extracts logger from context
func LoggerFromContext(ctx context.Context) Logger {
	if logger, ok := ctx.Value("logger").(Logger); ok {
		return logger
	}
	return NewStructuredLogger("unknown", "unknown")
}