package errors

import (
	"context"
	"fmt"
	"math"
	"math/rand"
	"time"

	"go.uber.org/zap"
)

// RetryConfig holds configuration for retry mechanisms
type RetryConfig struct {
	// MaxAttempts is the maximum number of retry attempts
	MaxAttempts int
	
	// InitialDelay is the initial delay before the first retry
	InitialDelay time.Duration
	
	// MaxDelay is the maximum delay between retries
	MaxDelay time.Duration
	
	// Multiplier is the multiplier for exponential backoff
	Multiplier float64
	
	// Jitter adds randomness to the delay to avoid thundering herd
	Jitter bool
	
	// RetryableErrors is a function that determines if an error is retryable
	RetryableErrors func(error) bool
	
	// OnRetry is called before each retry attempt
	OnRetry func(attempt int, err error, delay time.Duration)
}

// RetryManager manages retry operations with exponential backoff
type RetryManager struct {
	config *RetryConfig
	logger *zap.Logger
	rand   *rand.Rand
}

// NewRetryManager creates a new retry manager
func NewRetryManager(config *RetryConfig, logger *zap.Logger) *RetryManager {
	if config == nil {
		config = GetDefaultRetryConfig()
	}
	
	// Set default values if not provided
	if config.MaxAttempts <= 0 {
		config.MaxAttempts = 3
	}
	if config.InitialDelay <= 0 {
		config.InitialDelay = 100 * time.Millisecond
	}
	if config.MaxDelay <= 0 {
		config.MaxDelay = 30 * time.Second
	}
	if config.Multiplier <= 0 {
		config.Multiplier = 2.0
	}
	if config.RetryableErrors == nil {
		config.RetryableErrors = DefaultRetryableErrors
	}
	
	return &RetryManager{
		config: config,
		logger: logger,
		rand:   rand.New(rand.NewSource(time.Now().UnixNano())),
	}
}

// Execute executes a function with retry logic
func (rm *RetryManager) Execute(fn func() error) error {
	return rm.ExecuteWithContext(context.Background(), func(ctx context.Context) error {
		return fn()
	})
}

// ExecuteWithContext executes a function with retry logic and context support
func (rm *RetryManager) ExecuteWithContext(ctx context.Context, fn func(context.Context) error) error {
	var lastErr error
	
	for attempt := 1; attempt <= rm.config.MaxAttempts; attempt++ {
		// Check context cancellation
		select {
		case <-ctx.Done():
			return NewError(ErrorCodeCancelled, "Context cancelled during retry").
				WithCause(ctx.Err()).
				WithCategory(CategoryInternal).
				WithSeverity(SeverityInfo).
				WithRetryable(false).
				WithMetadata("attempt", attempt).
				WithMetadata("max_attempts", rm.config.MaxAttempts).
				Build()
		default:
		}
		
		// Execute the function
		err := fn(ctx)
		if err == nil {
			// Success
			if attempt > 1 {
				rm.logger.Info("Operation succeeded after retry",
					zap.Int("attempt", attempt),
					zap.Int("max_attempts", rm.config.MaxAttempts))
			}
			return nil
		}
		
		lastErr = err
		
		// Check if error is retryable
		if !rm.config.RetryableErrors(err) {
			rm.logger.Debug("Error is not retryable, stopping retry",
				zap.Error(err),
				zap.Int("attempt", attempt))
			return err
		}
		
		// Don't retry on the last attempt
		if attempt == rm.config.MaxAttempts {
			break
		}
		
		// Calculate delay with exponential backoff
		delay := rm.calculateDelay(attempt)
		
		// Call OnRetry callback if provided
		if rm.config.OnRetry != nil {
			rm.config.OnRetry(attempt, err, delay)
		}
		
		rm.logger.Debug("Retrying operation",
			zap.Error(err),
			zap.Int("attempt", attempt),
			zap.Int("max_attempts", rm.config.MaxAttempts),
			zap.Duration("delay", delay))
		
		// Wait for the delay or context cancellation
		select {
		case <-ctx.Done():
			return NewError(ErrorCodeCancelled, "Context cancelled during retry delay").
				WithCause(ctx.Err()).
				WithCategory(CategoryInternal).
				WithSeverity(SeverityInfo).
				WithRetryable(false).
				WithMetadata("attempt", attempt).
				WithMetadata("max_attempts", rm.config.MaxAttempts).
				Build()
		case <-time.After(delay):
			// Continue to next attempt
		}
	}
	
	// All attempts exhausted
	return NewError(ErrorCodeRetryExhausted, fmt.Sprintf("All %d retry attempts exhausted", rm.config.MaxAttempts)).
		WithCause(lastErr).
		WithCategory(CategoryInternal).
		WithSeverity(SeverityError).
		WithRetryable(false).
		WithMetadata("max_attempts", rm.config.MaxAttempts).
		Build()
}

// ExecuteWithResult executes a function that returns a result with retry logic
func (rm *RetryManager) ExecuteWithResult(fn func() (interface{}, error)) (interface{}, error) {
	return rm.ExecuteWithResultAndContext(context.Background(), func(ctx context.Context) (interface{}, error) {
		return fn()
	})
}

// ExecuteWithResultAndContext executes a function that returns a result with retry logic and context support
func (rm *RetryManager) ExecuteWithResultAndContext(ctx context.Context, fn func(context.Context) (interface{}, error)) (interface{}, error) {
	var lastErr error
	var result interface{}
	
	for attempt := 1; attempt <= rm.config.MaxAttempts; attempt++ {
		// Check context cancellation
		select {
		case <-ctx.Done():
			return nil, NewError(ErrorCodeCancelled, "Context cancelled during retry").
				WithCause(ctx.Err()).
				WithCategory(CategoryInternal).
				WithSeverity(SeverityInfo).
				WithRetryable(false).
				WithMetadata("attempt", attempt).
				WithMetadata("max_attempts", rm.config.MaxAttempts).
				Build()
		default:
		}
		
		// Execute the function
		res, err := fn(ctx)
		if err == nil {
			// Success
			if attempt > 1 {
				rm.logger.Info("Operation succeeded after retry",
					zap.Int("attempt", attempt),
					zap.Int("max_attempts", rm.config.MaxAttempts))
			}
			return res, nil
		}
		
		lastErr = err
		result = res
		
		// Check if error is retryable
		if !rm.config.RetryableErrors(err) {
			rm.logger.Debug("Error is not retryable, stopping retry",
				zap.Error(err),
				zap.Int("attempt", attempt))
			return result, err
		}
		
		// Don't retry on the last attempt
		if attempt == rm.config.MaxAttempts {
			break
		}
		
		// Calculate delay with exponential backoff
		delay := rm.calculateDelay(attempt)
		
		// Call OnRetry callback if provided
		if rm.config.OnRetry != nil {
			rm.config.OnRetry(attempt, err, delay)
		}
		
		rm.logger.Debug("Retrying operation",
			zap.Error(err),
			zap.Int("attempt", attempt),
			zap.Int("max_attempts", rm.config.MaxAttempts),
			zap.Duration("delay", delay))
		
		// Wait for the delay or context cancellation
		select {
		case <-ctx.Done():
			return nil, NewError(ErrorCodeCancelled, "Context cancelled during retry delay").
				WithCause(ctx.Err()).
				WithCategory(CategoryInternal).
				WithSeverity(SeverityInfo).
				WithRetryable(false).
				WithMetadata("attempt", attempt).
				WithMetadata("max_attempts", rm.config.MaxAttempts).
				Build()
		case <-time.After(delay):
			// Continue to next attempt
		}
	}
	
	// All attempts exhausted
	return result, NewError(ErrorCodeRetryExhausted, fmt.Sprintf("All %d retry attempts exhausted", rm.config.MaxAttempts)).
		WithCause(lastErr).
		WithCategory(CategoryInternal).
		WithSeverity(SeverityError).
		WithRetryable(false).
		WithMetadata("max_attempts", rm.config.MaxAttempts).
		Build()
}

// calculateDelay calculates the delay for the given attempt using exponential backoff
func (rm *RetryManager) calculateDelay(attempt int) time.Duration {
	// Calculate exponential backoff delay
	delay := float64(rm.config.InitialDelay) * math.Pow(rm.config.Multiplier, float64(attempt-1))
	
	// Apply maximum delay limit
	if delay > float64(rm.config.MaxDelay) {
		delay = float64(rm.config.MaxDelay)
	}
	
	// Add jitter if enabled
	if rm.config.Jitter {
		// Add random jitter up to 10% of the delay
		jitter := delay * 0.1 * rm.rand.Float64()
		delay += jitter
	}
	
	return time.Duration(delay)
}

// DefaultRetryableErrors determines if an error is retryable by default
func DefaultRetryableErrors(err error) bool {
	if err == nil {
		return false
	}
	
	// Check if it's an AuthError
	if authErr, ok := err.(*AuthError); ok {
		return authErr.IsRetryable()
	}
	
	// Check for common retryable error patterns
	errStr := err.Error()
	retryablePatterns := []string{
		"connection refused",
		"connection reset",
		"timeout",
		"temporary failure",
		"service unavailable",
		"too many requests",
		"rate limit",
		"circuit breaker",
		"deadline exceeded",
		"context deadline exceeded",
	}
	
	for _, pattern := range retryablePatterns {
		if contains(errStr, pattern) {
			return true
		}
	}
	
	return false
}

// GetDefaultRetryConfig returns a default retry configuration
func GetDefaultRetryConfig() *RetryConfig {
	return &RetryConfig{
		MaxAttempts:     3,
		InitialDelay:    100 * time.Millisecond,
		MaxDelay:        30 * time.Second,
		Multiplier:      2.0,
		Jitter:          true,
		RetryableErrors: DefaultRetryableErrors,
		OnRetry: func(attempt int, err error, delay time.Duration) {
			// Default no-op callback
		},
	}
}

// GetDatabaseRetryConfig returns retry configuration optimized for database operations
func GetDatabaseRetryConfig() *RetryConfig {
	return &RetryConfig{
		MaxAttempts:  5,
		InitialDelay: 50 * time.Millisecond,
		MaxDelay:     5 * time.Second,
		Multiplier:   1.5,
		Jitter:       true,
		RetryableErrors: func(err error) bool {
			if err == nil {
				return false
			}
			
			// Database-specific retryable errors
			if authErr, ok := err.(*AuthError); ok {
				return authErr.Code == ErrorCodeDatabaseError ||
					authErr.Code == ErrorCodeUnavailable ||
					authErr.Code == ErrorCodeDeadlineExceeded
			}
			
			errStr := err.Error()
			dbRetryablePatterns := []string{
				"connection refused",
				"connection reset",
				"timeout",
				"deadlock",
				"lock wait timeout",
				"connection lost",
				"server has gone away",
			}
			
			for _, pattern := range dbRetryablePatterns {
				if contains(errStr, pattern) {
					return true
				}
			}
			
			return false
		},
	}
}

// GetCacheRetryConfig returns retry configuration optimized for cache operations
func GetCacheRetryConfig() *RetryConfig {
	return &RetryConfig{
		MaxAttempts:  3,
		InitialDelay: 25 * time.Millisecond,
		MaxDelay:     1 * time.Second,
		Multiplier:   2.0,
		Jitter:       true,
		RetryableErrors: func(err error) bool {
			if err == nil {
				return false
			}
			
			// Cache-specific retryable errors
			if authErr, ok := err.(*AuthError); ok {
				return authErr.Code == ErrorCodeCacheError ||
					authErr.Code == ErrorCodeUnavailable ||
					authErr.Code == ErrorCodeDeadlineExceeded
			}
			
			errStr := err.Error()
			cacheRetryablePatterns := []string{
				"connection refused",
				"connection reset",
				"timeout",
				"redis: nil",
				"connection lost",
				"LOADING",
				"READONLY",
			}
			
			for _, pattern := range cacheRetryablePatterns {
				if contains(errStr, pattern) {
					return true
				}
			}
			
			return false
		},
	}
}

// GetEventRetryConfig returns retry configuration optimized for event publishing
func GetEventRetryConfig() *RetryConfig {
	return &RetryConfig{
		MaxAttempts:  5,
		InitialDelay: 200 * time.Millisecond,
		MaxDelay:     10 * time.Second,
		Multiplier:   2.0,
		Jitter:       true,
		RetryableErrors: func(err error) bool {
			if err == nil {
				return false
			}
			
			// Event publishing specific retryable errors
			if authErr, ok := err.(*AuthError); ok {
				return authErr.Code == ErrorCodeEventPublishError ||
					authErr.Code == ErrorCodeUnavailable ||
					authErr.Code == ErrorCodeDeadlineExceeded
			}
			
			errStr := err.Error()
			eventRetryablePatterns := []string{
				"connection refused",
				"broker not available",
				"timeout",
				"network error",
				"leader not available",
				"not enough replicas",
			}
			
			for _, pattern := range eventRetryablePatterns {
				if contains(errStr, pattern) {
					return true
				}
			}
			
			return false
		},
	}
}

// RetryableOperation represents an operation that can be retried
type RetryableOperation struct {
	Name        string
	Operation   func(context.Context) (interface{}, error)
	RetryConfig *RetryConfig
}

// BatchRetryManager manages multiple retryable operations
type BatchRetryManager struct {
	retryManager *RetryManager
	logger       *zap.Logger
}

// NewBatchRetryManager creates a new batch retry manager
func NewBatchRetryManager(config *RetryConfig, logger *zap.Logger) *BatchRetryManager {
	return &BatchRetryManager{
		retryManager: NewRetryManager(config, logger),
		logger:       logger,
	}
}

// ExecuteBatch executes multiple operations with retry logic
func (brm *BatchRetryManager) ExecuteBatch(ctx context.Context, operations []RetryableOperation) map[string]error {
	results := make(map[string]error)
	
	for _, op := range operations {
		// Use operation-specific retry config if provided
		retryManager := brm.retryManager
		if op.RetryConfig != nil {
			retryManager = NewRetryManager(op.RetryConfig, brm.logger)
		}
		
		_, err := retryManager.ExecuteWithResultAndContext(ctx, op.Operation)
		results[op.Name] = err
		
		if err != nil {
			brm.logger.Error("Batch operation failed",
				zap.String("operation", op.Name),
				zap.Error(err))
		}
	}
	
	return results
}