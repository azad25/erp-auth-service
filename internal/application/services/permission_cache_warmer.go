package services

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"erp-auth-service/internal/cache"
)

// PermissionCacheWarmer handles intelligent cache warming based on usage patterns
type PermissionCacheWarmer struct {
	permissionService *PermissionService
	cacheManager      cache.CacheManager
	logger           *zap.Logger
	
	// Usage tracking
	usageTracker     *UsageTracker
	warmingScheduler *WarmingScheduler
	
	// Configuration
	config CacheWarmerConfig
	
	// Control
	stopChan chan struct{}
	wg       sync.WaitGroup
}

// CacheWarmerConfig holds configuration for cache warming
type CacheWarmerConfig struct {
	WarmingInterval     time.Duration
	UsageTrackingWindow time.Duration
	MinUsageThreshold   int
	MaxWarmingBatch     int
	WarmingTimeout      time.Duration
	PredictiveWarming   bool
}

// UsageTracker tracks permission access patterns
type UsageTracker struct {
	mu           sync.RWMutex
	usageStats   map[string]*UsageStats
	timeWindow   time.Duration
	cleanupTicker *time.Ticker
}

// UsageStats represents usage statistics for a permission
type UsageStats struct {
	UserID       uuid.UUID
	Resource     string
	Action       string
	Scope        string
	AccessCount  int
	LastAccessed time.Time
	FirstSeen    time.Time
	Pattern      AccessPattern
}

// AccessPattern represents the access pattern for a permission
type AccessPattern struct {
	Frequency    float64 // accesses per hour
	Regularity   float64 // 0-1, how regular the access pattern is
	Trending     bool    // whether usage is increasing
	PeakHours    []int   // hours of day with peak usage
}

// WarmingScheduler schedules cache warming operations
type WarmingScheduler struct {
	warmingQueue chan WarmingTask
	workers      int
	logger       *zap.Logger
}

// WarmingTask represents a cache warming task
type WarmingTask struct {
	UserID      uuid.UUID
	Permissions []PermissionCheckRequest
	Priority    int
	CreatedAt   time.Time
}

// NewPermissionCacheWarmer creates a new permission cache warmer
func NewPermissionCacheWarmer(
	permissionService *PermissionService,
	cacheManager cache.CacheManager,
	logger *zap.Logger,
) *PermissionCacheWarmer {
	config := CacheWarmerConfig{
		WarmingInterval:     5 * time.Minute,
		UsageTrackingWindow: 24 * time.Hour,
		MinUsageThreshold:   3,
		MaxWarmingBatch:     100,
		WarmingTimeout:      30 * time.Second,
		PredictiveWarming:   true,
	}

	warmer := &PermissionCacheWarmer{
		permissionService: permissionService,
		cacheManager:      cacheManager,
		logger:           logger.With(zap.String("component", "permission_cache_warmer")),
		config:           config,
		stopChan:         make(chan struct{}),
	}

	// Initialize usage tracker
	warmer.usageTracker = NewUsageTracker(config.UsageTrackingWindow, logger)
	
	// Initialize warming scheduler
	warmer.warmingScheduler = NewWarmingScheduler(5, logger) // 5 workers

	return warmer
}

// Start starts the cache warming process
func (pcw *PermissionCacheWarmer) Start(ctx context.Context) {
	pcw.logger.Info("Starting permission cache warmer")
	
	// Start usage tracker
	pcw.usageTracker.Start()
	
	// Start warming scheduler
	pcw.warmingScheduler.Start(ctx, pcw.processWarmingTask)
	
	// Start warming ticker
	ticker := time.NewTicker(pcw.config.WarmingInterval)
	defer ticker.Stop()

	pcw.wg.Add(1)
	go func() {
		defer pcw.wg.Done()
		for {
			select {
			case <-ticker.C:
				if err := pcw.performScheduledWarming(ctx); err != nil {
					pcw.logger.Error("Scheduled warming failed", zap.Error(err))
				}
			case <-pcw.stopChan:
				return
			case <-ctx.Done():
				return
			}
		}
	}()
}

// Stop stops the cache warming process
func (pcw *PermissionCacheWarmer) Stop() {
	pcw.logger.Info("Stopping permission cache warmer")
	close(pcw.stopChan)
	pcw.wg.Wait()
	
	if pcw.usageTracker != nil {
		pcw.usageTracker.Stop()
	}
	
	if pcw.warmingScheduler != nil {
		pcw.warmingScheduler.Stop()
	}
}

// TrackUsage tracks permission usage for warming decisions
func (pcw *PermissionCacheWarmer) TrackUsage(userID uuid.UUID, resource, action, scope string) {
	pcw.usageTracker.TrackUsage(userID, resource, action, scope)
}

// WarmUserPermissions warms cache for specific users
func (pcw *PermissionCacheWarmer) WarmUserPermissions(ctx context.Context, userIDs []uuid.UUID) error {
	pcw.logger.Debug("Warming user permissions", zap.Int("user_count", len(userIDs)))

	for _, userID := range userIDs {
		// Get user's effective permissions and cache them
		permissions, err := pcw.permissionService.GetUserEffectivePermissions(ctx, userID)
		if err != nil {
			pcw.logger.Warn("Failed to get user permissions for warming",
				zap.String("user_id", userID.String()),
				zap.Error(err))
			continue
		}

		// Cache individual permission checks for common patterns
		for _, perm := range permissions {
			cacheKey := pcw.permissionService.buildPermissionCacheKey(userID, perm.Resource, perm.Action, perm.Scope)
			result := &PermissionEvaluationResult{
				Allowed:     true,
				Permission:  &perm,
				Reason:      "preloaded permission",
				EvaluatedAt: time.Now(),
			}
			
			if err := pcw.permissionService.cacheResult(ctx, cacheKey, result); err != nil {
				pcw.logger.Warn("Failed to cache permission result",
					zap.String("cache_key", cacheKey),
					zap.Error(err))
			}
		}
	}

	return nil
}

// WarmFrequentlyUsedPermissions warms cache based on usage patterns
func (pcw *PermissionCacheWarmer) WarmFrequentlyUsedPermissions(ctx context.Context) error {
	// Get frequently used permissions from usage tracker
	frequentPermissions := pcw.usageTracker.GetFrequentlyUsed(pcw.config.MinUsageThreshold)
	
	pcw.logger.Debug("Warming frequently used permissions",
		zap.Int("permission_count", len(frequentPermissions)))

	// Group by user for batch processing
	userPermissions := make(map[uuid.UUID][]PermissionCheckRequest)
	for _, stats := range frequentPermissions {
		userPermissions[stats.UserID] = append(userPermissions[stats.UserID], PermissionCheckRequest{
			Resource: stats.Resource,
			Action:   stats.Action,
			Scope:    stats.Scope,
		})
	}

	// Create warming tasks
	for userID, permissions := range userPermissions {
		if len(permissions) > pcw.config.MaxWarmingBatch {
			permissions = permissions[:pcw.config.MaxWarmingBatch]
		}

		task := WarmingTask{
			UserID:      userID,
			Permissions: permissions,
			Priority:    pcw.calculatePriority(permissions),
			CreatedAt:   time.Now(),
		}

		select {
		case pcw.warmingScheduler.warmingQueue <- task:
		case <-ctx.Done():
			return ctx.Err()
		default:
			pcw.logger.Warn("Warming queue full, skipping task",
				zap.String("user_id", userID.String()))
		}
	}

	return nil
}

// PredictiveWarm performs predictive cache warming based on patterns
func (pcw *PermissionCacheWarmer) PredictiveWarm(ctx context.Context) error {
	if !pcw.config.PredictiveWarming {
		return nil
	}

	// Get trending permissions
	trendingPermissions := pcw.usageTracker.GetTrendingPermissions()
	
	pcw.logger.Debug("Performing predictive warming",
		zap.Int("trending_count", len(trendingPermissions)))

	// Warm trending permissions
	for _, stats := range trendingPermissions {
		cacheKey := pcw.permissionService.buildPermissionCacheKey(
			stats.UserID, stats.Resource, stats.Action, stats.Scope)
		
		// Check if already cached
		if _, err := pcw.cacheManager.Get(ctx, cacheKey); err == nil {
			continue // Already cached
		}

		// Evaluate and cache
		result, err := pcw.permissionService.evaluatePermission(
			ctx, stats.UserID, stats.Resource, stats.Action, stats.Scope)
		if err != nil {
			pcw.logger.Warn("Failed to evaluate permission for predictive warming",
				zap.String("user_id", stats.UserID.String()),
				zap.String("resource", stats.Resource),
				zap.String("action", stats.Action),
				zap.Error(err))
			continue
		}

		if err := pcw.permissionService.cacheResult(ctx, cacheKey, result); err != nil {
			pcw.logger.Warn("Failed to cache predictive result", zap.Error(err))
		}
	}

	return nil
}

// performScheduledWarming performs the scheduled warming operations
func (pcw *PermissionCacheWarmer) performScheduledWarming(ctx context.Context) error {
	warmingCtx, cancel := context.WithTimeout(ctx, pcw.config.WarmingTimeout)
	defer cancel()

	// Warm frequently used permissions
	if err := pcw.WarmFrequentlyUsedPermissions(warmingCtx); err != nil {
		pcw.logger.Error("Failed to warm frequently used permissions", zap.Error(err))
	}

	// Perform predictive warming
	if err := pcw.PredictiveWarm(warmingCtx); err != nil {
		pcw.logger.Error("Failed to perform predictive warming", zap.Error(err))
	}

	return nil
}

// processWarmingTask processes a single warming task
func (pcw *PermissionCacheWarmer) processWarmingTask(ctx context.Context, task WarmingTask) error {
	// Create bulk request
	bulkRequest := &BulkPermissionRequest{
		UserID:      task.UserID,
		Permissions: task.Permissions,
	}

	// Evaluate permissions
	_, err := pcw.permissionService.CheckBulkPermissions(ctx, bulkRequest)
	if err != nil {
		return fmt.Errorf("failed to process warming task: %w", err)
	}

	pcw.logger.Debug("Processed warming task",
		zap.String("user_id", task.UserID.String()),
		zap.Int("permission_count", len(task.Permissions)),
		zap.Int("priority", task.Priority))

	return nil
}

// calculatePriority calculates the priority of a warming task
func (pcw *PermissionCacheWarmer) calculatePriority(permissions []PermissionCheckRequest) int {
	// Higher priority for more permissions and critical resources
	priority := len(permissions)
	
	for _, perm := range permissions {
		if pcw.isCriticalResource(perm.Resource) {
			priority += 10
		}
	}
	
	return priority
}

// isCriticalResource checks if a resource is considered critical
func (pcw *PermissionCacheWarmer) isCriticalResource(resource string) bool {
	criticalResources := []string{"user", "organization", "role", "permission", "auth"}
	for _, critical := range criticalResources {
		if resource == critical {
			return true
		}
	}
	return false
}

// GetStats returns cache warming statistics
func (pcw *PermissionCacheWarmer) GetStats() map[string]interface{} {
	return map[string]interface{}{
		"warming_interval":      pcw.config.WarmingInterval.String(),
		"usage_tracking_window": pcw.config.UsageTrackingWindow.String(),
		"min_usage_threshold":   pcw.config.MinUsageThreshold,
		"max_warming_batch":     pcw.config.MaxWarmingBatch,
		"predictive_warming":    pcw.config.PredictiveWarming,
		"usage_tracker_stats":   pcw.usageTracker.GetStats(),
		"scheduler_stats":       pcw.warmingScheduler.GetStats(),
	}
}

// NewUsageTracker creates a new usage tracker
func NewUsageTracker(timeWindow time.Duration, logger *zap.Logger) *UsageTracker {
	tracker := &UsageTracker{
		usageStats: make(map[string]*UsageStats),
		timeWindow: timeWindow,
	}

	// Start cleanup ticker
	tracker.cleanupTicker = time.NewTicker(time.Hour)
	go tracker.cleanupLoop()

	return tracker
}

// Start starts the usage tracker
func (ut *UsageTracker) Start() {
	// Already started in constructor
}

// Stop stops the usage tracker
func (ut *UsageTracker) Stop() {
	if ut.cleanupTicker != nil {
		ut.cleanupTicker.Stop()
	}
}

// TrackUsage tracks a permission usage
func (ut *UsageTracker) TrackUsage(userID uuid.UUID, resource, action, scope string) {
	key := fmt.Sprintf("%s:%s:%s:%s", userID.String(), resource, action, scope)
	now := time.Now()

	ut.mu.Lock()
	defer ut.mu.Unlock()

	stats, exists := ut.usageStats[key]
	if !exists {
		stats = &UsageStats{
			UserID:       userID,
			Resource:     resource,
			Action:       action,
			Scope:        scope,
			AccessCount:  0,
			FirstSeen:    now,
		}
		ut.usageStats[key] = stats
	}

	stats.AccessCount++
	stats.LastAccessed = now
	
	// Update access pattern
	ut.updateAccessPattern(stats)
}

// GetFrequentlyUsed returns frequently used permissions
func (ut *UsageTracker) GetFrequentlyUsed(minThreshold int) []*UsageStats {
	ut.mu.RLock()
	defer ut.mu.RUnlock()

	var frequent []*UsageStats
	for _, stats := range ut.usageStats {
		if stats.AccessCount >= minThreshold {
			frequent = append(frequent, stats)
		}
	}

	return frequent
}

// GetTrendingPermissions returns permissions with increasing usage
func (ut *UsageTracker) GetTrendingPermissions() []*UsageStats {
	ut.mu.RLock()
	defer ut.mu.RUnlock()

	var trending []*UsageStats
	for _, stats := range ut.usageStats {
		if stats.Pattern.Trending {
			trending = append(trending, stats)
		}
	}

	return trending
}

// updateAccessPattern updates the access pattern for a permission
func (ut *UsageTracker) updateAccessPattern(stats *UsageStats) {
	duration := time.Since(stats.FirstSeen)
	if duration > 0 {
		stats.Pattern.Frequency = float64(stats.AccessCount) / duration.Hours()
	}

	// Simple trending detection (could be more sophisticated)
	if stats.AccessCount > 1 {
		recentAccesses := 0
		if time.Since(stats.LastAccessed) < time.Hour {
			recentAccesses = stats.AccessCount / 2 // Simplified
		}
		stats.Pattern.Trending = recentAccesses > stats.AccessCount/2
	}
}

// cleanupLoop periodically cleans up old usage statistics
func (ut *UsageTracker) cleanupLoop() {
	for range ut.cleanupTicker.C {
		ut.cleanup()
	}
}

// cleanup removes old usage statistics
func (ut *UsageTracker) cleanup() {
	ut.mu.Lock()
	defer ut.mu.Unlock()

	cutoff := time.Now().Add(-ut.timeWindow)
	for key, stats := range ut.usageStats {
		if stats.LastAccessed.Before(cutoff) {
			delete(ut.usageStats, key)
		}
	}
}

// GetStats returns usage tracker statistics
func (ut *UsageTracker) GetStats() map[string]interface{} {
	ut.mu.RLock()
	defer ut.mu.RUnlock()

	return map[string]interface{}{
		"tracked_permissions": len(ut.usageStats),
		"time_window":        ut.timeWindow.String(),
	}
}

// NewWarmingScheduler creates a new warming scheduler
func NewWarmingScheduler(workers int, logger *zap.Logger) *WarmingScheduler {
	return &WarmingScheduler{
		warmingQueue: make(chan WarmingTask, 1000), // Buffered queue
		workers:      workers,
		logger:       logger.With(zap.String("component", "warming_scheduler")),
	}
}

// Start starts the warming scheduler
func (ws *WarmingScheduler) Start(ctx context.Context, processor func(context.Context, WarmingTask) error) {
	for i := 0; i < ws.workers; i++ {
		go ws.worker(ctx, processor, i)
	}
}

// Stop stops the warming scheduler
func (ws *WarmingScheduler) Stop() {
	close(ws.warmingQueue)
}

// worker processes warming tasks
func (ws *WarmingScheduler) worker(ctx context.Context, processor func(context.Context, WarmingTask) error, workerID int) {
	for {
		select {
		case task, ok := <-ws.warmingQueue:
			if !ok {
				return
			}
			
			if err := processor(ctx, task); err != nil {
				ws.logger.Error("Warming task failed",
					zap.Int("worker_id", workerID),
					zap.String("user_id", task.UserID.String()),
					zap.Error(err))
			}
		case <-ctx.Done():
			return
		}
	}
}

// GetStats returns warming scheduler statistics
func (ws *WarmingScheduler) GetStats() map[string]interface{} {
	return map[string]interface{}{
		"workers":     ws.workers,
		"queue_size":  len(ws.warmingQueue),
		"queue_cap":   cap(ws.warmingQueue),
	}
}