package errors

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/sony/gobreaker"
	"go.uber.org/zap"
)

// CircuitBreakerState represents the state of a circuit breaker
type CircuitBreakerState string

const (
	StateClosed   CircuitBreakerState = "CLOSED"
	StateOpen     CircuitBreakerState = "OPEN"
	StateHalfOpen CircuitBreakerState = "HALF_OPEN"
)

// CircuitBreakerConfig holds configuration for a circuit breaker
type CircuitBreakerConfig struct {
	// Name of the circuit breaker
	Name string
	
	// MaxRequests is the maximum number of requests allowed to pass through
	// when the CircuitBreaker is half-open
	MaxRequests uint32
	
	// Interval is the cyclic period of the closed state
	// for the CircuitBreaker to clear the internal Counts
	Interval time.Duration
	
	// Timeout is the period of the open state,
	// after which the state of the CircuitBreaker becomes half-open
	Timeout time.Duration
	
	// ReadyToTrip is called with a copy of Counts whenever a request fails in the closed state.
	// If ReadyToTrip returns true, the CircuitBreaker will be placed into the open state.
	ReadyToTrip func(counts gobreaker.Counts) bool
	
	// OnStateChange is called whenever the state of the CircuitBreaker changes
	OnStateChange func(name string, from gobreaker.State, to gobreaker.State)
	
	// IsSuccessful is called with the error returned from a request.
	// If IsSuccessful returns true, the request is considered successful;
	// otherwise, it's considered a failure.
	IsSuccessful func(err error) bool
}

// CircuitBreakerManager manages multiple circuit breakers for different services
type CircuitBreakerManager struct {
	breakers map[string]*gobreaker.CircuitBreaker
	configs  map[string]*CircuitBreakerConfig
	logger   *zap.Logger
	mu       sync.RWMutex
}

// NewCircuitBreakerManager creates a new circuit breaker manager
func NewCircuitBreakerManager(logger *zap.Logger) *CircuitBreakerManager {
	return &CircuitBreakerManager{
		breakers: make(map[string]*gobreaker.CircuitBreaker),
		configs:  make(map[string]*CircuitBreakerConfig),
		logger:   logger,
	}
}

// RegisterBreaker registers a new circuit breaker with the given configuration
func (cbm *CircuitBreakerManager) RegisterBreaker(name string, config *CircuitBreakerConfig) {
	cbm.mu.Lock()
	defer cbm.mu.Unlock()
	
	config.Name = name
	
	// Set default OnStateChange if not provided
	if config.OnStateChange == nil {
		config.OnStateChange = func(name string, from gobreaker.State, to gobreaker.State) {
			cbm.logger.Info("Circuit breaker state changed",
				zap.String("name", name),
				zap.String("from", cbm.stateToString(from)),
				zap.String("to", cbm.stateToString(to)))
		}
	}
	
	// Set default IsSuccessful if not provided
	if config.IsSuccessful == nil {
		config.IsSuccessful = func(err error) bool {
			return err == nil
		}
	}
	
	settings := gobreaker.Settings{
		Name:          name,
		MaxRequests:   config.MaxRequests,
		Interval:      config.Interval,
		Timeout:       config.Timeout,
		ReadyToTrip:   config.ReadyToTrip,
		OnStateChange: config.OnStateChange,
		IsSuccessful:  config.IsSuccessful,
	}
	
	cbm.breakers[name] = gobreaker.NewCircuitBreaker(settings)
	cbm.configs[name] = config
	
	cbm.logger.Info("Circuit breaker registered",
		zap.String("name", name),
		zap.Uint32("max_requests", config.MaxRequests),
		zap.Duration("interval", config.Interval),
		zap.Duration("timeout", config.Timeout))
}

// Execute executes a function with circuit breaker protection
func (cbm *CircuitBreakerManager) Execute(name string, fn func() (interface{}, error)) (interface{}, error) {
	cbm.mu.RLock()
	breaker, exists := cbm.breakers[name]
	cbm.mu.RUnlock()
	
	if !exists {
		return nil, NewError(ErrorCodeInternal, fmt.Sprintf("Circuit breaker '%s' not found", name)).
			WithCategory(CategoryCircuitBreaker).
			WithSeverity(SeverityError).
			WithRetryable(false).
			Build()
	}
	
	result, err := breaker.Execute(fn)
	if err != nil {
		// Check if it's a circuit breaker error
		if err == gobreaker.ErrOpenState {
			return nil, NewCircuitBreakerError(name)
		}
		if err == gobreaker.ErrTooManyRequests {
			return nil, NewError(ErrorCodeResourceExhausted, fmt.Sprintf("Too many requests to service: %s", name)).
				WithCategory(CategoryCircuitBreaker).
				WithSeverity(SeverityWarning).
				WithRetryable(true).
				WithMetadata("service", name).
				Build()
		}
		
		// Wrap the original error
		return nil, WrapError(err, ErrorCodeInternal, fmt.Sprintf("Circuit breaker execution failed for service: %s", name))
	}
	
	return result, nil
}

// ExecuteWithContext executes a function with circuit breaker protection and context
func (cbm *CircuitBreakerManager) ExecuteWithContext(ctx context.Context, name string, fn func(ctx context.Context) (interface{}, error)) (interface{}, error) {
	cbm.mu.RLock()
	breaker, exists := cbm.breakers[name]
	cbm.mu.RUnlock()
	
	if !exists {
		return nil, NewError(ErrorCodeInternal, fmt.Sprintf("Circuit breaker '%s' not found", name)).
			WithCategory(CategoryCircuitBreaker).
			WithSeverity(SeverityError).
			WithRetryable(false).
			Build()
	}
	
	// Check context cancellation before execution
	select {
	case <-ctx.Done():
		return nil, NewError(ErrorCodeCancelled, "Context cancelled before circuit breaker execution").
			WithCause(ctx.Err()).
			WithCategory(CategoryInternal).
			WithSeverity(SeverityInfo).
			WithRetryable(false).
			Build()
	default:
	}
	
	result, err := breaker.Execute(func() (interface{}, error) {
		return fn(ctx)
	})
	
	if err != nil {
		// Check if it's a circuit breaker error
		if err == gobreaker.ErrOpenState {
			return nil, NewCircuitBreakerError(name)
		}
		if err == gobreaker.ErrTooManyRequests {
			return nil, NewError(ErrorCodeResourceExhausted, fmt.Sprintf("Too many requests to service: %s", name)).
				WithCategory(CategoryCircuitBreaker).
				WithSeverity(SeverityWarning).
				WithRetryable(true).
				WithMetadata("service", name).
				Build()
		}
		
		// Wrap the original error with context information
		authErr := WrapError(err, ErrorCodeInternal, fmt.Sprintf("Circuit breaker execution failed for service: %s", name))
		return nil, ErrorFromContext(ctx, authErr)
	}
	
	return result, nil
}

// GetState returns the current state of a circuit breaker
func (cbm *CircuitBreakerManager) GetState(name string) (CircuitBreakerState, error) {
	cbm.mu.RLock()
	breaker, exists := cbm.breakers[name]
	cbm.mu.RUnlock()
	
	if !exists {
		return "", NewError(ErrorCodeNotFound, fmt.Sprintf("Circuit breaker '%s' not found", name)).
			WithCategory(CategoryCircuitBreaker).
			WithSeverity(SeverityError).
			WithRetryable(false).
			Build()
	}
	
	state := breaker.State()
	switch state {
	case gobreaker.StateClosed:
		return StateClosed, nil
	case gobreaker.StateOpen:
		return StateOpen, nil
	case gobreaker.StateHalfOpen:
		return StateHalfOpen, nil
	default:
		return "", NewError(ErrorCodeUnknown, fmt.Sprintf("Unknown circuit breaker state: %v", state)).
			WithCategory(CategoryCircuitBreaker).
			WithSeverity(SeverityError).
			WithRetryable(false).
			Build()
	}
}

// GetCounts returns the current counts for a circuit breaker
func (cbm *CircuitBreakerManager) GetCounts(name string) (gobreaker.Counts, error) {
	cbm.mu.RLock()
	breaker, exists := cbm.breakers[name]
	cbm.mu.RUnlock()
	
	if !exists {
		return gobreaker.Counts{}, NewError(ErrorCodeNotFound, fmt.Sprintf("Circuit breaker '%s' not found", name)).
			WithCategory(CategoryCircuitBreaker).
			WithSeverity(SeverityError).
			WithRetryable(false).
			Build()
	}
	
	return breaker.Counts(), nil
}

// Reset resets a circuit breaker to its initial state
func (cbm *CircuitBreakerManager) Reset(name string) error {
	cbm.mu.Lock()
	defer cbm.mu.Unlock()
	
	config, exists := cbm.configs[name]
	if !exists {
		return NewError(ErrorCodeNotFound, fmt.Sprintf("Circuit breaker '%s' not found", name)).
			WithCategory(CategoryCircuitBreaker).
			WithSeverity(SeverityError).
			WithRetryable(false).
			Build()
	}
	
	// Re-create the circuit breaker to reset it
	settings := gobreaker.Settings{
		Name:          name,
		MaxRequests:   config.MaxRequests,
		Interval:      config.Interval,
		Timeout:       config.Timeout,
		ReadyToTrip:   config.ReadyToTrip,
		OnStateChange: config.OnStateChange,
		IsSuccessful:  config.IsSuccessful,
	}
	
	cbm.breakers[name] = gobreaker.NewCircuitBreaker(settings)
	cbm.logger.Info("Circuit breaker reset", zap.String("name", name))
	return nil
}

// GetAllStates returns the states of all registered circuit breakers
func (cbm *CircuitBreakerManager) GetAllStates() map[string]CircuitBreakerState {
	cbm.mu.RLock()
	defer cbm.mu.RUnlock()
	
	states := make(map[string]CircuitBreakerState)
	for name, breaker := range cbm.breakers {
		state := breaker.State()
		switch state {
		case gobreaker.StateClosed:
			states[name] = StateClosed
		case gobreaker.StateOpen:
			states[name] = StateOpen
		case gobreaker.StateHalfOpen:
			states[name] = StateHalfOpen
		}
	}
	
	return states
}

// GetAllCounts returns the counts of all registered circuit breakers
func (cbm *CircuitBreakerManager) GetAllCounts() map[string]gobreaker.Counts {
	cbm.mu.RLock()
	defer cbm.mu.RUnlock()
	
	counts := make(map[string]gobreaker.Counts)
	for name, breaker := range cbm.breakers {
		counts[name] = breaker.Counts()
	}
	
	return counts
}

// Predefined circuit breaker configurations

// GetDefaultDatabaseConfig returns default configuration for database circuit breaker
func GetDefaultDatabaseConfig() *CircuitBreakerConfig {
	return &CircuitBreakerConfig{
		MaxRequests: 10,
		Interval:    time.Second * 30,
		Timeout:     time.Second * 60,
		ReadyToTrip: func(counts gobreaker.Counts) bool {
			failureRatio := float64(counts.TotalFailures) / float64(counts.Requests)
			return counts.Requests >= 5 && failureRatio >= 0.6
		},
		IsSuccessful: func(err error) bool {
			// Consider database connection errors as failures
			if err == nil {
				return true
			}
			
			// Check for specific database errors that should trip the breaker
			errStr := err.Error()
			return !(contains(errStr, "connection refused") ||
				contains(errStr, "connection reset") ||
				contains(errStr, "timeout") ||
				contains(errStr, "connection lost"))
		},
	}
}

// GetDefaultRedisConfig returns default configuration for Redis circuit breaker
func GetDefaultRedisConfig() *CircuitBreakerConfig {
	return &CircuitBreakerConfig{
		MaxRequests: 15,
		Interval:    time.Second * 20,
		Timeout:     time.Second * 30,
		ReadyToTrip: func(counts gobreaker.Counts) bool {
			failureRatio := float64(counts.TotalFailures) / float64(counts.Requests)
			return counts.Requests >= 3 && failureRatio >= 0.5
		},
		IsSuccessful: func(err error) bool {
			if err == nil {
				return true
			}
			
			// Redis errors that should trip the breaker
			errStr := err.Error()
			return !(contains(errStr, "connection refused") ||
				contains(errStr, "connection reset") ||
				contains(errStr, "timeout") ||
				contains(errStr, "connection lost") ||
				contains(errStr, "redis: nil"))
		},
	}
}

// GetDefaultKafkaConfig returns default configuration for Kafka circuit breaker
func GetDefaultKafkaConfig() *CircuitBreakerConfig {
	return &CircuitBreakerConfig{
		MaxRequests: 5,
		Interval:    time.Second * 60,
		Timeout:     time.Second * 120,
		ReadyToTrip: func(counts gobreaker.Counts) bool {
			failureRatio := float64(counts.TotalFailures) / float64(counts.Requests)
			return counts.Requests >= 3 && failureRatio >= 0.7
		},
		IsSuccessful: func(err error) bool {
			if err == nil {
				return true
			}
			
			// Kafka errors that should trip the breaker
			errStr := err.Error()
			return !(contains(errStr, "connection refused") ||
				contains(errStr, "broker not available") ||
				contains(errStr, "timeout") ||
				contains(errStr, "network error"))
		},
	}
}

// GetDefaultGRPCConfig returns default configuration for gRPC handler circuit breaker
func GetDefaultGRPCConfig() *CircuitBreakerConfig {
	return &CircuitBreakerConfig{
		MaxRequests: 20,
		Interval:    time.Second * 30,
		Timeout:     time.Second * 60,
		ReadyToTrip: func(counts gobreaker.Counts) bool {
			failureRatio := float64(counts.TotalFailures) / float64(counts.Requests)
			return counts.Requests >= 10 && failureRatio >= 0.3
		},
		IsSuccessful: func(err error) bool {
			if err == nil {
				return true
			}
			
			// Only consider internal server errors as failures
			if authErr, ok := err.(*AuthError); ok {
				return authErr.Code != ErrorCodeInternal &&
					authErr.Code != ErrorCodeDatabaseError &&
					authErr.Code != ErrorCodeCacheError
			}
			
			return false
		},
	}
}

// stateToString converts gobreaker.State to string
func (cbm *CircuitBreakerManager) stateToString(state gobreaker.State) string {
	switch state {
	case gobreaker.StateClosed:
		return "CLOSED"
	case gobreaker.StateOpen:
		return "OPEN"
	case gobreaker.StateHalfOpen:
		return "HALF_OPEN"
	default:
		return "UNKNOWN"
	}
}

// Helper function to check if a string contains a substring (case-insensitive)
func contains(s, substr string) bool {
	return len(s) >= len(substr) && 
		(s == substr || 
		 (len(s) > len(substr) && 
		  (s[:len(substr)] == substr || 
		   s[len(s)-len(substr):] == substr || 
		   containsSubstring(s, substr))))
}

func containsSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}