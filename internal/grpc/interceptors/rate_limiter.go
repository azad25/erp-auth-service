package interceptors

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
	"golang.org/x/time/rate"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/status"
)

// RateLimiterConfig holds configuration for rate limiting
type RateLimiterConfig struct {
	RequestsPerSecond int
	BurstSize         int
	WindowSize        time.Duration
	RedisKeyPrefix    string
}

// RateLimiter implements Redis-based rate limiting for gRPC
type RateLimiter struct {
	redis  *redis.Client
	config RateLimiterConfig
	logger *zap.Logger
	
	// In-memory fallback rate limiter
	fallbackLimiter *rate.Limiter
}

// NewRateLimiter creates a new Redis-based rate limiter
func NewRateLimiter(redisClient *redis.Client, config RateLimiterConfig, logger *zap.Logger) *RateLimiter {
	return &RateLimiter{
		redis:           redisClient,
		config:          config,
		logger:          logger,
		fallbackLimiter: rate.NewLimiter(rate.Limit(config.RequestsPerSecond), config.BurstSize),
	}
}

// UnaryServerInterceptor returns a gRPC unary server interceptor for rate limiting
func (rl *RateLimiter) UnaryServerInterceptor() grpc.UnaryServerInterceptor {
	return func(
		ctx context.Context,
		req interface{},
		info *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (interface{}, error) {
		// Extract client identifier (IP address or user ID)
		clientID := rl.getClientIdentifier(ctx)
		
		// Check rate limit
		allowed, err := rl.isAllowed(ctx, clientID, info.FullMethod)
		if err != nil {
			rl.logger.Error("Rate limiter error", 
				zap.String("client_id", clientID),
				zap.String("method", info.FullMethod),
				zap.Error(err))
			
			// Fallback to in-memory rate limiter
			if !rl.fallbackLimiter.Allow() {
				return nil, status.Errorf(codes.ResourceExhausted, "rate limit exceeded")
			}
		} else if !allowed {
			return nil, status.Errorf(codes.ResourceExhausted, "rate limit exceeded")
		}
		
		return handler(ctx, req)
	}
}

// StreamServerInterceptor returns a gRPC stream server interceptor for rate limiting
func (rl *RateLimiter) StreamServerInterceptor() grpc.StreamServerInterceptor {
	return func(
		srv interface{},
		stream grpc.ServerStream,
		info *grpc.StreamServerInfo,
		handler grpc.StreamHandler,
	) error {
		ctx := stream.Context()
		clientID := rl.getClientIdentifier(ctx)
		
		allowed, err := rl.isAllowed(ctx, clientID, info.FullMethod)
		if err != nil {
			rl.logger.Error("Rate limiter error", 
				zap.String("client_id", clientID),
				zap.String("method", info.FullMethod),
				zap.Error(err))
			
			if !rl.fallbackLimiter.Allow() {
				return status.Errorf(codes.ResourceExhausted, "rate limit exceeded")
			}
		} else if !allowed {
			return status.Errorf(codes.ResourceExhausted, "rate limit exceeded")
		}
		
		return handler(srv, stream)
	}
}

// isAllowed checks if the request is allowed based on Redis sliding window
func (rl *RateLimiter) isAllowed(ctx context.Context, clientID, method string) (bool, error) {
	key := fmt.Sprintf("%s:%s:%s", rl.config.RedisKeyPrefix, clientID, method)
	now := time.Now().Unix()
	windowStart := now - int64(rl.config.WindowSize.Seconds())
	
	pipe := rl.redis.Pipeline()
	
	// Remove expired entries
	pipe.ZRemRangeByScore(ctx, key, "0", fmt.Sprintf("%d", windowStart))
	
	// Count current requests in window
	countCmd := pipe.ZCard(ctx, key)
	
	// Add current request
	pipe.ZAdd(ctx, key, redis.Z{Score: float64(now), Member: fmt.Sprintf("%d", now)})
	
	// Set expiry
	pipe.Expire(ctx, key, rl.config.WindowSize)
	
	_, err := pipe.Exec(ctx)
	if err != nil {
		return false, err
	}
	
	count := countCmd.Val()
	return count < int64(rl.config.RequestsPerSecond), nil
}

// getClientIdentifier extracts client identifier from context
func (rl *RateLimiter) getClientIdentifier(ctx context.Context) string {
	// Try to get user ID from metadata first
	if md, ok := metadata.FromIncomingContext(ctx); ok {
		if userIDs := md.Get("user-id"); len(userIDs) > 0 {
			return fmt.Sprintf("user:%s", userIDs[0])
		}
	}
	
	// Fallback to IP address
	if peer, ok := peer.FromContext(ctx); ok {
		return fmt.Sprintf("ip:%s", peer.Addr.String())
	}
	
	return "unknown"
}