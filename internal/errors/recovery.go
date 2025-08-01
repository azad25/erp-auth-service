package errors

import (
	"context"
	"fmt"
	"sync"
	"time"

	"go.uber.org/zap"
)

// RecoveryStrategy defines different recovery strategies for system failures
type RecoveryStrategy string

const (
	// StrategyFailFast fails immediately without attempting recovery
	StrategyFailFast RecoveryStrategy = "FAIL_FAST"
	
	// StrategyGracefulDegradation continues with reduced functionality
	StrategyGracefulDegradation RecoveryStrategy = "GRACEFUL_DEGRADATION"
	
	// StrategyFallback uses alternative implementation or cached data
	StrategyFallback RecoveryStrategy = "FALLBACK"
	
	// StrategyRetryWithBackoff retries with exponential backoff
	StrategyRetryWithBackoff RecoveryStrategy = "RETRY_WITH_BACKOFF"
	
	// StrategyCircuitBreaker uses circuit breaker pattern
	StrategyCircuitBreaker RecoveryStrategy = "CIRCUIT_BREAKER"
	
	// StrategyBulkhead isolates failures to prevent cascade
	StrategyBulkhead RecoveryStrategy = "BULKHEAD"
)

// RecoveryConfig holds configuration for error recovery
type RecoveryConfig struct {
	// Strategy defines the recovery strategy to use
	Strategy RecoveryStrategy
	
	// FallbackFunction is called when using fallback strategy
	FallbackFunction func(ctx context.Context, err error) (interface{}, error)
	
	// RetryConfig for retry-based recovery
	RetryConfig *RetryConfig
	
	// CircuitBreakerConfig for circuit breaker recovery
	CircuitBreakerConfig *CircuitBreakerConfig
	
	// GracefulDegradationConfig for graceful degradation
	GracefulDegradationConfig *GracefulDegradationConfig
	
	// BulkheadConfig for bulkhead isolation
	BulkheadConfig *BulkheadConfig
	
	// OnRecovery is called when recovery is attempted
	OnRecovery func(strategy RecoveryStrategy, err error, recovered bool)
}

// GracefulDegradationConfig holds configuration for graceful degradation
type GracefulDegradationConfig struct {
	// ReducedFunctionality defines what functionality to maintain
	ReducedFunctionality map[string]bool
	
	// FallbackData provides default data when primary source fails
	FallbackData interface{}
	
	// MaxDegradationTime limits how long to stay in degraded mode
	MaxDegradationTime time.Duration
}

// BulkheadConfig holds configuration for bulkhead isolation
type BulkheadConfig struct {
	// MaxConcurrentRequests limits concurrent requests to prevent cascade failures
	MaxConcurrentRequests int
	
	// TimeoutPerRequest sets timeout for individual requests
	TimeoutPerRequest time.Duration
	
	// QueueSize defines the size of the request queue
	QueueSize int
}

// RecoveryManager manages error recovery strategies
type RecoveryManager struct {
	strategies map[string]*RecoveryConfig
	logger     *zap.Logger
	mu         sync.RWMutex
	
	// Recovery state tracking
	recoveryStates map[string]*RecoveryState
	
	// Bulkhead semaphores for resource isolation
	bulkheads map[string]chan struct{}
}

// RecoveryState tracks the state of recovery for a service
type RecoveryState struct {
	ServiceName        string
	Strategy          RecoveryStrategy
	LastFailure       time.Time
	FailureCount      int
	RecoveryAttempts  int
	InDegradedMode    bool
	DegradationStart  time.Time
	LastRecoveryTime  time.Time
}

// NewRecoveryManager creates a new recovery manager
func NewRecoveryManager(logger *zap.Logger) *RecoveryManager {
	return &RecoveryManager{
		strategies:     make(map[string]*RecoveryConfig),
		logger:         logger,
		recoveryStates: make(map[string]*RecoveryState),
		bulkheads:      make(map[string]chan struct{}),
	}
}

// RegisterRecoveryStrategy registers a recovery strategy for a service
func (rm *RecoveryManager) RegisterRecoveryStrategy(serviceName string, config *RecoveryConfig) {
	rm.mu.Lock()
	defer rm.mu.Unlock()
	
	rm.strategies[serviceName] = config
	rm.recoveryStates[serviceName] = &RecoveryState{
		ServiceName: serviceName,
		Strategy:    config.Strategy,
	}
	
	// Initialize bulkhead if needed
	if config.Strategy == StrategyBulkhead && config.BulkheadConfig != nil {
		rm.bulkheads[serviceName] = make(chan struct{}, config.BulkheadConfig.MaxConcurrentRequests)
	}
	
	rm.logger.Info("Recovery strategy registered",
		zap.String("service", serviceName),
		zap.String("strategy", string(config.Strategy)))
}

// ExecuteWithRecovery executes an operation with the configured recovery strategy
func (rm *RecoveryManager) ExecuteWithRecovery(ctx context.Context, serviceName string, operation func(ctx context.Context) (interface{}, error)) (interface{}, error) {
	rm.mu.RLock()
	config, exists := rm.strategies[serviceName]
	state := rm.recoveryStates[serviceName]
	rm.mu.RUnlock()
	
	if !exists {
		// No recovery strategy configured, execute directly
		return operation(ctx)
	}
	
	switch config.Strategy {
	case StrategyFailFast:
		return rm.executeFailFast(ctx, serviceName, operation, config, state)
	case StrategyGracefulDegradation:
		return rm.executeGracefulDegradation(ctx, serviceName, operation, config, state)
	case StrategyFallback:
		return rm.executeFallback(ctx, serviceName, operation, config, state)
	case StrategyRetryWithBackoff:
		return rm.executeRetryWithBackoff(ctx, serviceName, operation, config, state)
	case StrategyCircuitBreaker:
		return rm.executeCircuitBreaker(ctx, serviceName, operation, config, state)
	case StrategyBulkhead:
		return rm.executeBulkhead(ctx, serviceName, operation, config, state)
	default:
		return operation(ctx)
	}
}

// executeFailFast executes with fail-fast strategy
func (rm *RecoveryManager) executeFailFast(ctx context.Context, serviceName string, operation func(ctx context.Context) (interface{}, error), config *RecoveryConfig, state *RecoveryState) (interface{}, error) {
	result, err := operation(ctx)
	if err != nil {
		rm.updateFailureState(state, err)
		rm.logger.Error("Operation failed with fail-fast strategy",
			zap.String("service", serviceName),
			zap.Error(err))
	}
	return result, err
}

// executeGracefulDegradation executes with graceful degradation strategy
func (rm *RecoveryManager) executeGracefulDegradation(ctx context.Context, serviceName string, operation func(ctx context.Context) (interface{}, error), config *RecoveryConfig, state *RecoveryState) (interface{}, error) {
	result, err := operation(ctx)
	if err != nil {
		rm.updateFailureState(state, err)
		
		// Check if we should enter degraded mode
		if !state.InDegradedMode {
			state.InDegradedMode = true
			state.DegradationStart = time.Now()
			rm.logger.Warn("Entering graceful degradation mode",
				zap.String("service", serviceName),
				zap.Error(err))
		}
		
		// Check if degradation time limit exceeded
		if config.GracefulDegradationConfig != nil &&
			config.GracefulDegradationConfig.MaxDegradationTime > 0 &&
			time.Since(state.DegradationStart) > config.GracefulDegradationConfig.MaxDegradationTime {
			
			rm.logger.Error("Degradation time limit exceeded, failing",
				zap.String("service", serviceName),
				zap.Duration("degradation_time", time.Since(state.DegradationStart)))
			return nil, NewError(ErrorCodeUnavailable, fmt.Sprintf("Service %s degradation time limit exceeded", serviceName)).
				WithCause(err).
				WithCategory(CategoryInternal).
				WithSeverity(SeverityError).
				WithRetryable(false).
				Build()
		}
		
		// Return fallback data if available
		if config.GracefulDegradationConfig != nil && config.GracefulDegradationConfig.FallbackData != nil {
			rm.logger.Info("Returning fallback data in degraded mode",
				zap.String("service", serviceName))
			return config.GracefulDegradationConfig.FallbackData, nil
		}
		
		return nil, err
	}
	
	// Operation succeeded, exit degraded mode if we were in it
	if state.InDegradedMode {
		state.InDegradedMode = false
		state.LastRecoveryTime = time.Now()
		rm.logger.Info("Exited graceful degradation mode",
			zap.String("service", serviceName),
			zap.Duration("degradation_duration", time.Since(state.DegradationStart)))
	}
	
	return result, nil
}

// executeFallback executes with fallback strategy
func (rm *RecoveryManager) executeFallback(ctx context.Context, serviceName string, operation func(ctx context.Context) (interface{}, error), config *RecoveryConfig, state *RecoveryState) (interface{}, error) {
	result, err := operation(ctx)
	if err != nil {
		rm.updateFailureState(state, err)
		
		// Try fallback function if configured
		if config.FallbackFunction != nil {
			rm.logger.Info("Attempting fallback recovery",
				zap.String("service", serviceName),
				zap.Error(err))
			
			fallbackResult, fallbackErr := config.FallbackFunction(ctx, err)
			if fallbackErr == nil {
				state.RecoveryAttempts++
				state.LastRecoveryTime = time.Now()
				
				if config.OnRecovery != nil {
					config.OnRecovery(StrategyFallback, err, true)
				}
				
				rm.logger.Info("Fallback recovery successful",
					zap.String("service", serviceName))
				return fallbackResult, nil
			}
			
			if config.OnRecovery != nil {
				config.OnRecovery(StrategyFallback, err, false)
			}
			
			rm.logger.Error("Fallback recovery failed",
				zap.String("service", serviceName),
				zap.Error(fallbackErr))
			return nil, fallbackErr
		}
		
		return nil, err
	}
	
	return result, nil
}

// executeRetryWithBackoff executes with retry and backoff strategy
func (rm *RecoveryManager) executeRetryWithBackoff(ctx context.Context, serviceName string, operation func(ctx context.Context) (interface{}, error), config *RecoveryConfig, state *RecoveryState) (interface{}, error) {
	if config.RetryConfig == nil {
		config.RetryConfig = GetDefaultRetryConfig()
	}
	
	retryManager := NewRetryManager(config.RetryConfig, rm.logger)
	
	result, err := retryManager.ExecuteWithResultAndContext(ctx, operation)
	if err != nil {
		rm.updateFailureState(state, err)
		
		if config.OnRecovery != nil {
			config.OnRecovery(StrategyRetryWithBackoff, err, false)
		}
	} else {
		state.RecoveryAttempts++
		state.LastRecoveryTime = time.Now()
		
		if config.OnRecovery != nil {
			config.OnRecovery(StrategyRetryWithBackoff, nil, true)
		}
	}
	
	return result, err
}

// executeCircuitBreaker executes with circuit breaker strategy
func (rm *RecoveryManager) executeCircuitBreaker(ctx context.Context, serviceName string, operation func(ctx context.Context) (interface{}, error), config *RecoveryConfig, state *RecoveryState) (interface{}, error) {
	if config.CircuitBreakerConfig == nil {
		config.CircuitBreakerConfig = GetDefaultDatabaseConfig()
	}
	
	cbManager := NewCircuitBreakerManager(rm.logger)
	cbManager.RegisterBreaker(serviceName, config.CircuitBreakerConfig)
	
	result, err := cbManager.ExecuteWithContext(ctx, serviceName, operation)
	if err != nil {
		rm.updateFailureState(state, err)
		
		if config.OnRecovery != nil {
			config.OnRecovery(StrategyCircuitBreaker, err, false)
		}
	} else {
		state.RecoveryAttempts++
		state.LastRecoveryTime = time.Now()
		
		if config.OnRecovery != nil {
			config.OnRecovery(StrategyCircuitBreaker, nil, true)
		}
	}
	
	return result, err
}

// executeBulkhead executes with bulkhead isolation strategy
func (rm *RecoveryManager) executeBulkhead(ctx context.Context, serviceName string, operation func(ctx context.Context) (interface{}, error), config *RecoveryConfig, state *RecoveryState) (interface{}, error) {
	bulkhead, exists := rm.bulkheads[serviceName]
	if !exists {
		return operation(ctx)
	}
	
	// Try to acquire semaphore
	select {
	case bulkhead <- struct{}{}:
		// Acquired semaphore, proceed with operation
		defer func() { <-bulkhead }()
		
		// Set timeout if configured
		operationCtx := ctx
		if config.BulkheadConfig != nil && config.BulkheadConfig.TimeoutPerRequest > 0 {
			var cancel context.CancelFunc
			operationCtx, cancel = context.WithTimeout(ctx, config.BulkheadConfig.TimeoutPerRequest)
			defer cancel()
		}
		
		result, err := operation(operationCtx)
		if err != nil {
			rm.updateFailureState(state, err)
			
			if config.OnRecovery != nil {
				config.OnRecovery(StrategyBulkhead, err, false)
			}
		} else {
			state.RecoveryAttempts++
			state.LastRecoveryTime = time.Now()
			
			if config.OnRecovery != nil {
				config.OnRecovery(StrategyBulkhead, nil, true)
			}
		}
		
		return result, err
		
	case <-ctx.Done():
		// Context cancelled while waiting for semaphore
		err := NewError(ErrorCodeResourceExhausted, fmt.Sprintf("Bulkhead semaphore acquisition cancelled for service: %s", serviceName)).
			WithCause(ctx.Err()).
			WithCategory(CategoryInternal).
			WithSeverity(SeverityWarning).
			WithRetryable(true).
			Build()
		
		rm.updateFailureState(state, err)
		return nil, err
		
	default:
		// Semaphore full, reject request
		err := NewError(ErrorCodeResourceExhausted, fmt.Sprintf("Bulkhead capacity exceeded for service: %s", serviceName)).
			WithCategory(CategoryInternal).
			WithSeverity(SeverityWarning).
			WithRetryable(true).
			WithMetadata("max_concurrent", config.BulkheadConfig.MaxConcurrentRequests).
			Build()
		
		rm.updateFailureState(state, err)
		return nil, err
	}
}

// updateFailureState updates the failure state for a service
func (rm *RecoveryManager) updateFailureState(state *RecoveryState, err error) {
	state.LastFailure = time.Now()
	state.FailureCount++
	
	rm.logger.Debug("Updated failure state",
		zap.String("service", state.ServiceName),
		zap.Int("failure_count", state.FailureCount),
		zap.Error(err))
}

// GetRecoveryState returns the current recovery state for a service
func (rm *RecoveryManager) GetRecoveryState(serviceName string) (*RecoveryState, error) {
	rm.mu.RLock()
	defer rm.mu.RUnlock()
	
	state, exists := rm.recoveryStates[serviceName]
	if !exists {
		return nil, NewError(ErrorCodeNotFound, fmt.Sprintf("Recovery state not found for service: %s", serviceName)).
			WithCategory(CategoryInternal).
			WithSeverity(SeverityWarning).
			WithRetryable(false).
			Build()
	}
	
	// Return a copy to prevent external modification
	stateCopy := *state
	return &stateCopy, nil
}

// GetAllRecoveryStates returns recovery states for all services
func (rm *RecoveryManager) GetAllRecoveryStates() map[string]*RecoveryState {
	rm.mu.RLock()
	defer rm.mu.RUnlock()
	
	states := make(map[string]*RecoveryState)
	for name, state := range rm.recoveryStates {
		stateCopy := *state
		states[name] = &stateCopy
	}
	
	return states
}

// ResetRecoveryState resets the recovery state for a service
func (rm *RecoveryManager) ResetRecoveryState(serviceName string) error {
	rm.mu.Lock()
	defer rm.mu.Unlock()
	
	state, exists := rm.recoveryStates[serviceName]
	if !exists {
		return NewError(ErrorCodeNotFound, fmt.Sprintf("Recovery state not found for service: %s", serviceName)).
			WithCategory(CategoryInternal).
			WithSeverity(SeverityWarning).
			WithRetryable(false).
			Build()
	}
	
	state.FailureCount = 0
	state.RecoveryAttempts = 0
	state.InDegradedMode = false
	state.LastFailure = time.Time{}
	state.DegradationStart = time.Time{}
	state.LastRecoveryTime = time.Time{}
	
	rm.logger.Info("Recovery state reset", zap.String("service", serviceName))
	return nil
}

// HealthCheck performs a health check on all services with recovery strategies
func (rm *RecoveryManager) HealthCheck(ctx context.Context) map[string]bool {
	rm.mu.RLock()
	defer rm.mu.RUnlock()
	
	health := make(map[string]bool)
	
	for serviceName, state := range rm.recoveryStates {
		// Consider service healthy if:
		// 1. No recent failures (within last 5 minutes)
		// 2. Not in degraded mode
		// 3. Recent recovery attempts were successful
		
		isHealthy := true
		
		// Check for recent failures
		if !state.LastFailure.IsZero() && time.Since(state.LastFailure) < 5*time.Minute {
			isHealthy = false
		}
		
		// Check degraded mode
		if state.InDegradedMode {
			isHealthy = false
		}
		
		// Check failure rate
		if state.FailureCount > 10 {
			isHealthy = false
		}
		
		health[serviceName] = isHealthy
	}
	
	return health
}