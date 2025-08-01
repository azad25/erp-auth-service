package services

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest"

	"erp-auth-service/internal/cache"
	"erp-auth-service/internal/domain/entities"
	"erp-auth-service/internal/models"
)

// PermissionServiceTestSuite defines the test suite for PermissionService
type PermissionServiceTestSuite struct {
	suite.Suite
	service          *PermissionService
	mockPermRepo     *MockPermissionRepository
	mockRoleRepo     *MockRoleRepository
	mockUserRepo     *MockUserRepository
	mockCacheManager *MockCacheManager
	logger           *zap.Logger
	ctx              context.Context
}

// SetupTest sets up the test environment before each test
func (suite *PermissionServiceTestSuite) SetupTest() {
	suite.mockPermRepo = &MockPermissionRepository{}
	suite.mockRoleRepo = &MockRoleRepository{}
	suite.mockUserRepo = &MockUserRepository{}
	suite.mockCacheManager = &MockCacheManager{}
	suite.logger = zaptest.NewLogger(suite.T())
	suite.ctx = context.Background()

	config := PermissionServiceConfig{
		CacheTTL:              5 * time.Minute,
		BulkEvaluationEnabled: true,
		CacheWarmingEnabled:   false, // Disable for tests
		MaxCacheSize:          1000,
		EvaluationTimeout:     5 * time.Second,
	}

	suite.service = NewPermissionService(
		suite.mockPermRepo,
		suite.mockRoleRepo,
		suite.mockUserRepo,
		suite.mockCacheManager,
		suite.logger,
		config,
	)
}

// TestCheckUserPermission_DirectMatch tests direct permission matching
func (suite *PermissionServiceTestSuite) TestCheckUserPermission_DirectMatch() {
	userID := uuid.New()
	resource := "user"
	action := "read"

	// Mock permission
	permission := models.Permission{
		ID:       uuid.New(),
		Name:     "user.read",
		Resource: resource,
		Action:   action,
		Scope:    "",
	}

	// Setup mocks
	suite.mockCacheManager.On("Get", mock.Anything, mock.AnythingOfType("string")).
		Return(nil, cache.ErrCacheNotFound)
	
	suite.mockPermRepo.On("GetUserPermissions", mock.Anything, userID).
		Return([]models.Permission{permission}, nil)
	
	suite.mockCacheManager.On("Set", mock.Anything, mock.AnythingOfType("string"), 
		mock.AnythingOfType("[]uint8"), mock.AnythingOfType("time.Duration")).
		Return(nil)

	// Execute
	allowed, err := suite.service.CheckUserPermission(suite.ctx, userID, resource, action)

	// Assert
	suite.NoError(err)
	suite.True(allowed)
	suite.mockPermRepo.AssertExpectations(suite.T())
	suite.mockCacheManager.AssertExpectations(suite.T())
}

// TestCheckUserPermission_CacheHit tests cache hit scenario
func (suite *PermissionServiceTestSuite) TestCheckUserPermission_CacheHit() {
	userID := uuid.New()
	resource := "user"
	action := "read"

	// Mock cached result
	cachedResult := PermissionEvaluationResult{
		Allowed:     true,
		Reason:      "cached result",
		EvaluatedAt: time.Now(),
	}
	cachedData, _ := json.Marshal(cachedResult)

	// Setup mocks - cache hit should return the cached data
	suite.mockCacheManager.On("Get", mock.Anything, mock.AnythingOfType("string")).
		Return(cachedData, nil).Once()

	// Execute
	allowed, err := suite.service.CheckUserPermission(suite.ctx, userID, resource, action)

	// Assert
	suite.NoError(err)
	suite.True(allowed)
	suite.mockCacheManager.AssertExpectations(suite.T())
	// Repository should not be called due to cache hit
	suite.mockPermRepo.AssertNotCalled(suite.T(), "GetUserPermissions")
}

// TestCheckUserPermission_WildcardMatch tests wildcard permission matching
func (suite *PermissionServiceTestSuite) TestCheckUserPermission_WildcardMatch() {
	userID := uuid.New()
	resource := "user"
	action := "read"

	// Mock wildcard permission
	permission := models.Permission{
		ID:       uuid.New(),
		Name:     "user.*",
		Resource: "user",
		Action:   "*",
		Scope:    "",
	}

	// Setup mocks
	suite.mockCacheManager.On("Get", mock.Anything, mock.AnythingOfType("string")).
		Return(nil, cache.ErrCacheNotFound)
	
	suite.mockPermRepo.On("GetUserPermissions", mock.Anything, userID).
		Return([]models.Permission{permission}, nil)
	
	suite.mockCacheManager.On("Set", mock.Anything, mock.AnythingOfType("string"), 
		mock.AnythingOfType("[]uint8"), mock.AnythingOfType("time.Duration")).
		Return(nil)

	// Execute
	allowed, err := suite.service.CheckUserPermission(suite.ctx, userID, resource, action)

	// Assert
	suite.NoError(err)
	suite.True(allowed)
}

// TestCheckUserPermission_HierarchicalMatch tests hierarchical permission matching
func (suite *PermissionServiceTestSuite) TestCheckUserPermission_HierarchicalMatch() {
	userID := uuid.New()
	resource := "user"
	action := "read"

	// Mock parent permission
	parentPermission := models.Permission{
		ID:       uuid.New(),
		Name:     "admin",
		Resource: "admin",
		Action:   "*",
		Scope:    "",
	}

	// Mock child permission
	childPermission := models.Permission{
		ID:       uuid.New(),
		Name:     "user.read",
		Resource: resource,
		Action:   action,
		Scope:    "",
		ParentID: &parentPermission.ID,
	}

	// Setup mocks
	suite.mockCacheManager.On("Get", mock.Anything, mock.AnythingOfType("string")).
		Return(nil, cache.ErrCacheNotFound)
	
	suite.mockPermRepo.On("GetUserPermissions", mock.Anything, userID).
		Return([]models.Permission{parentPermission}, nil)
	
	suite.mockPermRepo.On("GetChildPermissions", mock.Anything, parentPermission.ID).
		Return([]models.Permission{childPermission}, nil)
	
	suite.mockCacheManager.On("Set", mock.Anything, mock.AnythingOfType("string"), 
		mock.AnythingOfType("[]uint8"), mock.AnythingOfType("time.Duration")).
		Return(nil)

	// Execute
	allowed, err := suite.service.CheckUserPermission(suite.ctx, userID, resource, action)

	// Assert
	suite.NoError(err)
	suite.True(allowed)
}

// TestCheckUserPermission_NoPermission tests scenario with no matching permission
func (suite *PermissionServiceTestSuite) TestCheckUserPermission_NoPermission() {
	userID := uuid.New()
	resource := "user"
	action := "delete"

	// Mock different permission
	permission := models.Permission{
		ID:       uuid.New(),
		Name:     "user.read",
		Resource: "user",
		Action:   "read",
		Scope:    "",
	}

	// Setup mocks
	suite.mockCacheManager.On("Get", mock.Anything, mock.AnythingOfType("string")).
		Return(nil, cache.ErrCacheNotFound)
	
	suite.mockPermRepo.On("GetUserPermissions", mock.Anything, userID).
		Return([]models.Permission{permission}, nil)
	
	suite.mockPermRepo.On("GetChildPermissions", mock.Anything, permission.ID).
		Return([]models.Permission{}, nil)
	
	suite.mockCacheManager.On("Set", mock.Anything, mock.AnythingOfType("string"), 
		mock.AnythingOfType("[]uint8"), mock.AnythingOfType("time.Duration")).
		Return(nil)

	// Execute
	allowed, err := suite.service.CheckUserPermission(suite.ctx, userID, resource, action)

	// Assert
	suite.NoError(err)
	suite.False(allowed)
}

// TestCheckBulkPermissions tests bulk permission checking
func (suite *PermissionServiceTestSuite) TestCheckBulkPermissions() {
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

	// Bulk request
	request := &BulkPermissionRequest{
		UserID: userID,
		Permissions: []PermissionCheckRequest{
			{Resource: "user", Action: "read"},
			{Resource: "user", Action: "write"},
			{Resource: "user", Action: "delete"}, // This should fail
		},
	}

	// Setup mocks
	suite.mockPermRepo.On("GetUserPermissions", mock.Anything, userID).
		Return(permissions, nil).Times(3) // Called for each permission check

	// Execute
	response, err := suite.service.CheckBulkPermissions(suite.ctx, request)

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

// TestGetUserEffectivePermissions tests getting effective permissions
func (suite *PermissionServiceTestSuite) TestGetUserEffectivePermissions() {
	userID := uuid.New()

	// Mock permissions with hierarchy
	parentID := uuid.New()
	permissions := []models.Permission{
		{
			ID:       uuid.New(),
			Name:     "user.read",
			Resource: "user",
			Action:   "read",
		},
		{
			ID:       parentID,
			Name:     "admin",
			Resource: "admin",
			Action:   "*",
		},
	}

	hierarchicalPermissions := []models.Permission{
		{
			ID:       parentID,
			Name:     "admin",
			Resource: "admin",
			Action:   "*",
		},
		{
			ID:       uuid.New(),
			Name:     "admin.users",
			Resource: "user",
			Action:   "*",
			ParentID: &parentID,
		},
	}

	// Setup mocks
	suite.mockCacheManager.On("Get", mock.Anything, mock.AnythingOfType("string")).
		Return(nil, cache.ErrCacheNotFound)
	
	suite.mockPermRepo.On("GetUserPermissions", mock.Anything, userID).
		Return(permissions, nil)
	
	suite.mockPermRepo.On("GetPermissionHierarchy", mock.Anything, permissions[0].ID).
		Return([]models.Permission{permissions[0]}, nil)
	
	suite.mockPermRepo.On("GetPermissionHierarchy", mock.Anything, permissions[1].ID).
		Return(hierarchicalPermissions, nil)
	
	suite.mockCacheManager.On("Set", mock.Anything, mock.AnythingOfType("string"), 
		mock.AnythingOfType("[]uint8"), mock.AnythingOfType("time.Duration")).
		Return(nil)

	// Execute
	effectivePermissions, err := suite.service.GetUserEffectivePermissions(suite.ctx, userID)

	// Assert
	suite.NoError(err)
	suite.NotEmpty(effectivePermissions)
	// Should have deduplicated permissions
	suite.Len(effectivePermissions, 3) // user.read, admin, admin.users
}

// TestGetPermissionHierarchy tests getting permission hierarchy
func (suite *PermissionServiceTestSuite) TestGetPermissionHierarchy() {
	parentID := uuid.New()

	// Mock hierarchical permissions
	permissions := []models.Permission{
		{
			ID:       parentID,
			Name:     "admin",
			Resource: "admin",
			Action:   "*",
			ParentID: nil, // Explicitly set to nil for root
		},
		{
			ID:       uuid.New(),
			Name:     "admin.users",
			Resource: "user",
			Action:   "*",
			ParentID: &parentID,
		},
		{
			ID:       uuid.New(),
			Name:     "admin.roles",
			Resource: "role",
			Action:   "*",
			ParentID: &parentID,
		},
	}

	// Setup mocks
	suite.mockPermRepo.On("GetPermissionHierarchy", mock.Anything, parentID).
		Return(permissions, nil)

	// Execute
	hierarchy, err := suite.service.GetPermissionHierarchy(suite.ctx, &parentID)

	// Assert
	suite.NoError(err)
	suite.NotEmpty(hierarchy)
	// Should build proper tree structure
	suite.Len(hierarchy, 1) // One root
	suite.Len(hierarchy[0].Children, 2) // Two children
}

// TestInvalidateUserPermissionCache tests cache invalidation
func (suite *PermissionServiceTestSuite) TestInvalidateUserPermissionCache() {
	userID := uuid.New()
	expectedPattern := fmt.Sprintf("user:permissions:*:%s", userID.String())

	// Setup mocks
	suite.mockCacheManager.On("InvalidatePattern", suite.ctx, expectedPattern).
		Return(nil)

	// Execute
	err := suite.service.InvalidateUserPermissionCache(suite.ctx, userID)

	// Assert
	suite.NoError(err)
	suite.mockCacheManager.AssertExpectations(suite.T())
}

// Benchmark tests

// BenchmarkCheckUserPermission_CacheHit benchmarks cache hit performance
func BenchmarkCheckUserPermission_CacheHit(b *testing.B) {
	suite := &PermissionServiceTestSuite{}
	suite.SetupTest()

	userID := uuid.New()
	resource := "user"
	action := "read"

	// Mock cached result
	cachedResult := PermissionEvaluationResult{
		Allowed:     true,
		Reason:      "cached result",
		EvaluatedAt: time.Now(),
	}
	cachedData, _ := json.Marshal(cachedResult)

	suite.mockCacheManager.On("Get", mock.Anything, mock.AnythingOfType("string")).
		Return(cachedData, nil)

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_, err := suite.service.CheckUserPermission(context.Background(), userID, resource, action)
			if err != nil {
				b.Fatal(err)
			}
		}
	})
}

// BenchmarkCheckUserPermission_DatabaseLookup benchmarks database lookup performance
func BenchmarkCheckUserPermission_DatabaseLookup(b *testing.B) {
	suite := &PermissionServiceTestSuite{}
	suite.SetupTest()

	userID := uuid.New()
	resource := "user"
	action := "read"

	// Mock permission
	permission := models.Permission{
		ID:       uuid.New(),
		Name:     "user.read",
		Resource: resource,
		Action:   action,
		Scope:    "",
	}

	suite.mockCacheManager.On("Get", mock.Anything, mock.AnythingOfType("string")).
		Return(nil, cache.ErrCacheNotFound)
	
	suite.mockPermRepo.On("GetUserPermissions", mock.Anything, userID).
		Return([]models.Permission{permission}, nil)
	
	suite.mockCacheManager.On("Set", mock.Anything, mock.AnythingOfType("string"), 
		mock.AnythingOfType("[]uint8"), mock.AnythingOfType("time.Duration")).
		Return(nil)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := suite.service.CheckUserPermission(context.Background(), userID, resource, action)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkBulkPermissionCheck benchmarks bulk permission checking
func BenchmarkBulkPermissionCheck(b *testing.B) {
	suite := &PermissionServiceTestSuite{}
	suite.SetupTest()

	userID := uuid.New()

	// Mock permissions
	permissions := []models.Permission{
		{ID: uuid.New(), Name: "user.read", Resource: "user", Action: "read"},
		{ID: uuid.New(), Name: "user.write", Resource: "user", Action: "write"},
		{ID: uuid.New(), Name: "role.read", Resource: "role", Action: "read"},
	}

	// Bulk request with 10 permissions
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

	suite.mockPermRepo.On("GetUserPermissions", mock.Anything, userID).
		Return(permissions, nil)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := suite.service.CheckBulkPermissions(context.Background(), request)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// Table-driven tests for wildcard matching
func (suite *PermissionServiceTestSuite) TestMatchesWildcard() {
	testCases := []struct {
		name     string
		pattern  string
		value    string
		expected bool
	}{
		{"exact match", "user", "user", true},
		{"wildcard all", "*", "anything", true},
		{"prefix wildcard", "user*", "user.read", true},
		{"prefix wildcard no match", "user*", "role.read", false},
		{"suffix wildcard", "*read", "user.read", true},
		{"suffix wildcard no match", "*read", "user.write", false},
		{"no wildcard no match", "user", "role", false},
	}

	for _, tc := range testCases {
		suite.Run(tc.name, func() {
			result := suite.service.matchesWildcard(tc.pattern, tc.value)
			suite.Equal(tc.expected, result)
		})
	}
}

// Test permission evaluation with scope
func (suite *PermissionServiceTestSuite) TestCheckUserPermissionWithScope() {
	userID := uuid.New()
	resource := "user"
	action := "read"
	scope := "organization"

	// Mock permission with scope
	permission := models.Permission{
		ID:       uuid.New(),
		Name:     "user.read.org",
		Resource: resource,
		Action:   action,
		Scope:    scope,
	}

	// Setup mocks
	suite.mockCacheManager.On("Get", mock.Anything, mock.AnythingOfType("string")).
		Return(nil, cache.ErrCacheNotFound)
	
	suite.mockPermRepo.On("GetUserPermissions", mock.Anything, userID).
		Return([]models.Permission{permission}, nil)
	
	suite.mockCacheManager.On("Set", mock.Anything, mock.AnythingOfType("string"), 
		mock.AnythingOfType("[]uint8"), mock.AnythingOfType("time.Duration")).
		Return(nil)

	// Execute
	allowed, err := suite.service.CheckUserPermissionWithScope(suite.ctx, userID, resource, action, scope)

	// Assert
	suite.NoError(err)
	suite.True(allowed)
}

// Run the test suite
func TestPermissionServiceTestSuite(t *testing.T) {
	suite.Run(t, new(PermissionServiceTestSuite))
}

// Test helper functions
func (suite *PermissionServiceTestSuite) TestDeduplicatePermissions() {
	id1 := uuid.New()
	id2 := uuid.New()

	permissions := []entities.Permission{
		{ID: id1, Name: "perm1"},
		{ID: id2, Name: "perm2"},
		{ID: id1, Name: "perm1"}, // Duplicate
		{ID: id2, Name: "perm2"}, // Duplicate
	}

	result := suite.service.deduplicatePermissions(permissions)

	suite.Len(result, 2)
	suite.Equal(id1, result[0].ID)
	suite.Equal(id2, result[1].ID)
}

// Test model to entity conversion
func (suite *PermissionServiceTestSuite) TestModelToEntity() {
	parentID := uuid.New()
	model := &models.Permission{
		ID:          uuid.New(),
		Name:        "test.permission",
		Resource:    "test",
		Action:      "read",
		Scope:       "org",
		ParentID:    &parentID,
		Description: "Test permission",
		IsSystem:    true,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}

	entity := suite.service.modelToEntity(model)

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