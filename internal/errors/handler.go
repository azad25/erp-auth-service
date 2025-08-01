package errors

import (
	"context"
	"fmt"
	"sync"
	"time"

	"go.uber.org/zap"
)

// ErrorHandler provides a comprehensive error handling system
type ErrorHandler struct {
	logger           *ErrorLogger
	circuitBreaker   *CircuitBreakerManager
	retryManager     *RetryManager
	recoveryManager  *RecoveryManager
	config          *ErrorHandlerConfig
	mu              sync.RWMutex
}

// ErrorHandlerConfig holds configuration for the error handler
type ErrorHandlerConfig struct {
	// EnableCircuitBreaker enables circuit breaker functionality
	EnableCircuitBreaker bool
	
	// EnableRetry enables retry functionality
	EnableRetry bool
	
	// EnableRecovery enables recovery strategies
	EnableRecovery bool
	
	// EnableLogging enables structured error logging
	EnableLogging bool
	
	// EnableAlerting enables error alerting
	EnableAlerting bool
	
	// DefaultRetryConfig is the default retry configuration
	DefaultRetryConfig *RetryConfig
	
	// ErrorLoggingConfig is the error logging configuration
	ErrorLoggingConfig *ErrorLoggingConfig
	
	// GlobalErrorHandlers are global error handlers that process all errors
	GlobalErrorHandlers []func(context.Context, *AuthError)
}

// NewErrorHandler creates a new comprehensive error handler
func NewErrorHandler(logger *zap.Logger, config *ErrorHandlerConfig) *ErrorHandler {
	if config == nil {
		config = GetDefaultErrorHandlerConfig()
	}
	
	handler := &ErrorHandler{
		config: config,
	}
	
	// Initialize error logger if enabled
	if config.EnableLogging {
		handler.logger = NewErrorLogger(logger, config.ErrorLoggingConfig)
	}
	
	// Initialize circuit breaker manager if enabled
	if config.EnableCircuitBreaker {
		handler.circuitBreaker = NewCircuitBreakerManager(logger)
		
		// Register default circuit breakers
		handler.circuitBreaker.RegisterBreaker("database", GetDefaultDatabaseConfig())
		handler.circuitBreaker.RegisterBreaker("redis", GetDefaultRedisConfig())
		handler.circuitBreaker.RegisterBreaker("kafka", GetDefaultKafkaConfig())
	}
	
	// Initialize retry manager if enabled
	if config.EnableRetry {
		handler.retryManager = NewRetryManager(config.DefaultRetryConfig, logger)
	}
	
	// Initialize recovery manager if enabled
	if config.EnableRecovery {
		handler.recoveryManager = NewRecoveryManager(logger)
		
		// Register default recovery strategies
		handler.registerDefaultRecoveryStrategies()
	}
	
	return handler
}

// HandleError processes an error through the comprehensive error handling system
func (eh *ErrorHandler) HandleError(ctx context.Context, err error) *AuthError {
	if err == nil {
		return nil
	}
	
	// Convert to AuthError if not already
	authErr := GetAuthError(err)
	
	// Extract context information
	authErr = ErrorFromContext(ctx, authErr)
	
	// Process through global error handlers
	eh.processGlobalHandlers(ctx, authErr)
	
	// Log the error if logging is enabled
	if eh.config.EnableLogging && eh.logger != nil {
		eh.logger.LogError(ctx, authErr)
	}
	
	return authErr
}

// HandleErrorWithRecovery processes an error and attempts recovery
func (eh *ErrorHandler) HandleErrorWithRecovery(ctx context.Context, serviceName string, err error) *AuthError {
	if err == nil {
		return nil
	}
	
	authErr := eh.HandleError(ctx, err)
	
	// Attempt recovery if enabled and configured
	if eh.config.EnableRecovery && eh.recoveryManager != nil {
		// Check if recovery strategy is configured for this service
		if state, recoveryErr := eh.recoveryManager.GetRecoveryState(serviceName); recoveryErr == nil {
			// Update recovery state with the error
			eh.recoveryManager.updateFailureState(state, authErr)
		}
	}
	
	return authErr
}

// ExecuteWithErrorHandling executes an operation with comprehensive error handling
func (eh *ErrorHandler) ExecuteWithErrorHandling(ctx context.Context, serviceName string, operation func(ctx context.Context) (interface{}, error)) (interface{}, error) {
	// Use recovery manager if enabled
	if eh.config.EnableRecovery && eh.recoveryManager != nil {
		result, err := eh.recoveryManager.ExecuteWithRecovery(ctx, serviceName, operation)
		if err != nil {
			authErr := eh.HandleError(ctx, err)
			return result, authErr
		}
		return result, nil
	}
	
	// Use circuit breaker if enabled
	if eh.config.EnableCircuitBreaker && eh.circuitBreaker != nil {
		result, err := eh.circuitBreaker.ExecuteWithContext(ctx, serviceName, operation)
		if err != nil {
			authErr := eh.HandleError(ctx, err)
			return result, authErr
		}
		return result, nil
	}
	
	// Use retry manager if enabled
	if eh.config.EnableRetry && eh.retryManager != nil {
		result, err := eh.retryManager.ExecuteWithResultAndContext(ctx, operation)
		if err != nil {
			authErr := eh.HandleError(ctx, err)
			return result, authErr
		}
		return result, nil
	}
	
	// Execute directly
	result, err := operation(ctx)
	if err != nil {
		authErr := eh.HandleError(ctx, err)
		return result, authErr
	}
	
	return result, nil
}

// ExecuteWithRetry executes an operation with retry logic
func (eh *ErrorHandler) ExecuteWithRetry(ctx context.Context, operation func(ctx context.Context) (interface{}, error)) (interface{}, error) {
	if !eh.config.EnableRetry || eh.retryManager == nil {
		result, err := operation(ctx)
		if err != nil {
			authErr := eh.HandleError(ctx, err)
			return result, authErr
		}
		return result, nil
	}
	
	result, err := eh.retryManager.ExecuteWithResultAndContext(ctx, operation)
	if err != nil {
		authErr := eh.HandleError(ctx, err)
		return result, authErr
	}
	
	return result, nil
}

// ExecuteWithCircuitBreaker executes an operation with circuit breaker protection
func (eh *ErrorHandler) ExecuteWithCircuitBreaker(ctx context.Context, serviceName string, operation func(ctx context.Context) (interface{}, error)) (interface{}, error) {
	if !eh.config.EnableCircuitBreaker || eh.circuitBreaker == nil {
		result, err := operation(ctx)
		if err != nil {
			authErr := eh.HandleError(ctx, err)
			return result, authErr
		}
		return result, nil
	}
	
	result, err := eh.circuitBreaker.ExecuteWithContext(ctx, serviceName, operation)
	if err != nil {
		authErr := eh.HandleError(ctx, err)
		return result, authErr
	}
	
	return result, nil
}

// RegisterCircuitBreaker registers a circuit breaker for a service
func (eh *ErrorHandler) RegisterCircuitBreaker(serviceName string, config *CircuitBreakerConfig) error {
	if !eh.config.EnableCircuitBreaker || eh.circuitBreaker == nil {
		return NewError(ErrorCodeFailedPrecondition, "Circuit breaker is not enabled").
			WithCategory(CategoryInternal).
			WithSeverity(SeverityError).
			WithRetryable(false).
			Build()
	}
	
	eh.circuitBreaker.RegisterBreaker(serviceName, config)
	return nil
}

// RegisterRecoveryStrategy registers a recovery strategy for a service
func (eh *ErrorHandler) RegisterRecoveryStrategy(serviceName string, config *RecoveryConfig) error {
	if !eh.config.EnableRecovery || eh.recoveryManager == nil {
		return NewError(ErrorCodeFailedPrecondition, "Recovery manager is not enabled").
			WithCategory(CategoryInternal).
			WithSeverity(SeverityError).
			WithRetryable(false).
			Build()
	}
	
	eh.recoveryManager.RegisterRecoveryStrategy(serviceName, config)
	return nil
}

// AddGlobalErrorHandler adds a global error handler
func (eh *ErrorHandler) AddGlobalErrorHandler(handler func(context.Context, *AuthError)) {
	eh.mu.Lock()
	defer eh.mu.Unlock()
	
	if eh.config.GlobalErrorHandlers == nil {
		eh.config.GlobalErrorHandlers = make([]func(context.Context, *AuthError), 0)
	}
	
	eh.config.GlobalErrorHandlers = append(eh.config.GlobalErrorHandlers, handler)
}

// GetErrorMetrics returns current error metrics
func (eh *ErrorHandler) GetErrorMetrics() *ErrorMetrics {
	if eh.logger != nil {
		return eh.logger.GetMetrics()
	}
	return nil
}

// GetCircuitBreakerStates returns the states of all circuit breakers
func (eh *ErrorHandler) GetCircuitBreakerStates() map[string]CircuitBreakerState {
	if eh.circuitBreaker != nil {
		return eh.circuitBreaker.GetAllStates()
	}
	return nil
}

// GetRecoveryStates returns the recovery states of all services
func (eh *ErrorHandler) GetRecoveryStates() map[string]*RecoveryState {
	if eh.recoveryManager != nil {
		return eh.recoveryManager.GetAllRecoveryStates()
	}
	return nil
}

// HealthCheck performs a comprehensive health check
func (eh *ErrorHandler) HealthCheck(ctx context.Context) map[string]interface{} {
	health := make(map[string]interface{})
	
	// Circuit breaker health
	if eh.circuitBreaker != nil {
		health["circuit_breakers"] = eh.circuitBreaker.GetAllStates()
	}
	
	// Recovery manager health
	if eh.recoveryManager != nil {
		health["recovery_states"] = eh.recoveryManager.HealthCheck(ctx)
	}
	
	// Error metrics
	if eh.logger != nil {
		metrics := eh.logger.GetMetrics()
		if metrics != nil {
			health["error_metrics"] = map[string]interface{}{
				"total_errors":     metrics.TotalErrors,
				"error_counts":     metrics.ErrorCounts,
				"severity_counts":  metrics.SeverityCounts,
				"category_counts":  metrics.CategoryCounts,
			}
		}
	}
	
	health["timestamp"] = time.Now()
	health["status"] = "healthy"
	
	return health
}

// processGlobalHandlers processes the error through all global handlers
func (eh *ErrorHandler) processGlobalHandlers(ctx context.Context, authErr *AuthError) {
	eh.mu.RLock()
	handlers := eh.config.GlobalErrorHandlers
	eh.mu.RUnlock()
	
	for _, handler := range handlers {
		// Execute handler in a goroutine to prevent blocking
		go func(h func(context.Context, *AuthError)) {
			defer func() {
				if r := recover(); r != nil {
					// Log panic in global handler but don't propagate
					if eh.logger != nil {
						panicErr := NewError(ErrorCodeInternal, fmt.Sprintf("Panic in global error handler: %v", r)).
							WithCategory(CategoryInternal).
							WithSeverity(SeverityCritical).
							WithRetryable(false).
							Build()
						eh.logger.LogError(ctx, panicErr)
					}
				}
			}()
			h(ctx, authErr)
		}(handler)
	}
}

// registerDefaultRecoveryStrategies registers default recovery strategies
func (eh *ErrorHandler) registerDefaultRecoveryStrategies() {
	// Database recovery strategy
	eh.recoveryManager.RegisterRecoveryStrategy("database", &RecoveryConfig{
		Strategy:             StrategyRetryWithBackoff,
		RetryConfig:          GetDatabaseRetryConfig(),
		CircuitBreakerConfig: GetDefaultDatabaseConfig(),
		OnRecovery: func(strategy RecoveryStrategy, err error, recovered bool) {
			if eh.logger != nil {
				if recovered {
					eh.logger.LogError(context.Background(), NewError(ErrorCodeOK, "Database recovery successful").
						WithCategory(CategoryDatabase).
						WithSeverity(SeverityInfo).
						WithRetryable(false).
						WithMetadata("strategy", string(strategy)).
						Build())
				} else {
					eh.logger.LogError(context.Background(), NewError(ErrorCodeDatabaseError, "Database recovery failed").
						WithCause(err).
						WithCategory(CategoryDatabase).
						WithSeverity(SeverityError).
						WithRetryable(true).
						WithMetadata("strategy", string(strategy)).
						Build())
				}
			}
		},
	})
	
	// Cache recovery strategy
	eh.recoveryManager.RegisterRecoveryStrategy("cache", &RecoveryConfig{
		Strategy:    StrategyGracefulDegradation,
		RetryConfig: GetCacheRetryConfig(),
		GracefulDegradationConfig: &GracefulDegradationConfig{
			ReducedFunctionality: map[string]bool{
				"read_through":  true,
				"write_through": false,
			},
			MaxDegradationTime: 10 * time.Minute,
		},
		OnRecovery: func(strategy RecoveryStrategy, err error, recovered bool) {
			if eh.logger != nil {
				if recovered {
					eh.logger.LogError(context.Background(), NewError(ErrorCodeOK, "Cache recovery successful").
						WithCategory(CategoryCache).
						WithSeverity(SeverityInfo).
						WithRetryable(false).
						WithMetadata("strategy", string(strategy)).
						Build())
				} else {
					eh.logger.LogError(context.Background(), NewError(ErrorCodeCacheError, "Cache recovery failed").
						WithCause(err).
						WithCategory(CategoryCache).
						WithSeverity(SeverityWarning).
						WithRetryable(true).
						WithMetadata("strategy", string(strategy)).
						Build())
				}
			}
		},
	})
	
	// Event publishing recovery strategy
	eh.recoveryManager.RegisterRecoveryStrategy("events", &RecoveryConfig{
		Strategy:    StrategyRetryWithBackoff,
		RetryConfig: GetEventRetryConfig(),
		FallbackFunction: func(ctx context.Context, err error) (interface{}, error) {
			// Fallback to local event storage
			return nil, NewError(ErrorCodeOK, "Event stored locally for later retry").
				WithCategory(CategoryExternal).
				WithSeverity(SeverityInfo).
				WithRetryable(false).
				Build()
		},
		OnRecovery: func(strategy RecoveryStrategy, err error, recovered bool) {
			if eh.logger != nil {
				if recovered {
					eh.logger.LogError(context.Background(), NewError(ErrorCodeOK, "Event publishing recovery successful").
						WithCategory(CategoryExternal).
						WithSeverity(SeverityInfo).
						WithRetryable(false).
						WithMetadata("strategy", string(strategy)).
						Build())
				} else {
					eh.logger.LogError(context.Background(), NewError(ErrorCodeEventPublishError, "Event publishing recovery failed").
						WithCause(err).
						WithCategory(CategoryExternal).
						WithSeverity(SeverityWarning).
						WithRetryable(true).
						WithMetadata("strategy", string(strategy)).
						Build())
				}
			}
		},
	})
}

// GetDefaultErrorHandlerConfig returns default error handler configuration
func GetDefaultErrorHandlerConfig() *ErrorHandlerConfig {
	return &ErrorHandlerConfig{
		EnableCircuitBreaker: true,
		EnableRetry:          true,
		EnableRecovery:       true,
		EnableLogging:        true,
		EnableAlerting:       true,
		DefaultRetryConfig:   GetDefaultRetryConfig(),
		ErrorLoggingConfig:   GetDefaultErrorLoggingConfig(),
		GlobalErrorHandlers:  make([]func(context.Context, *AuthError), 0),
	}
}

// ErrorHandlerMiddleware creates a middleware function for error handling
func (eh *ErrorHandler) ErrorHandlerMiddleware() func(next func(ctx context.Context) (interface{}, error)) func(ctx context.Context) (interface{}, error) {
	return func(next func(ctx context.Context) (interface{}, error)) func(ctx context.Context) (interface{}, error) {
		return func(ctx context.Context) (interface{}, error) {
			result, err := next(ctx)
			if err != nil {
				authErr := eh.HandleError(ctx, err)
				return result, authErr
			}
			return result, nil
		}
	}
}