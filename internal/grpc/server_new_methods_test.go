package grpc

import (
	"context"
	"testing"

	"erp-auth-service/internal/config"
	pb "erp-auth-service/proto"

	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"
)

// TestNewGRPCMethods verifies that the new gRPC methods are implemented and handle validation correctly
func TestNewGRPCMethods(t *testing.T) {
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

	// Test CreateUser method validation
	t.Run("CreateUser_ValidationErrors", func(t *testing.T) {
		// Test empty request
		req := &pb.CreateUserRequest{}
		resp, err := server.CreateUser(ctx, req)
		
		assert.NoError(t, err)
		assert.NotNil(t, resp)
		assert.False(t, resp.Success)
		assert.Contains(t, resp.Error, "required")
		
		// Test invalid organization ID
		req = &pb.CreateUserRequest{
			OrganizationId: "invalid-uuid",
			Email:          "test@example.com",
			Password:       "password123",
			FirstName:      "Test",
			LastName:       "User",
		}
		resp, err = server.CreateUser(ctx, req)
		
		assert.NoError(t, err)
		assert.NotNil(t, resp)
		assert.False(t, resp.Success)
		assert.Contains(t, resp.Error, "Invalid organization ID format")
	})

	// Test UpdateUser method validation
	t.Run("UpdateUser_ValidationErrors", func(t *testing.T) {
		// Test empty request
		req := &pb.UpdateUserRequest{}
		resp, err := server.UpdateUser(ctx, req)
		
		assert.NoError(t, err)
		assert.NotNil(t, resp)
		assert.False(t, resp.Success)
		assert.Contains(t, resp.Error, "User ID is required")
		
		// Test invalid user ID
		req = &pb.UpdateUserRequest{
			UserId: "invalid-uuid",
		}
		resp, err = server.UpdateUser(ctx, req)
		
		assert.NoError(t, err)
		assert.NotNil(t, resp)
		assert.False(t, resp.Success)
		assert.Contains(t, resp.Error, "Invalid user ID format")
	})

	// Test ChangePassword method validation
	t.Run("ChangePassword_ValidationErrors", func(t *testing.T) {
		// Test empty request
		req := &pb.ChangePasswordRequest{}
		resp, err := server.ChangePassword(ctx, req)
		
		assert.NoError(t, err)
		assert.NotNil(t, resp)
		assert.False(t, resp.Success)
		assert.Contains(t, resp.Error, "required")
		
		// Test invalid user ID
		req = &pb.ChangePasswordRequest{
			UserId:          "invalid-uuid",
			CurrentPassword: "old",
			NewPassword:     "new",
		}
		resp, err = server.ChangePassword(ctx, req)
		
		assert.NoError(t, err)
		assert.NotNil(t, resp)
		assert.False(t, resp.Success)
		assert.Contains(t, resp.Error, "Invalid user ID format")
	})

	// Test CreateOrganization method validation
	t.Run("CreateOrganization_ValidationErrors", func(t *testing.T) {
		// Test empty request
		req := &pb.CreateOrganizationRequest{}
		resp, err := server.CreateOrganization(ctx, req)
		
		assert.NoError(t, err)
		assert.NotNil(t, resp)
		assert.False(t, resp.Success)
		assert.Contains(t, resp.Error, "required")
	})

	// Test GetOrganization method validation
	t.Run("GetOrganization_ValidationErrors", func(t *testing.T) {
		// Test empty request
		req := &pb.GetOrganizationRequest{}
		resp, err := server.GetOrganization(ctx, req)
		
		assert.NoError(t, err)
		assert.NotNil(t, resp)
		assert.Contains(t, resp.Error, "Organization ID is required")
		
		// Test invalid organization ID
		req = &pb.GetOrganizationRequest{
			OrganizationId: "invalid-uuid",
		}
		resp, err = server.GetOrganization(ctx, req)
		
		assert.NoError(t, err)
		assert.NotNil(t, resp)
		assert.Contains(t, resp.Error, "Invalid organization ID format")
	})

	// Test BulkCreateUsers method validation
	t.Run("BulkCreateUsers_ValidationErrors", func(t *testing.T) {
		// Test empty request
		req := &pb.BulkCreateUsersRequest{}
		resp, err := server.BulkCreateUsers(ctx, req)
		
		assert.NoError(t, err)
		assert.NotNil(t, resp)
		assert.False(t, resp.Success)
		assert.Contains(t, resp.Error, "required")
		
		// Test invalid organization ID
		req = &pb.BulkCreateUsersRequest{
			OrganizationId: "invalid-uuid",
			Users: []*pb.CreateUserData{
				{Email: "test@example.com"},
			},
		}
		resp, err = server.BulkCreateUsers(ctx, req)
		
		assert.NoError(t, err)
		assert.NotNil(t, resp)
		assert.False(t, resp.Success)
		assert.Contains(t, resp.Error, "Invalid organization ID format")
	})

	// Test BulkUpdateUsers method validation
	t.Run("BulkUpdateUsers_ValidationErrors", func(t *testing.T) {
		// Test empty request
		req := &pb.BulkUpdateUsersRequest{}
		resp, err := server.BulkUpdateUsers(ctx, req)
		
		assert.NoError(t, err)
		assert.NotNil(t, resp)
		assert.False(t, resp.Success)
		assert.Contains(t, resp.Error, "required")
	})

	// Test BulkCheckPermissions method validation
	t.Run("BulkCheckPermissions_ValidationErrors", func(t *testing.T) {
		// Test empty request
		req := &pb.BulkCheckPermissionsRequest{}
		resp, err := server.BulkCheckPermissions(ctx, req)
		
		assert.NoError(t, err)
		assert.NotNil(t, resp)
		assert.False(t, resp.Success)
		assert.Contains(t, resp.Error, "required")
	})
}