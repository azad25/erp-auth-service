package interceptors

import (
	"context"
	"fmt"
	"runtime/debug"

	"erp-auth-service/internal/errors"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// RecoveryInterceptor provides panic recovery for gRPC requests
type RecoveryInterceptor struct {
	logger *zap.Logger
}

// NewRecoveryInterceptor creates a new recovery interceptor
func NewRecoveryInterceptor(logger *zap.Logger) *RecoveryInterceptor {
	return &RecoveryInterceptor{
		logger: logger,
	}
}

// UnaryServerInterceptor returns a gRPC unary server interceptor for panic recovery
func (ri *RecoveryInterceptor) UnaryServerInterceptor() grpc.UnaryServerInterceptor {
	return func(
		ctx context.Context,
		req interface{},
		info *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (resp interface{}, err error) {
		defer func() {
			if r := recover(); r != nil {
				// Create structured error for panic
				panicErr := errors.NewError(errors.ErrorCodeInternal, fmt.Sprintf("Panic recovered in gRPC handler: %v", r)).
					WithCategory(errors.CategoryInternal).
					WithSeverity(errors.SeverityCritical).
					WithRetryable(false).
					WithMetadata("method", info.FullMethod).
					WithMetadata("panic_value", r).
					WithMetadata("stack_trace", string(debug.Stack())).
					Build()
				
				// Extract context information if available
				if traceID := ctx.Value("trace_id"); traceID != nil {
					if traceIDStr, ok := traceID.(string); ok {
						panicErr.TraceID = traceIDStr
					}
				}
				
				// Log the panic with structured error
				ri.logger.Error("gRPC handler panic recovered",
					zap.String("method", info.FullMethod),
					zap.Any("panic", r),
					zap.String("trace_id", panicErr.TraceID),
					zap.String("stack", string(debug.Stack())))
				
				// Convert to gRPC status
				err = panicErr.ToGRPCStatus().Err()
				resp = nil
			}
		}()
		
		return handler(ctx, req)
	}
}

// StreamServerInterceptor returns a gRPC stream server interceptor for panic recovery
func (ri *RecoveryInterceptor) StreamServerInterceptor() grpc.StreamServerInterceptor {
	return func(
		srv interface{},
		stream grpc.ServerStream,
		info *grpc.StreamServerInfo,
		handler grpc.StreamHandler,
	) (err error) {
		defer func() {
			if r := recover(); r != nil {
				// Create structured error for panic
				panicErr := errors.NewError(errors.ErrorCodeInternal, fmt.Sprintf("Panic recovered in gRPC stream handler: %v", r)).
					WithCategory(errors.CategoryInternal).
					WithSeverity(errors.SeverityCritical).
					WithRetryable(false).
					WithMetadata("method", info.FullMethod).
					WithMetadata("panic_value", r).
					WithMetadata("stack_trace", string(debug.Stack())).
					Build()
				
				// Extract context information if available from stream context
				ctx := stream.Context()
				if traceID := ctx.Value("trace_id"); traceID != nil {
					if traceIDStr, ok := traceID.(string); ok {
						panicErr.TraceID = traceIDStr
					}
				}
				
				// Log the panic with structured error
				ri.logger.Error("gRPC stream handler panic recovered",
					zap.String("method", info.FullMethod),
					zap.Any("panic", r),
					zap.String("trace_id", panicErr.TraceID),
					zap.String("stack", string(debug.Stack())))
				
				// Convert to gRPC status
				err = panicErr.ToGRPCStatus().Err()
			}
		}()
		
		return handler(srv, stream)
	}
}