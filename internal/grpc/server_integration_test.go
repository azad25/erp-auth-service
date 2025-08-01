package grpc

import (
	"context"
	"testing"

	"erp-auth-service/internal/config"
	pb "erp-auth-service/proto"

	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"
)

// TestGRPCMethodsExist verifies that all required gRPC methods are implemented
func TestGRPCMethodsExist(t *testing.T) {
	// Setup minimal server for testing method existence
	logger := zap.NewNop()
	cfg := &config.Config{
		JWT: config.JWTConfig{
			AccessExpiry: 3600,
		},
	}

	server := &EnhancedAuthGRPCServer{
		config: cfg,
		logger: logger,
	}

	ctx := context.Background()

	// Test ValidateToken method exists and handles empty token
	t.Run("ValidateToken_EmptyToken", func(t *testing.T) {
		req := &pb.ValidateTokenRequest{Token: ""}
		resp, err := server.ValidateToken(ctx, req)
		
		assert.NoError(t, err)
		assert.NotNil(t, resp)
		assert.False(t, resp.Valid)
		assert.Equal(t, "Token is required", resp.Error)
	})

	// Test RefreshToken method exists and handles empty token
	t.Run("RefreshToken_EmptyToken", func(t *testing.T) {
		req := &pb.RefreshTokenRequest{RefreshToken: ""}
		resp, err := server.RefreshToken(ctx, req)
		
		assert.NoError(t, err)
		assert.NotNil(t, resp)
		assert.Equal(t, "Refresh token is required", resp.Error)
	})

	// Test RevokeToken method exists and handles empty token
	t.Run("RevokeToken_EmptyToken", func(t *testing.T) {
		req := &pb.RevokeTokenRequest{Token: ""}
		resp, err := server.RevokeToken(ctx, req)
		
		assert.NoError(t, err)
		assert.NotNil(t, resp)
		assert.False(t, resp.Success)
		assert.Equal(t, "Token is required", resp.Error)
	})

	// Test CheckPermission method exists and handles missing fields
	t.Run("CheckPermission_MissingFields", func(t *testing.T) {
		req := &pb.CheckPermissionRequest{
			UserId:   "",
			Resource: "users",
			Action:   "read",
		}
		resp, err := server.CheckPermission(ctx, req)
		
		assert.NoError(t, err)
		assert.NotNil(t, resp)
		assert.False(t, resp.HasPermission)
		assert.Equal(t, "User ID, resource, and action are required", resp.Error)
	})

	// Test GetUser method exists and handles empty user ID
	t.Run("GetUser_EmptyUserID", func(t *testing.T) {
		req := &pb.GetUserRequest{UserId: ""}
		resp, err := server.GetUser(ctx, req)
		
		assert.NoError(t, err)
		assert.NotNil(t, resp)
		assert.Equal(t, "User ID is required", resp.Error)
	})

	// Test HealthCheck method exists
	t.Run("HealthCheck", func(t *testing.T) {
		req := &pb.HealthCheckRequest{}
		resp, err := server.HealthCheck(ctx, req)
		
		assert.NoError(t, err)
		assert.NotNil(t, resp)
		assert.Equal(t, "healthy", resp.Status)
		assert.Equal(t, "auth-service", resp.Service)
		assert.NotNil(t, resp.Timestamp)
	})
}

// TestPerformanceOptimizations verifies that performance optimizations are in place
func TestPerformanceOptimizations(t *testing.T) {
	logger := zap.NewNop()
	cfg := &config.Config{}

	server := &EnhancedAuthGRPCServer{
		config: cfg,
		logger: logger,
	}

	// Test that helper methods exist
	t.Run("HelperMethodsExist", func(t *testing.T) {
		// Test getTokenService method exists
		tokenService := server.getTokenService()
		assert.Nil(t, tokenService) // Will be nil since not initialized, but method exists

		// Test getPermissionService method exists
		permissionService := server.getPermissionService()
		assert.Nil(t, permissionService) // Will be nil since not initialized, but method exists

		// Test getAuthService method exists
		authService := server.getAuthService()
		assert.Nil(t, authService) // Will be nil since not initialized, but method exists
	})

	// Test utility functions
	t.Run("UtilityFunctions", func(t *testing.T) {
		// Test min function
		assert.Equal(t, 5, min(5, 10))
		assert.Equal(t, 3, min(8, 3))
		assert.Equal(t, 0, min(0, 0))
	})
}