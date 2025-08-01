package interceptors

import (
	"context"
	"strings"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

// AuthInterceptor provides JWT-based authentication for gRPC requests
type AuthInterceptor struct {
	jwtSecret       string
	logger          *zap.Logger
	publicMethods   map[string]bool
	serviceKeyMethods map[string]bool
	serviceKey      string
}

// Claims represents JWT claims structure
type Claims struct {
	UserID         uuid.UUID `json:"user_id"`
	OrganizationID uuid.UUID `json:"organization_id"`
	Email          string    `json:"email"`
	jwt.RegisteredClaims
}

// NewAuthInterceptor creates a new authentication interceptor
func NewAuthInterceptor(jwtSecret, serviceKey string, logger *zap.Logger) *AuthInterceptor {
	// Define methods that don't require authentication
	publicMethods := map[string]bool{
		"/auth.AuthService/HealthCheck": true,
		"/auth.AuthService/GetMetrics":  true,
	}
	
	// Define methods that can use service key authentication
	serviceKeyMethods := map[string]bool{
		"/auth.AuthService/ValidateToken":    true,
		"/auth.AuthService/CheckPermission":  true,
		"/auth.AuthService/GetUser":          true,
	}
	
	return &AuthInterceptor{
		jwtSecret:         jwtSecret,
		logger:            logger,
		publicMethods:     publicMethods,
		serviceKeyMethods: serviceKeyMethods,
		serviceKey:        serviceKey,
	}
}

// UnaryServerInterceptor returns a gRPC unary server interceptor for authentication
func (ai *AuthInterceptor) UnaryServerInterceptor() grpc.UnaryServerInterceptor {
	return func(
		ctx context.Context,
		req interface{},
		info *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (interface{}, error) {
		// Check if method is public
		if ai.publicMethods[info.FullMethod] {
			return handler(ctx, req)
		}
		
		// Extract metadata
		md, ok := metadata.FromIncomingContext(ctx)
		if !ok {
			return nil, status.Errorf(codes.Unauthenticated, "missing metadata")
		}
		
		// Try service key authentication first for applicable methods
		if ai.serviceKeyMethods[info.FullMethod] {
			if serviceKeys := md.Get("x-service-key"); len(serviceKeys) > 0 {
				if ai.validateServiceKey(serviceKeys[0]) {
					return handler(ctx, req)
				}
			}
		}
		
		// Try JWT authentication
		authHeaders := md.Get("authorization")
		if len(authHeaders) == 0 {
			return nil, status.Errorf(codes.Unauthenticated, "missing authorization header")
		}
		
		token := strings.TrimPrefix(authHeaders[0], "Bearer ")
		if token == authHeaders[0] {
			return nil, status.Errorf(codes.Unauthenticated, "invalid authorization header format")
		}
		
		claims, err := ai.validateJWT(token)
		if err != nil {
			ai.logger.Warn("JWT validation failed",
				zap.String("method", info.FullMethod),
				zap.Error(err))
			return nil, status.Errorf(codes.Unauthenticated, "invalid token")
		}
		
		// Add user context to metadata
		ctx = ai.addUserContext(ctx, claims)
		
		return handler(ctx, req)
	}
}

// StreamServerInterceptor returns a gRPC stream server interceptor for authentication
func (ai *AuthInterceptor) StreamServerInterceptor() grpc.StreamServerInterceptor {
	return func(
		srv interface{},
		stream grpc.ServerStream,
		info *grpc.StreamServerInfo,
		handler grpc.StreamHandler,
	) error {
		ctx := stream.Context()
		
		// Check if method is public
		if ai.publicMethods[info.FullMethod] {
			return handler(srv, stream)
		}
		
		// Extract metadata
		md, ok := metadata.FromIncomingContext(ctx)
		if !ok {
			return status.Errorf(codes.Unauthenticated, "missing metadata")
		}
		
		// Try service key authentication first for applicable methods
		if ai.serviceKeyMethods[info.FullMethod] {
			if serviceKeys := md.Get("x-service-key"); len(serviceKeys) > 0 {
				if ai.validateServiceKey(serviceKeys[0]) {
					return handler(srv, stream)
				}
			}
		}
		
		// Try JWT authentication
		authHeaders := md.Get("authorization")
		if len(authHeaders) == 0 {
			return status.Errorf(codes.Unauthenticated, "missing authorization header")
		}
		
		token := strings.TrimPrefix(authHeaders[0], "Bearer ")
		if token == authHeaders[0] {
			return status.Errorf(codes.Unauthenticated, "invalid authorization header format")
		}
		
		claims, err := ai.validateJWT(token)
		if err != nil {
			ai.logger.Warn("JWT validation failed",
				zap.String("method", info.FullMethod),
				zap.Error(err))
			return status.Errorf(codes.Unauthenticated, "invalid token")
		}
		
		// Create new context with user information
		ctx = ai.addUserContext(ctx, claims)
		
		// Create wrapped stream with new context
		wrappedStream := &wrappedServerStream{
			ServerStream: stream,
			ctx:          ctx,
		}
		
		return handler(srv, wrappedStream)
	}
}

// validateJWT validates a JWT token and returns claims
func (ai *AuthInterceptor) validateJWT(tokenString string) (*Claims, error) {
	claims := &Claims{}
	token, err := jwt.ParseWithClaims(tokenString, claims, func(token *jwt.Token) (interface{}, error) {
		return []byte(ai.jwtSecret), nil
	})
	
	if err != nil {
		return nil, err
	}
	
	if !token.Valid {
		return nil, jwt.ErrTokenMalformed
	}
	
	return claims, nil
}

// validateServiceKey validates a service key
func (ai *AuthInterceptor) validateServiceKey(key string) bool {
	return key == ai.serviceKey && ai.serviceKey != ""
}

// addUserContext adds user information to the context metadata
func (ai *AuthInterceptor) addUserContext(ctx context.Context, claims *Claims) context.Context {
	md := metadata.Pairs(
		"user-id", claims.UserID.String(),
		"organization-id", claims.OrganizationID.String(),
		"email", claims.Email,
	)
	
	return metadata.NewIncomingContext(ctx, md)
}

// wrappedServerStream wraps a grpc.ServerStream with a custom context
type wrappedServerStream struct {
	grpc.ServerStream
	ctx context.Context
}

// Context returns the wrapped context
func (w *wrappedServerStream) Context() context.Context {
	return w.ctx
}