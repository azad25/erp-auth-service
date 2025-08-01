package interceptors

import (
	"context"
	"time"

	"github.com/sony/gobreaker"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// CircuitBreakerConfig holds configuration for circuit breaker
type CircuitBreakerConfig struct {
	MaxRequests        uint32
	Interval           time.Duration
	Timeout            time.Duration
	ReadyToTrip        func(counts gobreaker.Counts) bool
	OnStateChange      func(name string, from gobreaker.State, to gobreaker.State)
}

// CircuitBreakerManager manages circuit breakers for different dependencies
type CircuitBreakerManager struct {
	breakers map[string]*gobreaker.CircuitBreaker
	logger   *zap.Logger
}

// NewCircuitBreakerManager creates a new circuit breaker manager
func NewCircuitBreakerManager(logger *zap.Logger) *CircuitBreakerManager {
	return &CircuitBreakerManager{
		breakers: make(map[string]*gobreaker.CircuitBreaker),
		logger:   logger,
	}
}

// RegisterBreaker registers a new circuit breaker for a dependency
func (cbm *CircuitBreakerManager) RegisterBreaker(name string, config CircuitBreakerConfig) {
	settings := gobreaker.Settings{
		Name:        name,
		MaxRequests: config.MaxRequests,
		Interval:    config.Interval,
		Timeout:     config.Timeout,
		ReadyToTrip: config.ReadyToTrip,
		OnStateChange: func(name string, from gobreaker.State, to gobreaker.State) {
			cbm.logger.Info("Circuit breaker state changed",
				zap.String("name", name),
				zap.String("from", from.String()),
				zap.String("to", to.String()))
			
			if config.OnStateChange != nil {
				config.OnStateChange(name, from, to)
			}
		},
	}
	
	cbm.breakers[name] = gobreaker.NewCircuitBreaker(settings)
}

// Execute executes a function with circuit breaker protection
func (cbm *CircuitBreakerManager) Execute(name string, fn func() (interface{}, error)) (interface{}, error) {
	breaker, exists := cbm.breakers[name]
	if !exists {
		cbm.logger.Warn("Circuit breaker not found, executing without protection", zap.String("name", name))
		return fn()
	}
	
	return breaker.Execute(fn)
}

// UnaryServerInterceptor returns a gRPC unary server interceptor for circuit breaker
func (cbm *CircuitBreakerManager) UnaryServerInterceptor() grpc.UnaryServerInterceptor {
	return func(
		ctx context.Context,
		req interface{},
		info *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (interface{}, error) {
		// Execute handler with circuit breaker protection
		result, err := cbm.Execute("grpc-handler", func() (interface{}, error) {
			return handler(ctx, req)
		})
		
		if err != nil {
			// Check if error is from circuit breaker
			if err == gobreaker.ErrOpenState {
				return nil, status.Errorf(codes.Unavailable, "service temporarily unavailable")
			} else if err == gobreaker.ErrTooManyRequests {
				return nil, status.Errorf(codes.ResourceExhausted, "too many requests")
			}
		}
		
		return result, err
	}
}

// StreamServerInterceptor returns a gRPC stream server interceptor for circuit breaker
func (cbm *CircuitBreakerManager) StreamServerInterceptor() grpc.StreamServerInterceptor {
	return func(
		srv interface{},
		stream grpc.ServerStream,
		info *grpc.StreamServerInfo,
		handler grpc.StreamHandler,
	) error {
		// Execute handler with circuit breaker protection
		_, err := cbm.Execute("grpc-stream-handler", func() (interface{}, error) {
			return nil, handler(srv, stream)
		})
		
		if err != nil {
			// Check if error is from circuit breaker
			if err == gobreaker.ErrOpenState {
				return status.Errorf(codes.Unavailable, "service temporarily unavailable")
			} else if err == gobreaker.ErrTooManyRequests {
				return status.Errorf(codes.ResourceExhausted, "too many requests")
			}
		}
		
		return err
	}
}

// GetDefaultDatabaseConfig returns default configuration for database circuit breaker
func GetDefaultDatabaseConfig() CircuitBreakerConfig {
	return CircuitBreakerConfig{
		MaxRequests: 3,
		Interval:    time.Second * 30,
		Timeout:     time.Second * 60,
		ReadyToTrip: func(counts gobreaker.Counts) bool {
			failureRatio := float64(counts.TotalFailures) / float64(counts.Requests)
			return counts.Requests >= 3 && failureRatio >= 0.6
		},
	}
}

// GetDefaultRedisConfig returns default configuration for Redis circuit breaker
func GetDefaultRedisConfig() CircuitBreakerConfig {
	return CircuitBreakerConfig{
		MaxRequests: 5,
		Interval:    time.Second * 10,
		Timeout:     time.Second * 30,
		ReadyToTrip: func(counts gobreaker.Counts) bool {
			failureRatio := float64(counts.TotalFailures) / float64(counts.Requests)
			return counts.Requests >= 5 && failureRatio >= 0.5
		},
	}
}

// GetDefaultKafkaConfig returns default configuration for Kafka circuit breaker
func GetDefaultKafkaConfig() CircuitBreakerConfig {
	return CircuitBreakerConfig{
		MaxRequests: 2,
		Interval:    time.Second * 20,
		Timeout:     time.Second * 45,
		ReadyToTrip: func(counts gobreaker.Counts) bool {
			failureRatio := float64(counts.TotalFailures) / float64(counts.Requests)
			return counts.Requests >= 2 && failureRatio >= 0.7
		},
	}
}