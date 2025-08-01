package errors

import (
	"context"
	"errors"
	"testing"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zaptest"
)

func TestAuthError_Creation(t *testing.T) {
	tests := []struct {
		name     string
		code     ErrorCode
		message  string
		expected string
	}{
		{
			name:     "simple error",
			code:     ErrorCodeInvalidCredentials,
			message:  "Invalid credentials",
			expected: "[1000] Invalid credentials",
		},
		{
			name:     "database error",
			code:     ErrorCodeDatabaseError,
			message:  "Database connection failed",
			expected: "[1013] Database connection failed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := NewError(tt.code, tt.message).Build()
			if err.Error() != tt.expected {
				t.Errorf("Expected %s, got %s", tt.expected, err.Error())
			}
		})
	}
}

func TestAuthError_WithCause(t *testing.T) {
	originalErr := errors.New("original error")
	authErr := NewError(ErrorCodeDatabaseError, "Database operation failed").
		WithCause(originalErr).
		Build()

	if authErr.Cause != originalErr {
		t.Errorf("Expected cause to be %v, got %v", originalErr, authErr.Cause)
	}

	if authErr.Unwrap() != originalErr {
		t.Errorf("Expected Unwrap() to return %v, got %v", originalErr, authErr.Unwrap())
	}
}

func TestAuthError_ToGRPCStatus(t *testing.T) {
	authErr := NewError(ErrorCodeInvalidCredentials, "Invalid credentials").
		WithCategory(CategoryAuthentication).
		WithSeverity(SeverityWarning).
		Build()

	status := authErr.ToGRPCStatus()
	if status == nil {
		t.Fatal("Expected gRPC status, got nil")
	}

	if status.Message() != "Invalid credentials" {
		t.Errorf("Expected message 'Invalid credentials', got %s", status.Message())
	}
}

func TestCircuitBreakerManager(t *testing.T) {
	logger := zaptest.NewLogger(t)
	cbm := NewCircuitBreakerManager(logger)

	// Register a circuit breaker
	config := GetDefaultDatabaseConfig()
	cbm.RegisterBreaker("test-service", config)

	// Test successful execution
	result, err := cbm.Execute("test-service", func() (interface{}, error) {
		return "success", nil
	})

	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}

	if result != "success" {
		t.Errorf("Expected 'success', got %v", result)
	}

	// Test error execution
	_, err = cbm.Execute("test-service", func() (interface{}, error) {
		return nil, errors.New("test error")
	})

	if err == nil {
		t.Error("Expected error, got nil")
	}
}

func TestRetryManager(t *testing.T) {
	logger := zaptest.NewLogger(t)
	config := &RetryConfig{
		MaxAttempts:  3,
		InitialDelay: 10 * time.Millisecond,
		MaxDelay:     100 * time.Millisecond,
		Multiplier:   2.0,
		Jitter:       false,
		RetryableErrors: func(err error) bool {
			return err.Error() == "retryable error"
		},
	}

	rm := NewRetryManager(config, logger)

	// Test successful retry
	attempts := 0
	result, err := rm.ExecuteWithResultAndContext(context.Background(), func(ctx context.Context) (interface{}, error) {
		attempts++
		if attempts < 3 {
			return nil, errors.New("retryable error")
		}
		return "success", nil
	})

	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}

	if result != "success" {
		t.Errorf("Expected 'success', got %v", result)
	}

	if attempts != 3 {
		t.Errorf("Expected 3 attempts, got %d", attempts)
	}
}

func TestRecoveryManager(t *testing.T) {
	logger := zaptest.NewLogger(t)
	rm := NewRecoveryManager(logger)

	// Register a recovery strategy
	config := &RecoveryConfig{
		Strategy: StrategyFailFast,
	}
	rm.RegisterRecoveryStrategy("test-service", config)

	// Test execution with recovery
	result, err := rm.ExecuteWithRecovery(context.Background(), "test-service", func(ctx context.Context) (interface{}, error) {
		return "success", nil
	})

	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}

	if result != "success" {
		t.Errorf("Expected 'success', got %v", result)
	}
}

func TestErrorLogger(t *testing.T) {
	logger := zaptest.NewLogger(t)
	config := &ErrorLoggingConfig{
		EnableMetrics:  true,
		EnableAlerting: false,
	}

	el := NewErrorLogger(logger, config)

	// Test error logging
	testErr := NewError(ErrorCodeInvalidCredentials, "Test error").
		WithCategory(CategoryAuthentication).
		WithSeverity(SeverityWarning).
		Build()

	el.LogError(context.Background(), testErr)

	// Check metrics
	metrics := el.GetMetrics()
	if metrics == nil {
		t.Fatal("Expected metrics, got nil")
	}

	if metrics.ErrorCounts[ErrorCodeInvalidCredentials] != 1 {
		t.Errorf("Expected error count 1, got %d", metrics.ErrorCounts[ErrorCodeInvalidCredentials])
	}

	if metrics.SeverityCounts[SeverityWarning] != 1 {
		t.Errorf("Expected severity count 1, got %d", metrics.SeverityCounts[SeverityWarning])
	}
}

func TestErrorHandler_Integration(t *testing.T) {
	logger := zaptest.NewLogger(t)
	config := GetDefaultErrorHandlerConfig()
	config.EnableAlerting = false // Disable alerting for tests

	eh := NewErrorHandler(logger, config)

	// Test error handling
	testErr := errors.New("test error")
	authErr := eh.HandleError(context.Background(), testErr)

	if authErr == nil {
		t.Fatal("Expected AuthError, got nil")
	}

	if authErr.Code != ErrorCodeUnknown {
		t.Errorf("Expected ErrorCodeUnknown, got %d", authErr.Code)
	}

	// Test execution with error handling
	result, err := eh.ExecuteWithErrorHandling(context.Background(), "test-service", func(ctx context.Context) (interface{}, error) {
		return "success", nil
	})

	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}

	if result != "success" {
		t.Errorf("Expected 'success', got %v", result)
	}
}

func TestPredefinedErrors(t *testing.T) {
	tests := []struct {
		name     string
		errorFn  func() *AuthError
		expected ErrorCode
	}{
		{
			name:     "invalid credentials",
			errorFn:  func() *AuthError { return NewInvalidCredentialsError("test@example.com") },
			expected: ErrorCodeInvalidCredentials,
		},
		{
			name:     "token expired",
			errorFn:  func() *AuthError { return NewTokenExpiredError("access") },
			expected: ErrorCodeTokenExpired,
		},
		{
			name:     "user not found",
			errorFn:  func() *AuthError { return NewUserNotFoundError("test@example.com") },
			expected: ErrorCodeUserNotFound,
		},
		{
			name:     "database error",
			errorFn:  func() *AuthError { return NewDatabaseError("SELECT", errors.New("connection failed")) },
			expected: ErrorCodeDatabaseError,
		},
		{
			name:     "cache error",
			errorFn:  func() *AuthError { return NewCacheError("GET", errors.New("redis down")) },
			expected: ErrorCodeCacheError,
		},
		{
			name:     "rate limit error",
			errorFn:  func() *AuthError { return NewRateLimitError(100, time.Minute) },
			expected: ErrorCodeRateLimitExceeded,
		},
		{
			name:     "circuit breaker error",
			errorFn:  func() *AuthError { return NewCircuitBreakerError("database") },
			expected: ErrorCodeCircuitBreakerOpen,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.errorFn()
			if err.Code != tt.expected {
				t.Errorf("Expected error code %d, got %d", tt.expected, err.Code)
			}
		})
	}
}

func TestErrorFromContext(t *testing.T) {
	ctx := context.WithValue(context.Background(), "trace_id", "test-trace-123")
	ctx = context.WithValue(ctx, "request_id", "test-request-456")
	ctx = context.WithValue(ctx, "user_id", "test-user-789")

	originalErr := errors.New("test error")
	authErr := ErrorFromContext(ctx, originalErr)

	if authErr.TraceID != "test-trace-123" {
		t.Errorf("Expected trace ID 'test-trace-123', got %s", authErr.TraceID)
	}

	if authErr.RequestID != "test-request-456" {
		t.Errorf("Expected request ID 'test-request-456', got %s", authErr.RequestID)
	}

	if authErr.UserID != "test-user-789" {
		t.Errorf("Expected user ID 'test-user-789', got %s", authErr.UserID)
	}
}

func BenchmarkErrorCreation(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_ = NewError(ErrorCodeInvalidCredentials, "Invalid credentials").
			WithCategory(CategoryAuthentication).
			WithSeverity(SeverityWarning).
			WithRetryable(false).
			Build()
	}
}

func BenchmarkErrorHandling(b *testing.B) {
	logger := zap.NewNop()
	config := GetDefaultErrorHandlerConfig()
	config.EnableAlerting = false
	eh := NewErrorHandler(logger, config)

	testErr := errors.New("test error")
	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = eh.HandleError(ctx, testErr)
	}
}