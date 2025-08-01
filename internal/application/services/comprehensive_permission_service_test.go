package services

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"
	"go.uber.org/zap/zaptest"

	"erp-auth-service/internal/cache"
	"erp-auth-service/internal/domain/entities"
	"erp-auth-service/internal/models"
)

// ComprehensivePermissionServiceTestSuite provides comprehensive test coverage for PermissionService
type ComprehensivePermissionServiceTestSuite struct {
	suite.Suite
	permissionService *PermissionService
	mockPermRepo      *MockPermissionRepository
	mockRoleRepo      *MockRoleRepository
	mockUserRepo      *MockUserRepository
	mockCache         *MockCacheManager
	config            PermissionServiceConfig
	ctx               context.Context
}

// SetupTest sets up the test suite
func (suite *ComprehensivePermissionServiceTestSuite) SetupTest() {
	suite.ctx = context.Background()
	suite.mockPermRepo = new(MockPermissionRepository)
	suite.mockRoleRepo = new(MockRoleRepository)
	suite.mockUserRepo = new(MockUserRepository)
	suite.mockCache = NewMockCacheManager()
	
	suite.config = PermissionServiceConfig{
		CacheTTL:              5 * time.Minute,
		BulkEvaluationEnabled: true,
		CacheWarmingEnabled:   false, // Disable for testing
		MaxCacheSize:          1000,
		EvaluationTimeout:     10 * time.Second,
	}
	
	logger := zaptest.NewLogger(suite.T())
	
	// Setup default mock expectations for cache operations
	suite.mockCache.On("Get", mock.Anything, mock.AnythingOfType("string")).Return(nil, cache.ErrCacheNotFound).Maybe()
	suite.mockCache.On("Set", mock.Anything, mock.AnythingOfType("string"), mock.Anything, mock.AnythingOfType("time.Duration")).Return(nil).Maybe()
	suite.mockCache.On("Delete", mock.Anything, mock.AnythingOfType("string")).Return(nil).Maybe()
	suite.mockCache.On("InvalidatePattern", mock.Anything, mock.AnythingOfType("string")).Return(nil).Maybe()
	
	suite.permissionService = NewPermissionService(
		suite.mockPermRepo,
		suite.mockRoleRepo,
		suite.mockUserRepo,
		suite.mockCache,
		logger,
		suite.config,
	)
}

// TearDownTest cleans up after each test
func (suite *ComprehensivePermissionServiceTestSuite) TearDownTest() {
	if suite.permissionService != nil {
		suite.permissionService.Close()
	}
}

// TestCheckUserPermission_Success tests successful permission check
func (suite *ComprehensivePermissionServiceTestSuite) TestCheckUserPermission_Success() {
	// Arrange
	userID := uuid.New()
	resource := "users"
	action := "read"
	
	permissions := []models.Permission{
		{
			ID:       uuid.New(),
			Name:     "users.read",
			Resource: "users",
			Action:   "read",
			Scope:    "",
		},
	}
	
	// Mock expectations
	suite.mockPermRepo.On("GetUserPermissions", suite.ctx, userID).Return(permissions, nil)
	
	// Act
	hasPermission, err := suite.permissionService.CheckUserPermission(suite.ctx, userID, resource, action)
	
	// Assert
	suite.NoError(err)
	suite.True(hasPermission)
	
	// Verify expectations
	suite.mockPermRepo.AssertExpectations(suite.T())
}

// TestCheckUserPermission_Failure tests permission check failure
func (suite *ComprehensivePermissionServiceTestSuite) TestCheckUserPermission_Failure() {
	// Arrange
	userID := uuid.New()
	resource := "users"
	action := "delete"
	
	permissions := []models.Permission{
		{
			ID:       uuid.New(),
			Name:     "users.read",
			Resource: "users",
			Action:   "read",
			Scope:    "",
		},
	}
	
	// Mock expectations
	suite.mockPermRepo.On("GetUserPermissions", suite.ctx, userID).Return(permissions, nil)
	
	// Act
	hasPermission, err := suite.permissionService.CheckUserPermission(suite.ctx, userID, resource, action)
	
	// Assert
	suite.NoError(err)
	suite.False(hasPermission)
	
	// Verify expectations
	suite.mockPermRepo.AssertExpectations(suite.T())
}

// TestCheckUserPermissionWithScope_Success tests successful permission check with scope
func (suite *ComprehensivePermissionServiceTestSuite) TestCheckUserPermissionWithScope_Success() {
	// Arrange
	userID := uuid.New()
	resource := "documents"
	action := "read"
	scope := "organization"
	
	permissions := []models.Permission{
		{
			ID:       uuid.New(),
			Name:     "documents.read.organization",
			Resource: "documents",
			Action:   "read",
			Scope:    "organization",
		},
	}
	
	// Mock expectations
	suite.mockPermRepo.On("GetUserPermissions", suite.ctx, userID).Return(permissions, nil)
	
	// Act
	hasPermission, err := suite.permissionService.CheckUserPermissionWithScope(suite.ctx, userID, resource, action, scope)
	
	// Assert
	suite.NoError(err)
	suite.True(hasPermission)
	
	// Verify expectations
	suite.mockPermRepo.AssertExpectations(suite.T())
}

// TestCheckUserPermissionWithScope_WildcardMatch tests wildcard permission matching
func (suite *ComprehensivePermissionServiceTestSuite) TestCheckUserPermissionWithScope_WildcardMatch() {
	// Arrange
	userID := uuid.New()
	resource := "documents"
	action := "read"
	scope := "organization"
	
	permissions := []models.Permission{
		{
			ID:       uuid.New(),
			Name:     "documents.*",
			Resource: "documents",
			Action:   "*", // Wildcard action
			Scope:    "",
		},
	}
	
	// Mock expectations
	suite.mockPermRepo.On("GetUserPermissions", suite.ctx, userID).Return(permissions, nil)
	
	// Act
	hasPermission, err := suite.permissionService.CheckUserPermissionWithScope(suite.ctx, userID, resource, action, scope)
	
	// Assert
	suite.NoError(err)
	suite.True(hasPermission)
	
	// Verify expectations
	suite.mockPermRepo.AssertExpectations(suite.T())
}

// TestCheckUserPermissionWithScope_HierarchicalMatch tests hierarchical permission matching
func (suite *ComprehensivePermissionServiceTestSuite) TestCheckUserPermissionWithScope_HierarchicalMatch() {
	// Arrange
	userID := uuid.New()
	resource := "documents"
	action := "read"
	scope := "organization"
	
	parentPermission := models.Permission{
		ID:       uuid.New(),
		Name:     "documents.admin",
		Resource: "documents",
		Action:   "admin",
		Scope:    "",
	}
	
	childPermissions := []models.Permission{
		{
			ID:       uuid.New(),
			Name:     "documents.read",
			Resource: "documents",
			Action:   "read",
			Scope:    "",
			ParentID: &parentPermission.ID,
		},
	}
	
	// Mock expectations
	suite.mockPermRepo.On("GetUserPermissions", suite.ctx, userID).Return([]models.Permission{parentPermission}, nil)
	suite.mockPermRepo.On("GetChildPermissions", suite.ctx, parentPermission.ID).Return(childPermissions, nil)
	
	// Act
	hasPermission, err := suite.permissionService.CheckUserPermissionWithScope(suite.ctx, userID, resource, action, scope)
	
	// Assert
	suite.NoError(err)
	suite.True(hasPermission)
	
	// Verify expectations
	suite.mockPermRepo.AssertExpectations(suite.T())
}

// TestCheckBulkPermissions_Success tests successful bulk permission checking
func (suite *ComprehensivePermissionServiceTestSuite) TestCheckBulkPermissions_Success() {
	// Arrange
	userID := uuid.New()
	request := &BulkPermissionRequest{
		UserID: userID,
		Permissions: []PermissionCheckRequest{
			{Resource: "users", Action: "read"},
			{Resource: "users", Action: "write"},
			{Resource: "documents", Action: "read"},
		},
	}
	
	permissions := []models.Permission{
		{
			ID:       uuid.New(),
			Name:     "users.read",
			Resource: "users",
			Action:   "read",
		},
		{
			ID:       uuid.New(),
			Name:     "documents.read",
			Resource: "documents",
			Action:   "read",
		},
	}
	
	// Mock expectations for bulk evaluation
	suite.mockPermRepo.On("GetUserPermissions", suite.ctx, userID).Return(permissions, nil).Times(3)
	
	// Act
	response, err := suite.permissionService.CheckBulkPermissions(suite.ctx, request)
	
	// Assert
	suite.NoError(err)
	suite.NotNil(response)
	suite.Equal(userID, response.UserID)
	suite.Len(response.Results, 3)
	
	// Check individual results
	suite.True(response.Results[0].Allowed)   // users.read - should be allowed
	suite.False(response.Results[1].Allowed)  // users.write - should be denied
	suite.True(response.Results[2].Allowed)   // documents.read - should be allowed
	
	// Verify expectations
	suite.mockPermRepo.AssertExpectations(suite.T())
}

// TestCheckBulkPermissions_Sequential tests sequential bulk permission checking
func (suite *ComprehensivePermissionServiceTestSuite) TestCheckBulkPermissions_Sequential() {
	// Arrange - disable bulk evaluation
	suite.config.BulkEvaluationEnabled = false
	suite.permissionService = NewPermissionService(
		suite.mockPermRepo,
		suite.mockRoleRepo,
		suite.mockUserRepo,
		suite.mockCache,
		zaptest.NewLogger(suite.T()),
		suite.config,
	)
	
	userID := uuid.New()
	request := &BulkPermissionRequest{
		UserID: userID,
		Permissions: []PermissionCheckRequest{
			{Resource: "users", Action: "read"},
		},
	}
	
	permissions := []models.Permission{
		{
			ID:       uuid.New(),
			Name:     "users.read",
			Resource: "users",
			Action:   "read",
		},
	}
	
	// Mock expectations
	suite.mockPermRepo.On("GetUserPermissions", suite.ctx, userID).Return(permissions, nil)
	
	// Act
	response, err := suite.permissionService.CheckBulkPermissions(suite.ctx, request)
	
	// Assert
	suite.NoError(err)
	suite.NotNil(response)
	suite.Equal(userID, response.UserID)
	suite.Len(response.Results, 1)
	suite.True(response.Results[0].Allowed)
	
	// Verify expectations
	suite.mockPermRepo.AssertExpectations(suite.T())
}

// TestGetUserEffectivePermissions_Success tests getting user effective permissions
func (suite *ComprehensivePermissionServiceTestSuite) TestGetUserEffectivePermissions_Success() {
	// Arrange
	userID := uuid.New()
	
	permissions := []models.Permission{
		{
			ID:       uuid.New(),
			Name:     "users.read",
			Resource: "users",
			Action:   "read",
		},
		{
			ID:       uuid.New(),
			Name:     "documents.write",
			Resource: "documents",
			Action:   "write",
		},
	}
	
	// Mock expectations
	suite.mockPermRepo.On("GetUserPermissions", suite.ctx, userID).Return(permissions, nil)
	suite.mockPermRepo.On("GetPermissionHierarchy", suite.ctx, permissions[0].ID).Return([]models.Permission{permissions[0]}, nil)
	suite.mockPermRepo.On("GetPermissionHierarchy", suite.ctx, permissions[1].ID).Return([]models.Permission{permissions[1]}, nil)
	
	// Act
	effectivePermissions, err := suite.permissionService.GetUserEffectivePermissions(suite.ctx, userID)
	
	// Assert
	suite.NoError(err)
	suite.Len(effectivePermissions, 2)
	suite.Equal("users.read", effectivePermissions[0].Name)
	suite.Equal("documents.write", effectivePermissions[1].Name)
	
	// Verify expectations
	suite.mockPermRepo.AssertExpectations(suite.T())
}

// TestGetPermissionHierarchy_RootPermissions tests getting root permissions
func (suite *ComprehensivePermissionServiceTestSuite) TestGetPermissionHierarchy_RootPermissions() {
	// Arrange
	rootPermissions := []models.Permission{
		{
			ID:       uuid.New(),
			Name:     "admin",
			Resource: "system",
			Action:   "admin",
			ParentID: nil,
		},
		{
			ID:       uuid.New(),
			Name:     "user",
			Resource: "system",
			Action:   "user",
			ParentID: nil,
		},
	}
	
	// Mock expectations
	suite.mockPermRepo.On("GetPermissionsByResource", suite.ctx, "").Return(rootPermissions, nil)
	
	// Act
	hierarchy, err := suite.permissionService.GetPermissionHierarchy(suite.ctx, nil)
	
	// Assert
	suite.NoError(err)
	suite.Len(hierarchy, 2)
	
	// Verify expectations
	suite.mockPermRepo.AssertExpectations(suite.T())
}

// TestGetPermissionHierarchy_WithParent tests getting permission hierarchy with parent
func (suite *ComprehensivePermissionServiceTestSuite) TestGetPermissionHierarchy_WithParent() {
	// Arrange
	parentID := uuid.New()
	hierarchyPermissions := []models.Permission{
		{
			ID:       uuid.New(),
			Name:     "users.read",
			Resource: "users",
			Action:   "read",
			ParentID: &parentID,
		},
		{
			ID:       uuid.New(),
			Name:     "users.write",
			Resource: "users",
			Action:   "write",
			ParentID: &parentID,
		},
	}
	
	// Mock expectations
	suite.mockPermRepo.On("GetPermissionHierarchy", suite.ctx, parentID).Return(hierarchyPermissions, nil)
	
	// Act
	hierarchy, err := suite.permissionService.GetPermissionHierarchy(suite.ctx, &parentID)
	
	// Assert
	suite.NoError(err)
	suite.Len(hierarchy, 2)
	
	// Verify expectations
	suite.mockPermRepo.AssertExpectations(suite.T())
}

// TestInvalidateUserPermissionCache tests cache invalidation
func (suite *ComprehensivePermissionServiceTestSuite) TestInvalidateUserPermissionCache() {
	// Arrange
	userID := uuid.New()
	expectedPattern := fmt.Sprintf("user:permissions:*:%s", userID.String())
	
	// Mock expectations
	suite.mockCache.On("InvalidatePattern", suite.ctx, expectedPattern).Return(nil)
	
	// Act
	err := suite.permissionService.InvalidateUserPermissionCache(suite.ctx, userID)
	
	// Assert
	suite.NoError(err)
	
	// Verify expectations
	suite.mockCache.AssertExpectations(suite.T())
}

// TestWildcardMatching tests wildcard permission matching
func (suite *ComprehensivePermissionServiceTestSuite) TestWildcardMatching() {
	testCases := []struct {
		name     string
		pattern  string
		value    string
		expected bool
	}{
		{"exact match", "users", "users", true},
		{"wildcard all", "*", "anything", true},
		{"prefix wildcard", "users.*", "users.read", true},
		{"prefix wildcard no match", "users.*", "documents.read", false},
		{"suffix wildcard", "*.read", "users.read", true},
		{"suffix wildcard no match", "*.read", "users.write", false},
		{"no wildcard no match", "users", "documents", false},
	}
	
	for _, tc := range testCases {
		suite.Run(tc.name, func() {
			result := suite.permissionService.matchesWildcard(tc.pattern, tc.value)
			suite.Equal(tc.expected, result)
		})
	}
}

// TestPermissionMatching tests permission matching logic
func (suite *ComprehensivePermissionServiceTestSuite) TestPermissionMatching() {
	testCases := []struct {
		name       string
		permission models.Permission
		resource   string
		action     string
		scope      string
		expected   bool
	}{
		{
			name: "exact match",
			permission: models.Permission{
				Resource: "users",
				Action:   "read",
				Scope:    "organization",
			},
			resource: "users",
			action:   "read",
			scope:    "organization",
			expected: true,
		},
		{
			name: "wildcard action match",
			permission: models.Permission{
				Resource: "users",
				Action:   "*",
				Scope:    "",
			},
			resource: "users",
			action:   "read",
			scope:    "",
			expected: true,
		},
		{
			name: "no match different resource",
			permission: models.Permission{
				Resource: "users",
				Action:   "read",
				Scope:    "",
			},
			resource: "documents",
			action:   "read",
			scope:    "",
			expected: false,
		},
		{
			name: "scope mismatch",
			permission: models.Permission{
				Resource: "users",
				Action:   "read",
				Scope:    "organization",
			},
			resource: "users",
			action:   "read",
			scope:    "global",
			expected: false,
		},
		{
			name: "empty scope matches any",
			permission: models.Permission{
				Resource: "users",
				Action:   "read",
				Scope:    "",
			},
			resource: "users",
			action:   "read",
			scope:    "organization",
			expected: true,
		},
	}
	
	for _, tc := range testCases {
		suite.Run(tc.name, func() {
			result := suite.permissionService.matchesPermission(&tc.permission, tc.resource, tc.action, tc.scope)
			suite.Equal(tc.expected, result)
		})
	}
}

// TestModelToEntity tests model to entity conversion
func (suite *ComprehensivePermissionServiceTestSuite) TestModelToEntity() {
	// Arrange
	parentID := uuid.New()
	model := &models.Permission{
		ID:          uuid.New(),
		Name:        "users.read",
		Resource:    "users",
		Action:      "read",
		Scope:       "organization",
		ParentID:    &parentID,
		Description: "Read users permission",
		IsSystem:    true,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}
	
	// Act
	entity := suite.permissionService.modelToEntity(model)
	
	// Assert
	suite.Equal(model.ID, entity.ID)
	suite.Equal(model.Name, entity.Name)
	suite.Equal(model.Resource, entity.Resource)
	suite.Equal(model.Action, entity.Action)
	suite.Equal(model.Scope, entity.Scope)
	suite.Equal(model.ParentID, entity.ParentID)
	suite.Equal(model.Description, entity.Description)
	suite.Equal(model.IsSystem, entity.IsSystem)
	suite.Equal(model.CreatedAt, entity.CreatedAt)
	suite.Equal(model.UpdatedAt, entity.UpdatedAt)
}

// TestDeduplicatePermissions tests permission deduplication
func (suite *ComprehensivePermissionServiceTestSuite) TestDeduplicatePermissions() {
	// Arrange
	id1 := uuid.New()
	id2 := uuid.New()
	
	permissions := []entities.Permission{
		{ID: id1, Name: "users.read"},
		{ID: id2, Name: "users.write"},
		{ID: id1, Name: "users.read"}, // Duplicate
		{ID: id2, Name: "users.write"}, // Duplicate
	}
	
	// Act
	deduplicated := suite.permissionService.deduplicatePermissions(permissions)
	
	// Assert
	suite.Len(deduplicated, 2)
	
	// Verify unique IDs
	ids := make(map[uuid.UUID]bool)
	for _, perm := range deduplicated {
		suite.False(ids[perm.ID], "Found duplicate permission ID")
		ids[perm.ID] = true
	}
}

// TestBuildPermissionCacheKey tests cache key building
func (suite *ComprehensivePermissionServiceTestSuite) TestBuildPermissionCacheKey() {
	// Arrange
	userID := uuid.New()
	resource := "users"
	action := "read"
	scope := "organization"
	
	// Act
	cacheKey := suite.permissionService.buildPermissionCacheKey(userID, resource, action, scope)
	
	// Assert
	expectedKey := fmt.Sprintf("user:permission:%s:%s:%s:%s", userID.String(), resource, action, scope)
	suite.Equal(expectedKey, cacheKey)
}

// TestEvaluationTimeout tests permission evaluation timeout
func (suite *ComprehensivePermissionServiceTestSuite) TestEvaluationTimeout() {
	// Arrange - set very short timeout
	suite.config.EvaluationTimeout = 1 * time.Millisecond
	suite.permissionService = NewPermissionService(
		suite.mockPermRepo,
		suite.mockRoleRepo,
		suite.mockUserRepo,
		suite.mockCache,
		zaptest.NewLogger(suite.T()),
		suite.config,
	)
	
	userID := uuid.New()
	resource := "users"
	action := "read"
	
	// Mock expectations with delay
	suite.mockPermRepo.On("GetUserPermissions", mock.Anything, userID).Return([]models.Permission{}, nil).Run(func(args mock.Arguments) {
		time.Sleep(10 * time.Millisecond) // Longer than timeout
	})
	
	// Act
	hasPermission, err := suite.permissionService.CheckUserPermission(suite.ctx, userID, resource, action)
	
	// Assert - should handle timeout gracefully
	suite.NoError(err) // The implementation should handle timeout gracefully
	suite.False(hasPermission)
}

// TestConcurrentPermissionChecks tests thread safety
func (suite *ComprehensivePermissionServiceTestSuite) TestConcurrentPermissionChecks() {
	// Arrange
	userID := uuid.New()
	resource := "users"
	action := "read"
	
	permissions := []models.Permission{
		{
			ID:       uuid.New(),
			Name:     "users.read",
			Resource: "users",
			Action:   "read",
		},
	}
	
	// Mock expectations for concurrent operations
	suite.mockPermRepo.On("GetUserPermissions", suite.ctx, userID).Return(permissions, nil)
	
	// Run concurrent permission checks
	const numGoroutines = 10
	results := make(chan bool, numGoroutines)
	errors := make(chan error, numGoroutines)
	
	for i := 0; i < numGoroutines; i++ {
		go func() {
			hasPermission, err := suite.permissionService.CheckUserPermission(suite.ctx, userID, resource, action)
			results <- hasPermission
			errors <- err
		}()
	}
	
	// Collect results
	for i := 0; i < numGoroutines; i++ {
		hasPermission := <-results
		err := <-errors
		suite.NoError(err)
		suite.True(hasPermission)
	}
}

// TestErrorHandling tests error handling in permission service
func (suite *ComprehensivePermissionServiceTestSuite) TestErrorHandling() {
	// Arrange
	userID := uuid.New()
	resource := "users"
	action := "read"
	
	// Mock expectations with error
	suite.mockPermRepo.On("GetUserPermissions", suite.ctx, userID).Return(nil, assert.AnError)
	
	// Act
	hasPermission, err := suite.permissionService.CheckUserPermission(suite.ctx, userID, resource, action)
	
	// Assert
	suite.Error(err)
	suite.False(hasPermission)
	suite.Contains(err.Error(), "failed to get user permissions")
	
	// Verify expectations
	suite.mockPermRepo.AssertExpectations(suite.T())
}

// Run the comprehensive permission service test suite
func TestComprehensivePermissionService(t *testing.T) {
	suite.Run(t, new(ComprehensivePermissionServiceTestSuite))
}