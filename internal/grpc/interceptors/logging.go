package interceptors

import (
	"context"
	"time"

	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/status"
)

// LoggingInterceptor provides structured logging for gRPC requests
type LoggingInterceptor struct {
	logger *zap.Logger
}

// NewLoggingInterceptor creates a new logging interceptor
func NewLoggingInterceptor(logger *zap.Logger) *LoggingInterceptor {
	return &LoggingInterceptor{
		logger: logger,
	}
}

// UnaryServerInterceptor returns a gRPC unary server interceptor for logging
func (li *LoggingInterceptor) UnaryServerInterceptor() grpc.UnaryServerInterceptor {
	return func(
		ctx context.Context,
		req interface{},
		info *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (interface{}, error) {
		start := time.Now()
		
		// Extract request metadata
		fields := li.extractRequestFields(ctx, info.FullMethod)
		
		// Execute handler
		resp, err := handler(ctx, req)
		
		// Calculate duration
		duration := time.Since(start)
		
		// Add response fields
		fields = append(fields,
			zap.Duration("duration", duration),
			zap.String("grpc_code", status.Code(err).String()),
		)
		
		// Log based on error status
		if err != nil {
			code := status.Code(err)
			if code == codes.Internal || code == codes.Unknown {
				li.logger.Error("gRPC request failed", fields...)
			} else {
				li.logger.Warn("gRPC request error", append(fields, zap.Error(err))...)
			}
		} else {
			li.logger.Info("gRPC request completed", fields...)
		}
		
		return resp, err
	}
}

// StreamServerInterceptor returns a gRPC stream server interceptor for logging
func (li *LoggingInterceptor) StreamServerInterceptor() grpc.StreamServerInterceptor {
	return func(
		srv interface{},
		stream grpc.ServerStream,
		info *grpc.StreamServerInfo,
		handler grpc.StreamHandler,
	) error {
		start := time.Now()
		ctx := stream.Context()
		
		// Extract request metadata
		fields := li.extractRequestFields(ctx, info.FullMethod)
		fields = append(fields, zap.Bool("is_stream", true))
		
		// Execute handler
		err := handler(srv, stream)
		
		// Calculate duration
		duration := time.Since(start)
		fields = append(fields,
			zap.Duration("duration", duration),
			zap.String("grpc_code", status.Code(err).String()),
		)
		
		// Log based on error status
		if err != nil {
			code := status.Code(err)
			if code == codes.Internal || code == codes.Unknown {
				li.logger.Error("gRPC stream failed", fields...)
			} else {
				li.logger.Warn("gRPC stream error", append(fields, zap.Error(err))...)
			}
		} else {
			li.logger.Info("gRPC stream completed", fields...)
		}
		
		return err
	}
}

// extractRequestFields extracts common fields from the request context
func (li *LoggingInterceptor) extractRequestFields(ctx context.Context, method string) []zap.Field {
	fields := []zap.Field{
		zap.String("method", method),
		zap.Time("timestamp", time.Now()),
	}
	
	// Extract peer information
	if peer, ok := peer.FromContext(ctx); ok {
		fields = append(fields, zap.String("peer_addr", peer.Addr.String()))
	}
	
	// Extract metadata
	if md, ok := metadata.FromIncomingContext(ctx); ok {
		// Add correlation ID if present
		if correlationIDs := md.Get("correlation-id"); len(correlationIDs) > 0 {
			fields = append(fields, zap.String("correlation_id", correlationIDs[0]))
		}
		
		// Add user agent if present
		if userAgents := md.Get("user-agent"); len(userAgents) > 0 {
			fields = append(fields, zap.String("user_agent", userAgents[0]))
		}
		
		// Add user ID if present
		if userIDs := md.Get("user-id"); len(userIDs) > 0 {
			fields = append(fields, zap.String("user_id", userIDs[0]))
		}
		
		// Add organization ID if present
		if orgIDs := md.Get("organization-id"); len(orgIDs) > 0 {
			fields = append(fields, zap.String("organization_id", orgIDs[0]))
		}
	}
	
	return fields
}