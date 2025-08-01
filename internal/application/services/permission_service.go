package services

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"erp-auth-service/internal/cache"
	"erp-auth-service/internal/domain/entities"
	"erp-auth-service/internal/domain/interfaces"
	"erp-auth-service/internal/models"
)

// PermissionService implements hierarchical RBAC with caching optimization
type PermissionService struct {
	permissionRepo interfaces.PermissionRepository
	roleRepo       interfaces.RoleRepository
	userRepo       interfaces.UserRepository
	cacheManager   cache.CacheManager
	logger         *zap.Logger
	
	// Performance optimization
	evaluationCache sync.Map // In-memory cache for frequent evaluations
	bulkEvaluator   *BulkPermissionEvaluator
	cacheWarmer     *PermissionCacheWarmer
	
	// Configuration
	config PermissionServiceConfig
}

// PermissionServiceConfig holds configuration for the permission service
type PermissionServiceConfig struct {
	CacheTTL              time.Duration
	BulkEvaluationEnabled bool
	CacheWarmingEnabled   bool
	MaxCacheSize          int
	EvaluationTimeout     time.Duration
}

// PermissionEvaluationResult represents the result of a permission check
type PermissionEvaluationResult struct {
	Allowed     bool                   `json:"allowed"`
	Permission  *entities.Permission   `json:"permission,omitempty"`
	Reason      string                 `json:"reason"`
	Metadata    map[string]interface{} `json:"metadata,omitempty"`
	EvaluatedAt time.Time              `json:"evaluated_at"`
}

// BulkPermissionRequest represents a bulk permission check request
type BulkPermissionRequest struct {
	UserID      uuid.UUID                    `json:"user_id"`
	Permissions []PermissionCheckRequest     `json:"permissions"`
	Context     map[string]interface{}       `json:"context,omitempty"`
}

// PermissionCheckRequest represents a single permission check
type PermissionCheckRequest struct {
	Resource string                 `json:"resource"`
	Action   string                 `json:"action"`
	Scope    string                 `json:"scope,omitempty"`
	Context  map[string]interface{} `json:"context,omitempty"`
}

// BulkPermissionResponse represents the response for bulk permission checks
type BulkPermissionResponse struct {
	UserID  uuid.UUID                     `json:"user_id"`
	Results []PermissionEvaluationResult  `json:"results"`
}

// NewPermissionService creates a new permission service instance
func NewPermissionService(
	permissionRepo interfaces.PermissionRepository,
	roleRepo interfaces.RoleRepository,
	userRepo interfaces.UserRepository,
	cacheManager cache.CacheManager,
	logger *zap.Logger,
	config PermissionServiceConfig,
) *PermissionService {
	service := &PermissionService{
		permissionRepo: permissionRepo,
		roleRepo:       roleRepo,
		userRepo:       userRepo,
		cacheManager:   cacheManager,
		logger:         logger.With(zap.String("component", "permission_service")),
		config:         config,
	}

	// Initialize performance optimization components
	service.bulkEvaluator = NewBulkPermissionEvaluator(service, logger)
	service.cacheWarmer = NewPermissionCacheWarmer(service, cacheManager, logger)

	// Start cache warming if enabled
	if config.CacheWarmingEnabled {
		go service.cacheWarmer.Start(context.Background())
	}

	return service
}

// CheckUserPermission checks if a user has a specific permission with caching
func (ps *PermissionService) CheckUserPermission(ctx context.Context, userID uuid.UUID, resource, action string) (bool, error) {
	return ps.CheckUserPermissionWithScope(ctx, userID, resource, action, "")
}

// CheckUserPermissionWithScope checks permission with scope support
func (ps *PermissionService) CheckUserPermissionWithScope(ctx context.Context, userID uuid.UUID, resource, action, scope string) (bool, error) {
	// Create cache key for this permission check
	cacheKey := ps.buildPermissionCacheKey(userID, resource, action, scope)
	
	// Try to get from cache first
	if cached, err := ps.getFromCache(ctx, cacheKey); err == nil {
		ps.logger.Debug("Permission check cache hit", 
			zap.String("user_id", userID.String()),
			zap.String("resource", resource),
			zap.String("action", action),
			zap.String("scope", scope))
		return cached.Allowed, nil
	}

	// Evaluate permission
	result, err := ps.evaluatePermission(ctx, userID, resource, action, scope)
	if err != nil {
		return false, fmt.Errorf("failed to evaluate permission: %w", err)
	}

	// Cache the result
	if err := ps.cacheResult(ctx, cacheKey, result); err != nil {
		ps.logger.Warn("Failed to cache permission result", zap.Error(err))
	}

	return result.Allowed, nil
}

// CheckBulkPermissions performs bulk permission checking for performance
func (ps *PermissionService) CheckBulkPermissions(ctx context.Context, request *BulkPermissionRequest) (*BulkPermissionResponse, error) {
	if !ps.config.BulkEvaluationEnabled {
		return ps.checkBulkPermissionsSequential(ctx, request)
	}

	return ps.bulkEvaluator.EvaluateBulk(ctx, request)
}

// GetUserEffectivePermissions retrieves all effective permissions for a user
func (ps *PermissionService) GetUserEffectivePermissions(ctx context.Context, userID uuid.UUID) ([]entities.Permission, error) {
	// Try cache first
	cacheKey := fmt.Sprintf("user:permissions:effective:%s", userID.String())
	if cached, err := ps.getUserPermissionsFromCache(ctx, cacheKey); err == nil {
		return cached, nil
	}

	// Get user permissions from repository
	permissions, err := ps.permissionRepo.GetUserPermissions(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to get user permissions: %w", err)
	}

	// Convert to entities and resolve hierarchy
	effectivePermissions := make([]entities.Permission, 0, len(permissions))
	for _, perm := range permissions {
		entityPerm := ps.modelToEntity(&perm)
		
		// Get hierarchical permissions
		hierarchical, err := ps.getHierarchicalPermissions(ctx, entityPerm.ID)
		if err != nil {
			ps.logger.Warn("Failed to get hierarchical permissions", 
				zap.String("permission_id", entityPerm.ID.String()),
				zap.Error(err))
			continue
		}
		
		effectivePermissions = append(effectivePermissions, hierarchical...)
	}

	// Remove duplicates
	effectivePermissions = ps.deduplicatePermissions(effectivePermissions)

	// Cache the result
	if err := ps.cacheUserPermissions(ctx, cacheKey, effectivePermissions); err != nil {
		ps.logger.Warn("Failed to cache user permissions", zap.Error(err))
	}

	return effectivePermissions, nil
}

// GetPermissionHierarchy retrieves the permission hierarchy starting from a parent
func (ps *PermissionService) GetPermissionHierarchy(ctx context.Context, parentID *uuid.UUID) ([]entities.Permission, error) {
	var permissions []models.Permission
	var err error

	if parentID == nil {
		// Get root permissions (no parent)
		permissions, err = ps.permissionRepo.GetPermissionsByResource(ctx, "")
		if err != nil {
			return nil, fmt.Errorf("failed to get root permissions: %w", err)
		}
	} else {
		// Get hierarchy for specific parent
		permissions, err = ps.permissionRepo.GetPermissionHierarchy(ctx, *parentID)
		if err != nil {
			return nil, fmt.Errorf("failed to get permission hierarchy: %w", err)
		}
	}

	// Convert to entities and build hierarchy
	return ps.buildPermissionTree(permissions), nil
}

// InvalidateUserPermissionCache invalidates cached permissions for a user
func (ps *PermissionService) InvalidateUserPermissionCache(ctx context.Context, userID uuid.UUID) error {
	pattern := fmt.Sprintf("user:permissions:*:%s", userID.String())
	return ps.cacheManager.InvalidatePattern(ctx, pattern)
}

// WarmPermissionCache warms the cache with frequently accessed permissions
func (ps *PermissionService) WarmPermissionCache(ctx context.Context, userIDs []uuid.UUID) error {
	if !ps.config.CacheWarmingEnabled {
		return nil
	}

	return ps.cacheWarmer.WarmUserPermissions(ctx, userIDs)
}

// evaluatePermission performs the core permission evaluation logic
func (ps *PermissionService) evaluatePermission(ctx context.Context, userID uuid.UUID, resource, action, scope string) (*PermissionEvaluationResult, error) {
	// Set evaluation timeout
	evalCtx, cancel := context.WithTimeout(ctx, ps.config.EvaluationTimeout)
	defer cancel()

	// Get user permissions
	permissions, err := ps.permissionRepo.GetUserPermissions(evalCtx, userID)
	if err != nil {
		return &PermissionEvaluationResult{
			Allowed:     false,
			Reason:      "failed to retrieve user permissions",
			EvaluatedAt: time.Now(),
		}, fmt.Errorf("failed to get user permissions: %w", err)
	}

	// Check direct permissions
	for _, perm := range permissions {
		if ps.matchesPermission(&perm, resource, action, scope) {
			return &PermissionEvaluationResult{
				Allowed:     true,
				Permission:  ps.modelToEntity(&perm),
				Reason:      "direct permission match",
				EvaluatedAt: time.Now(),
			}, nil
		}
	}

	// Check hierarchical permissions
	for _, perm := range permissions {
		if allowed, matchedPerm := ps.checkHierarchicalPermission(evalCtx, &perm, resource, action, scope); allowed {
			return &PermissionEvaluationResult{
				Allowed:     true,
				Permission:  matchedPerm,
				Reason:      "hierarchical permission match",
				EvaluatedAt: time.Now(),
			}, nil
		}
	}

	return &PermissionEvaluationResult{
		Allowed:     false,
		Reason:      "no matching permission found",
		EvaluatedAt: time.Now(),
	}, nil
}

// matchesPermission checks if a permission matches the requested resource/action/scope
func (ps *PermissionService) matchesPermission(perm *models.Permission, resource, action, scope string) bool {
	// Exact match
	if perm.Resource == resource && perm.Action == action {
		if scope == "" || perm.Scope == "" || perm.Scope == scope {
			return true
		}
	}

	// Wildcard matching
	if ps.matchesWildcard(perm.Resource, resource) && ps.matchesWildcard(perm.Action, action) {
		if scope == "" || perm.Scope == "" || ps.matchesWildcard(perm.Scope, scope) {
			return true
		}
	}

	return false
}

// matchesWildcard performs wildcard matching for permissions
func (ps *PermissionService) matchesWildcard(pattern, value string) bool {
	if pattern == "*" {
		return true
	}
	
	if strings.HasSuffix(pattern, "*") {
		prefix := strings.TrimSuffix(pattern, "*")
		return strings.HasPrefix(value, prefix)
	}
	
	if strings.HasPrefix(pattern, "*") {
		suffix := strings.TrimPrefix(pattern, "*")
		return strings.HasSuffix(value, suffix)
	}
	
	return pattern == value
}

// checkHierarchicalPermission checks if a permission grants access through hierarchy
func (ps *PermissionService) checkHierarchicalPermission(ctx context.Context, perm *models.Permission, resource, action, scope string) (bool, *entities.Permission) {
	// Get child permissions
	children, err := ps.permissionRepo.GetChildPermissions(ctx, perm.ID)
	if err != nil {
		ps.logger.Warn("Failed to get child permissions", 
			zap.String("permission_id", perm.ID.String()),
			zap.Error(err))
		return false, nil
	}

	// Check each child permission
	for _, child := range children {
		if ps.matchesPermission(&child, resource, action, scope) {
			return true, ps.modelToEntity(&child)
		}
		
		// Recursive check for deeper hierarchy
		if allowed, matchedPerm := ps.checkHierarchicalPermission(ctx, &child, resource, action, scope); allowed {
			return true, matchedPerm
		}
	}

	return false, nil
}

// getHierarchicalPermissions gets all permissions in a hierarchy
func (ps *PermissionService) getHierarchicalPermissions(ctx context.Context, permissionID uuid.UUID) ([]entities.Permission, error) {
	permissions, err := ps.permissionRepo.GetPermissionHierarchy(ctx, permissionID)
	if err != nil {
		return nil, err
	}

	result := make([]entities.Permission, len(permissions))
	for i, perm := range permissions {
		result[i] = *ps.modelToEntity(&perm)
	}

	return result, nil
}

// buildPermissionTree builds a hierarchical tree from flat permission list
func (ps *PermissionService) buildPermissionTree(permissions []models.Permission) []entities.Permission {
	permMap := make(map[uuid.UUID]*entities.Permission)
	var roots []*entities.Permission

	// First pass: create all permission entities
	for _, perm := range permissions {
		entity := ps.modelToEntity(&perm)
		entity.Children = make([]entities.Permission, 0) // Initialize children slice
		permMap[entity.ID] = entity
	}

	// Second pass: build parent-child relationships
	for _, perm := range permissions {
		entity := permMap[perm.ID]
		if perm.ParentID != nil {
			if parent, exists := permMap[*perm.ParentID]; exists {
				parent.Children = append(parent.Children, *entity)
			}
		} else {
			roots = append(roots, entity)
		}
	}

	// Convert pointers back to values for return
	result := make([]entities.Permission, len(roots))
	for i, root := range roots {
		result[i] = *root
	}

	return result
}

// checkBulkPermissionsSequential performs sequential bulk permission checking
func (ps *PermissionService) checkBulkPermissionsSequential(ctx context.Context, request *BulkPermissionRequest) (*BulkPermissionResponse, error) {
	results := make([]PermissionEvaluationResult, len(request.Permissions))

	for i, permReq := range request.Permissions {
		result, err := ps.evaluatePermission(ctx, request.UserID, permReq.Resource, permReq.Action, permReq.Scope)
		if err != nil {
			results[i] = PermissionEvaluationResult{
				Allowed:     false,
				Reason:      fmt.Sprintf("evaluation error: %v", err),
				EvaluatedAt: time.Now(),
			}
		} else {
			results[i] = *result
		}
	}

	return &BulkPermissionResponse{
		UserID:  request.UserID,
		Results: results,
	}, nil
}

// Cache management methods

func (ps *PermissionService) buildPermissionCacheKey(userID uuid.UUID, resource, action, scope string) string {
	return fmt.Sprintf("user:permission:%s:%s:%s:%s", userID.String(), resource, action, scope)
}

func (ps *PermissionService) getFromCache(ctx context.Context, key string) (*PermissionEvaluationResult, error) {
	data, err := ps.cacheManager.Get(ctx, key)
	if err != nil {
		return nil, err
	}

	var result PermissionEvaluationResult
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, err
	}

	return &result, nil
}

func (ps *PermissionService) cacheResult(ctx context.Context, key string, result *PermissionEvaluationResult) error {
	data, err := json.Marshal(result)
	if err != nil {
		return err
	}

	return ps.cacheManager.Set(ctx, key, data, ps.config.CacheTTL)
}

func (ps *PermissionService) getUserPermissionsFromCache(ctx context.Context, key string) ([]entities.Permission, error) {
	data, err := ps.cacheManager.Get(ctx, key)
	if err != nil {
		return nil, err
	}

	var permissions []entities.Permission
	if err := json.Unmarshal(data, &permissions); err != nil {
		return nil, err
	}

	return permissions, nil
}

func (ps *PermissionService) cacheUserPermissions(ctx context.Context, key string, permissions []entities.Permission) error {
	data, err := json.Marshal(permissions)
	if err != nil {
		return err
	}

	return ps.cacheManager.Set(ctx, key, data, ps.config.CacheTTL)
}

// Utility methods

func (ps *PermissionService) modelToEntity(perm *models.Permission) *entities.Permission {
	return &entities.Permission{
		ID:          perm.ID,
		Name:        perm.Name,
		Resource:    perm.Resource,
		Action:      perm.Action,
		Scope:       perm.Scope,
		ParentID:    perm.ParentID,
		Description: perm.Description,
		IsSystem:    perm.IsSystem,
		CreatedAt:   perm.CreatedAt,
		UpdatedAt:   perm.UpdatedAt,
	}
}

func (ps *PermissionService) deduplicatePermissions(permissions []entities.Permission) []entities.Permission {
	seen := make(map[uuid.UUID]bool)
	result := make([]entities.Permission, 0, len(permissions))

	for _, perm := range permissions {
		if !seen[perm.ID] {
			seen[perm.ID] = true
			result = append(result, perm)
		}
	}

	return result
}

// Close gracefully shuts down the permission service
func (ps *PermissionService) Close() error {
	if ps.cacheWarmer != nil {
		ps.cacheWarmer.Stop()
	}
	if ps.bulkEvaluator != nil {
		ps.bulkEvaluator.Close()
	}
	return nil
}