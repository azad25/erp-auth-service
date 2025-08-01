package cache

import (
	"context"
	"fmt"
	"sync"
	"time"

	"go.uber.org/zap"
)

// CacheInvalidatorImpl implements CacheInvalidator interface
type CacheInvalidatorImpl struct {
	manager           *CacheManagerImpl
	logger            *zap.Logger
	invalidationTicker *time.Ticker
	stopChan          chan struct{}
	mu                sync.RWMutex
	scheduledInvalidations map[string]*InvalidationJob
	taggedKeys        map[string][]string // tag -> keys mapping
}

// InvalidationJob represents a scheduled invalidation job
type InvalidationJob struct {
	Key       string
	Tags      []string
	Pattern   string
	ScheduledAt time.Time
	ExecuteAt   time.Time
	Active      bool
}

// NewCacheInvalidatorImpl creates a new cache invalidator
func NewCacheInvalidatorImpl(manager *CacheManagerImpl, logger *zap.Logger) *CacheInvalidatorImpl {
	invalidator := &CacheInvalidatorImpl{
		manager:                manager,
		logger:                 logger.With(zap.String("component", "cache_invalidator")),
		stopChan:               make(chan struct{}),
		scheduledInvalidations: make(map[string]*InvalidationJob),
		taggedKeys:             make(map[string][]string),
	}

	// Start background invalidation processor
	invalidator.startInvalidationProcessor()

	return invalidator
}

// InvalidateKey removes a key from all cache levels
func (ci *CacheInvalidatorImpl) InvalidateKey(ctx context.Context, key string) error {
	if key == "" {
		return ErrInvalidKey
	}

	ci.logger.Debug("Invalidating cache key", zap.String("key", key))

	// Remove from all cache levels
	errChan := make(chan error, 2)

	go func() {
		errChan <- ci.manager.l1Cache.Delete(ctx, key)
	}()

	go func() {
		errChan <- ci.manager.l2Cache.Delete(ctx, key)
	}()

	// Collect errors but don't fail if key doesn't exist
	var errors []error
	for i := 0; i < 2; i++ {
		if err := <-errChan; err != nil && err != ErrCacheNotFound {
			errors = append(errors, err)
		}
	}

	if len(errors) > 0 {
		ci.logger.Error("Cache key invalidation failed", zap.String("key", key), zap.Errors("errors", errors))
		return errors[0]
	}

	// Remove from tag mappings
	ci.removeKeyFromTags(key)

	ci.logger.Debug("Cache key invalidated", zap.String("key", key))
	return nil
}

// InvalidatePattern removes all keys matching a pattern from all cache levels
func (ci *CacheInvalidatorImpl) InvalidatePattern(ctx context.Context, pattern string) error {
	if pattern == "" {
		return fmt.Errorf("pattern cannot be empty")
	}

	ci.logger.Info("Invalidating cache by pattern", zap.String("pattern", pattern))

	// For L1 cache (BigCache), we need to iterate through all keys
	// This is not efficient, but BigCache doesn't support pattern matching
	// In a production system, you might want to maintain a key registry

	// For L2 cache (Redis), we can use pattern matching
	if redisCache, ok := ci.manager.l2Cache.(*RedisCacheImpl); ok {
		if err := redisCache.DeletePattern(ctx, pattern); err != nil {
			ci.logger.Error("Failed to invalidate pattern in Redis", zap.String("pattern", pattern), zap.Error(err))
			return err
		}
	}

	// For L1 cache, we'll need to clear all since we can't efficiently pattern match
	// In a real implementation, you might maintain a key registry for pattern matching
	ci.logger.Warn("Pattern invalidation for L1 cache not efficiently supported, consider using tags")

	ci.logger.Info("Cache pattern invalidation completed", zap.String("pattern", pattern))
	return nil
}

// InvalidateByTags removes all keys associated with the given tags
func (ci *CacheInvalidatorImpl) InvalidateByTags(ctx context.Context, tags []string) error {
	if len(tags) == 0 {
		return fmt.Errorf("tags cannot be empty")
	}

	ci.logger.Info("Invalidating cache by tags", zap.Strings("tags", tags))

	ci.mu.RLock()
	var keysToInvalidate []string
	for _, tag := range tags {
		if keys, exists := ci.taggedKeys[tag]; exists {
			keysToInvalidate = append(keysToInvalidate, keys...)
		}
	}
	ci.mu.RUnlock()

	if len(keysToInvalidate) == 0 {
		ci.logger.Debug("No keys found for tags", zap.Strings("tags", tags))
		return nil
	}

	// Remove duplicates
	keySet := make(map[string]bool)
	var uniqueKeys []string
	for _, key := range keysToInvalidate {
		if !keySet[key] {
			keySet[key] = true
			uniqueKeys = append(uniqueKeys, key)
		}
	}

	// Invalidate all unique keys
	var wg sync.WaitGroup
	semaphore := make(chan struct{}, 10) // Limit concurrent invalidations

	for _, key := range uniqueKeys {
		wg.Add(1)
		go func(k string) {
			defer wg.Done()
			semaphore <- struct{}{}
			defer func() { <-semaphore }()

			if err := ci.InvalidateKey(ctx, k); err != nil {
				ci.logger.Error("Failed to invalidate tagged key", zap.String("key", k), zap.Error(err))
			}
		}(key)
	}

	wg.Wait()

	// Clean up tag mappings
	ci.mu.Lock()
	for _, tag := range tags {
		delete(ci.taggedKeys, tag)
	}
	ci.mu.Unlock()

	ci.logger.Info("Cache tag invalidation completed", 
		zap.Strings("tags", tags), 
		zap.Int("keys_invalidated", len(uniqueKeys)))
	return nil
}

// ScheduleInvalidation schedules a key for invalidation after a delay
func (ci *CacheInvalidatorImpl) ScheduleInvalidation(ctx context.Context, key string, delay time.Duration) error {
	if key == "" {
		return ErrInvalidKey
	}
	if delay < 0 {
		return fmt.Errorf("delay cannot be negative")
	}

	jobID := fmt.Sprintf("invalidate_%s_%d", key, time.Now().UnixNano())
	executeAt := time.Now().Add(delay)

	ci.mu.Lock()
	ci.scheduledInvalidations[jobID] = &InvalidationJob{
		Key:         key,
		ScheduledAt: time.Now(),
		ExecuteAt:   executeAt,
		Active:      true,
	}
	ci.mu.Unlock()

	ci.logger.Info("Scheduled cache invalidation", 
		zap.String("job_id", jobID),
		zap.String("key", key),
		zap.Duration("delay", delay),
		zap.Time("execute_at", executeAt))

	return nil
}

// SchedulePatternInvalidation schedules pattern invalidation after a delay
func (ci *CacheInvalidatorImpl) SchedulePatternInvalidation(ctx context.Context, pattern string, delay time.Duration) error {
	if pattern == "" {
		return fmt.Errorf("pattern cannot be empty")
	}
	if delay < 0 {
		return fmt.Errorf("delay cannot be negative")
	}

	jobID := fmt.Sprintf("invalidate_pattern_%d", time.Now().UnixNano())
	executeAt := time.Now().Add(delay)

	ci.mu.Lock()
	ci.scheduledInvalidations[jobID] = &InvalidationJob{
		Pattern:     pattern,
		ScheduledAt: time.Now(),
		ExecuteAt:   executeAt,
		Active:      true,
	}
	ci.mu.Unlock()

	ci.logger.Info("Scheduled cache pattern invalidation", 
		zap.String("job_id", jobID),
		zap.String("pattern", pattern),
		zap.Duration("delay", delay))

	return nil
}

// ScheduleTagInvalidation schedules tag-based invalidation after a delay
func (ci *CacheInvalidatorImpl) ScheduleTagInvalidation(ctx context.Context, tags []string, delay time.Duration) error {
	if len(tags) == 0 {
		return fmt.Errorf("tags cannot be empty")
	}
	if delay < 0 {
		return fmt.Errorf("delay cannot be negative")
	}

	jobID := fmt.Sprintf("invalidate_tags_%d", time.Now().UnixNano())
	executeAt := time.Now().Add(delay)

	ci.mu.Lock()
	ci.scheduledInvalidations[jobID] = &InvalidationJob{
		Tags:        tags,
		ScheduledAt: time.Now(),
		ExecuteAt:   executeAt,
		Active:      true,
	}
	ci.mu.Unlock()

	ci.logger.Info("Scheduled cache tag invalidation", 
		zap.String("job_id", jobID),
		zap.Strings("tags", tags),
		zap.Duration("delay", delay))

	return nil
}

// TagKey associates a key with tags for group invalidation
func (ci *CacheInvalidatorImpl) TagKey(key string, tags []string) {
	if key == "" || len(tags) == 0 {
		return
	}

	ci.mu.Lock()
	defer ci.mu.Unlock()

	for _, tag := range tags {
		if ci.taggedKeys[tag] == nil {
			ci.taggedKeys[tag] = make([]string, 0)
		}
		
		// Check if key already exists for this tag
		exists := false
		for _, existingKey := range ci.taggedKeys[tag] {
			if existingKey == key {
				exists = true
				break
			}
		}
		
		if !exists {
			ci.taggedKeys[tag] = append(ci.taggedKeys[tag], key)
		}
	}

	ci.logger.Debug("Tagged cache key", zap.String("key", key), zap.Strings("tags", tags))
}

// removeKeyFromTags removes a key from all tag mappings
func (ci *CacheInvalidatorImpl) removeKeyFromTags(key string) {
	ci.mu.Lock()
	defer ci.mu.Unlock()

	for tag, keys := range ci.taggedKeys {
		for i, k := range keys {
			if k == key {
				// Remove key from slice
				ci.taggedKeys[tag] = append(keys[:i], keys[i+1:]...)
				break
			}
		}
		
		// Clean up empty tag mappings
		if len(ci.taggedKeys[tag]) == 0 {
			delete(ci.taggedKeys, tag)
		}
	}
}

// startInvalidationProcessor starts the background invalidation processor
func (ci *CacheInvalidatorImpl) startInvalidationProcessor() {
	interval := 10 * time.Second
	if ci.manager.config.InvalidationDelay > 0 && ci.manager.config.InvalidationDelay < interval {
		interval = ci.manager.config.InvalidationDelay
	}
	ci.invalidationTicker = time.NewTicker(interval)

	go func() {
		for {
			select {
			case <-ci.invalidationTicker.C:
				ci.processScheduledInvalidations()
			case <-ci.stopChan:
				return
			}
		}
	}()
}

// processScheduledInvalidations processes scheduled invalidation jobs
func (ci *CacheInvalidatorImpl) processScheduledInvalidations() {
	now := time.Now()
	
	ci.mu.RLock()
	var jobsToExecute []*InvalidationJob
	var jobIDsToExecute []string
	
	for jobID, job := range ci.scheduledInvalidations {
		if job.Active && now.After(job.ExecuteAt) {
			jobsToExecute = append(jobsToExecute, job)
			jobIDsToExecute = append(jobIDsToExecute, jobID)
		}
	}
	ci.mu.RUnlock()

	if len(jobsToExecute) == 0 {
		return
	}

	// Execute invalidation jobs
	for i, job := range jobsToExecute {
		jobID := jobIDsToExecute[i]
		
		go func(j *InvalidationJob, id string) {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()

			var err error
			if j.Key != "" {
				err = ci.InvalidateKey(ctx, j.Key)
			} else if j.Pattern != "" {
				err = ci.InvalidatePattern(ctx, j.Pattern)
			} else if len(j.Tags) > 0 {
				err = ci.InvalidateByTags(ctx, j.Tags)
			}

			if err != nil {
				ci.logger.Error("Scheduled invalidation failed", zap.String("job_id", id), zap.Error(err))
			} else {
				ci.logger.Debug("Scheduled invalidation completed", zap.String("job_id", id))
			}

			// Remove completed job
			ci.mu.Lock()
			delete(ci.scheduledInvalidations, id)
			ci.mu.Unlock()
		}(job, jobID)
	}
}

// GetInvalidationStats returns statistics about invalidation operations
func (ci *CacheInvalidatorImpl) GetInvalidationStats() map[string]interface{} {
	ci.mu.RLock()
	defer ci.mu.RUnlock()

	stats := make(map[string]interface{})
	stats["scheduled_jobs"] = len(ci.scheduledInvalidations)
	stats["tagged_keys_count"] = len(ci.taggedKeys)
	
	totalTaggedKeys := 0
	for _, keys := range ci.taggedKeys {
		totalTaggedKeys += len(keys)
	}
	stats["total_tagged_keys"] = totalTaggedKeys

	activeJobs := 0
	for _, job := range ci.scheduledInvalidations {
		if job.Active {
			activeJobs++
		}
	}
	stats["active_jobs"] = activeJobs

	return stats
}

// Stop stops the invalidation processor
func (ci *CacheInvalidatorImpl) Stop() {
	if ci.invalidationTicker != nil {
		ci.invalidationTicker.Stop()
	}
	select {
	case <-ci.stopChan:
		// Channel already closed
	default:
		close(ci.stopChan)
	}
	ci.logger.Info("Cache invalidator stopped")
}

// ClearScheduledInvalidations clears all scheduled invalidation jobs
func (ci *CacheInvalidatorImpl) ClearScheduledInvalidations() {
	ci.mu.Lock()
	defer ci.mu.Unlock()
	
	count := len(ci.scheduledInvalidations)
	ci.scheduledInvalidations = make(map[string]*InvalidationJob)
	
	ci.logger.Info("Cleared scheduled invalidations", zap.Int("count", count))
}

// GetTaggedKeys returns all keys associated with a tag
func (ci *CacheInvalidatorImpl) GetTaggedKeys(tag string) []string {
	ci.mu.RLock()
	defer ci.mu.RUnlock()
	
	if keys, exists := ci.taggedKeys[tag]; exists {
		// Return a copy to avoid race conditions
		result := make([]string, len(keys))
		copy(result, keys)
		return result
	}
	
	return nil
}