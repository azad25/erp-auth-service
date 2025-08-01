package services

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest"

	"erp-auth-service/internal/models"
)

// BulkPermissionEvaluatorTestSuite defines the test suite for BulkPermissionEvaluator
type BulkPermissionEvaluatorTestSuite struct {
	suite.Suite
	evaluator        *BulkPermissionEvaluator
	permissionService *PermissionService
	mockPermRepo     *MockPermissionRepository
	mockRoleRepo     *MockRoleRepository
	mockUserRepo     *MockUserRepository
	mockCacheManager *MockCacheManager
	logger           *zap.Logger
	ctx              context.Context
}

// SetupTest sets up the test environment before each test
func (suite *BulkPermissionEvaluatorTestSuite) SetupTest() {
	suite.mockPermRepo = &MockPermissionRepository{}
	suite.mockRoleRepo = &MockRoleRepository{}
	suite.mockUserRepo = &MockUserRepository{}
	suite.mockCacheManager = &MockCacheManager{}
	suite.logger = zaptest.NewLogger(suite.T())
	suite.ctx = context.Background()

	config := PermissionServiceConfig{
		CacheTTL:              5 * time.Minute,
		BulkEvaluationEnabled: true,
		CacheWarmingEnabled:   false,
		MaxCacheSize:          1000,
		EvaluationTimeout:     5 * time.Second,
	}

	suite.permissionService = NewPermissionService(
		suite.mockPermRepo,
		suite.mockRoleRepo,
		suite.mockUserRepo,
		suite.mockCacheManager,
		suite.logger,
		config,
	)

	suite.evaluator = NewBulkPermissionEvaluator(suite.permissionService, suite.logger)
}

// TestEvaluateBulk_Success tests successful bulk evaluation
func (suite *BulkPermissionEvaluatorTestSuite) TestEvaluateBulk_Success() {
	userID := uuid.New()

	// Mock permissions
	permissions := []models.Permission{
		{
			ID:       uuid.New(),
			Name:     "user.read",
			Resource: "user",
			Action:   "read",
		},
		{
			ID:       uuid.New(),
			Name:     "user.write",
			Resource: "user",
			Action:   "write",
		},
	}

	// Create bulk request
	request := &BulkPermissionRequest{
		UserID: userID,
		Permissions: []PermissionCheckRequest{
			{Resource: "user", Action: "read"},
			{Resource: "user", Action: "write"},
			{Resource: "user", Action: "delete"}, // This should fail
		},
	}

	// Setup mocks - called multiple times for each permission check
	suite.mockPermRepo.On("GetUserPermissions", suite.ctx, userID).
		Return(permissions, nil)

	// Execute
	response, err := suite.evaluator.EvaluateBulk(suite.ctx, request)

	// Assert
	suite.NoError(err)
	suite.NotNil(response)
	suite.Equal(userID, response.UserID)
	suite.Len(response.Results, 3)

	// First two should be allowed
	suite.True(response.Results[0].Allowed)
	suite.True(response.Results[1].Allowed)
	// Third should be denied
	suite.False(response.Results[2].Allowed)
}

// TestEvaluateBulk_EmptyRequest tests bulk evaluation with empty request
func (suite *BulkPermissionEvaluatorTestSuite) TestEvaluateBulk_EmptyRequest() {
	userID := uuid.New()

	request := &BulkPermissionRequest{
		UserID:      userID,
		Permissions: []PermissionCheckRequest{},
	}

	// Execute
	response, err := suite.evaluator.EvaluateBulk(suite.ctx, request)

	// Assert
	suite.NoError(err)
	suite.NotNil(response)
	suite.Equal(userID, response.UserID)
	suite.Empty(response.Results)
}

// TestEvaluateBulk_ContextCancellation tests behavior when context is cancelled
func (suite *BulkPermissionEvaluatorTestSuite) TestEvaluateBulk_ContextCancellation() {
	userID := uuid.New()

	request := &BulkPermissionRequest{
		UserID: userID,
		Permissions: []PermissionCheckRequest{
			{Resource: "user", Action: "read"},
		},
	}

	// Create cancelled context
	cancelledCtx, cancel := context.WithCancel(suite.ctx)
	cancel()

	// Execute
	response, err := suite.evaluator.EvaluateBulk(cancelledCtx, request)

	// Assert
	suite.Error(err)
	suite.Nil(response)
	suite.Equal(context.Canceled, err)
}

// TestEvaluateBulkWithBatching tests bulk evaluation with batching
func (suite *BulkPermissionEvaluatorTestSuite) TestEvaluateBulkWithBatching() {
	userID := uuid.New()

	// Create large request that exceeds batch size
	permissions := make([]PermissionCheckRequest, 100)
	for i := 0; i < 100; i++ {
		permissions[i] = PermissionCheckRequest{
			Resource: "user",
			Action:   "read",
		}
	}

	request := &BulkPermissionRequest{
		UserID:      userID,
		Permissions: permissions,
	}

	// Mock permission that allows all requests
	mockPermission := models.Permission{
		ID:       uuid.New(),
		Name:     "user.read",
		Resource: "user",
		Action:   "read",
	}

	// Setup mocks - will be called multiple times due to batching
	suite.mockPermRepo.On("GetUserPermissions", suite.ctx, userID).
		Return([]models.Permission{mockPermission}, nil)

	// Execute
	response, err := suite.evaluator.EvaluateBulkWithBatching(suite.ctx, request)

	// Assert
	suite.NoError(err)
	suite.NotNil(response)
	suite.Equal(userID, response.UserID)
	suite.Len(response.Results, 100)

	// All should be allowed
	for _, result := range response.Results {
		suite.True(result.Allowed)
	}
}

// TestOptimizedBulkEvaluate tests optimized bulk evaluation with deduplication
func (suite *BulkPermissionEvaluatorTestSuite) TestOptimizedBulkEvaluate() {
	userID := uuid.New()

	// Create request with duplicate permissions
	request := &BulkPermissionRequest{
		UserID: userID,
		Permissions: []PermissionCheckRequest{
			{Resource: "user", Action: "read"},
			{Resource: "user", Action: "write"},
			{Resource: "user", Action: "read"}, // Duplicate
			{Resource: "user", Action: "write"}, // Duplicate
			{Resource: "user", Action: "read"}, // Duplicate
		},
	}

	// Mock permissions
	mockPermissions := []models.Permission{
		{
			ID:       uuid.New(),
			Name:     "user.read",
			Resource: "user",
			Action:   "read",
		},
		{
			ID:       uuid.New(),
			Name:     "user.write",
			Resource: "user",
			Action:   "write",
		},
	}

	// Setup mocks - should be called fewer times due to deduplication
	suite.mockPermRepo.On("GetUserPermissions", suite.ctx, userID).
		Return(mockPermissions, nil)

	// Execute
	response, err := suite.evaluator.OptimizedBulkEvaluate(suite.ctx, request)

	// Assert
	suite.NoError(err)
	suite.NotNil(response)
	suite.Equal(userID, response.UserID)
	suite.Len(response.Results, 5) // Original request size

	// All should be allowed and properly mapped
	for _, result := range response.Results {
		suite.True(result.Allowed)
	}
}

// TestDeduplicateRequests tests request deduplication logic
func (suite *BulkPermissionEvaluatorTestSuite) TestDeduplicateRequests() {
	requests := []PermissionCheckRequest{
		{Resource: "user", Action: "read", Scope: ""},
		{Resource: "user", Action: "write", Scope: ""},
		{Resource: "user", Action: "read", Scope: ""}, // Duplicate
		{Resource: "role", Action: "read", Scope: ""},
		{Resource: "user", Action: "read", Scope: ""}, // Duplicate
	}

	unique, indexMap := suite.evaluator.deduplicateRequests(requests)

	// Should have 3 unique requests
	suite.Len(unique, 3)
	suite.Len(indexMap, 5) // Original length

	// Check mapping
	suite.Equal(0, indexMap[0]) // user.read -> index 0
	suite.Equal(1, indexMap[1]) // user.write -> index 1
	suite.Equal(0, indexMap[2]) // user.read (duplicate) -> index 0
	suite.Equal(2, indexMap[3]) // role.read -> index 2
	suite.Equal(0, indexMap[4]) // user.read (duplicate) -> index 0
}

// TestGetStats tests statistics retrieval
func (suite *BulkPermissionEvaluatorTestSuite) TestGetStats() {
	stats := suite.evaluator.GetStats()

	suite.NotNil(stats)
	suite.Contains(stats, "max_workers")
	suite.Contains(stats, "batch_size")
	suite.Contains(stats, "evaluation_timeout")
	suite.Contains(stats, "worker_pool_stats")

	// Verify types
	suite.IsType(int(0), stats["max_workers"])
	suite.IsType(int(0), stats["batch_size"])
	suite.IsType("", stats["evaluation_timeout"])
}

// Benchmark tests

// BenchmarkEvaluateBulk benchmarks bulk evaluation performance
func BenchmarkEvaluateBulk(b *testing.B) {
	suite := &BulkPermissionEvaluatorTestSuite{}
	suite.SetupTest()

	userID := uuid.New()

	// Create request with 10 permissions
	request := &BulkPermissionRequest{
		UserID: userID,
		Permissions: make([]PermissionCheckRequest, 10),
	}

	for i := 0; i < 10; i++ {
		request.Permissions[i] = PermissionCheckRequest{
			Resource: "user",
			Action:   "read",
		}
	}

	// Mock permission
	mockPermission := models.Permission{
		ID:       uuid.New(),
		Name:     "user.read",
		Resource: "user",
		Action:   "read",
	}

	suite.mockPermRepo.On("GetUserPermissions", context.Background(), userID).
		Return([]models.Permission{mockPermission}, nil)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := suite.evaluator.EvaluateBulk(context.Background(), request)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkOptimizedBulkEvaluate benchmarks optimized bulk evaluation
func BenchmarkOptimizedBulkEvaluate(b *testing.B) {
	suite := &BulkPermissionEvaluatorTestSuite{}
	suite.SetupTest()

	userID := uuid.New()

	// Create request with many duplicate permissions
	request := &BulkPermissionRequest{
		UserID: userID,
		Permissions: make([]PermissionCheckRequest, 50),
	}

	// Fill with duplicates (only 5 unique permissions)
	actions := []string{"read", "write", "delete", "create", "update"}
	for i := 0; i < 50; i++ {
		request.Permissions[i] = PermissionCheckRequest{
			Resource: "user",
			Action:   actions[i%5],
		}
	}

	// Mock permissions
	mockPermissions := make([]models.Permission, 5)
	for i, action := range actions {
		mockPermissions[i] = models.Permission{
			ID:       uuid.New(),
			Name:     "user." + action,
			Resource: "user",
			Action:   action,
		}
	}

	suite.mockPermRepo.On("GetUserPermissions", context.Background(), userID).
		Return(mockPermissions, nil)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := suite.evaluator.OptimizedBulkEvaluate(context.Background(), request)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// Test concurrent bulk evaluation
func (suite *BulkPermissionEvaluatorTestSuite) TestConcurrentBulkEvaluation() {
	userID := uuid.New()

	request := &BulkPermissionRequest{
		UserID: userID,
		Permissions: []PermissionCheckRequest{
			{Resource: "user", Action: "read"},
			{Resource: "user", Action: "write"},
		},
	}

	// Mock permissions
	mockPermissions := []models.Permission{
		{
			ID:       uuid.New(),
			Name:     "user.read",
			Resource: "user",
			Action:   "read",
		},
		{
			ID:       uuid.New(),
			Name:     "user.write",
			Resource: "user",
			Action:   "write",
		},
	}

	suite.mockPermRepo.On("GetUserPermissions", suite.ctx, userID).
		Return(mockPermissions, nil)

	// Run multiple concurrent evaluations
	const numGoroutines = 10
	results := make(chan *BulkPermissionResponse, numGoroutines)
	errors := make(chan error, numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		go func() {
			response, err := suite.evaluator.EvaluateBulk(suite.ctx, request)
			if err != nil {
				errors <- err
			} else {
				results <- response
			}
		}()
	}

	// Collect results
	for i := 0; i < numGoroutines; i++ {
		select {
		case response := <-results:
			suite.NotNil(response)
			suite.Equal(userID, response.UserID)
			suite.Len(response.Results, 2)
		case err := <-errors:
			suite.Fail("Unexpected error: %v", err)
		case <-time.After(5 * time.Second):
			suite.Fail("Timeout waiting for results")
		}
	}
}

// Run the test suite
func TestBulkPermissionEvaluatorTestSuite(t *testing.T) {
	suite.Run(t, new(BulkPermissionEvaluatorTestSuite))
}

// Test error handling in bulk evaluation
func (suite *BulkPermissionEvaluatorTestSuite) TestEvaluateBulk_ErrorHandling() {
	userID := uuid.New()

	request := &BulkPermissionRequest{
		UserID: userID,
		Permissions: []PermissionCheckRequest{
			{Resource: "user", Action: "read"},
		},
	}

	// Mock repository error
	suite.mockPermRepo.On("GetUserPermissions", suite.ctx, userID).
		Return(nil, assert.AnError)

	// Execute
	response, err := suite.evaluator.EvaluateBulk(suite.ctx, request)

	// Assert
	suite.NoError(err) // Bulk evaluator should handle individual errors gracefully
	suite.NotNil(response)
	suite.Len(response.Results, 1)
	suite.False(response.Results[0].Allowed)
	suite.Contains(response.Results[0].Reason, "evaluation error")
}

// Test timeout handling
func (suite *BulkPermissionEvaluatorTestSuite) TestEvaluateBulk_Timeout() {
	// Set a very short timeout for the evaluator
	suite.evaluator.evaluationTimeout = 1 * time.Millisecond

	userID := uuid.New()

	request := &BulkPermissionRequest{
		UserID: userID,
		Permissions: []PermissionCheckRequest{
			{Resource: "user", Action: "read"},
		},
	}

	// Mock slow repository response
	suite.mockPermRepo.On("GetUserPermissions", suite.ctx, userID).
		After(100*time.Millisecond).Return([]models.Permission{}, nil)

	// Execute
	response, err := suite.evaluator.EvaluateBulk(suite.ctx, request)

	// Assert - should complete but may have timeout results
	suite.NoError(err)
	suite.NotNil(response)
	suite.Len(response.Results, 1)
}