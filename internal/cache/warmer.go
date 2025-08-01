package cache

import (
	"context"
	"fmt"
	"sync"
	"time"

	"go.uber.org/zap"
)

// CacheWarmerImpl implements CacheWarmer interface
type CacheWarmerImpl struct {
	manager    *CacheManagerImpl
	logger     *zap.Logger
	warmTicker *time.Ticker
	stopChan   chan struct{}
	mu         sync.RWMutex
	warmJobs   map[string]*WarmJob
}

// WarmJob represents a cache warming job
type WarmJob struct {
	Keys     []string
	Interval time.Duration
	LastRun  time.Time
	Active   bool
}

// NewCacheWarmerImpl creates a new cache warmer
func NewCacheWarmerImpl(manager *CacheManagerImpl, logger *zap.Logger) *CacheWarmerImpl {
	return &CacheWarmerImpl{
		manager:  manager,
		logger:   logger.With(zap.String("component", "cache_warmer")),
		stopChan: make(chan struct{}),
		warmJobs: make(map[string]*WarmJob),
	}
}

// WarmKeys preloads cache with specified keys
func (cw *CacheWarmerImpl) WarmKeys(ctx context.Context, keys []string) error {
	if len(keys) == 0 {
		return nil
	}

	cw.logger.Info("Starting cache warming", zap.Int("key_count", len(keys)))

	// Process keys in batches to avoid overwhelming the system
	batchSize := cw.manager.config.WarmupBatchSize
	if batchSize <= 0 {
		batchSize = 100 // Default batch size
	}

	var wg sync.WaitGroup
	semaphore := make(chan struct{}, 10) // Limit concurrent warming operations

	for i := 0; i < len(keys); i += batchSize {
		end := i + batchSize
		if end > len(keys) {
			end = len(keys)
		}

		batch := keys[i:end]
		wg.Add(1)

		go func(keyBatch []string) {
			defer wg.Done()
			semaphore <- struct{}{} // Acquire semaphore
			defer func() { <-semaphore }() // Release semaphore

			cw.warmBatch(ctx, keyBatch)
		}(batch)
	}

	wg.Wait()
	cw.logger.Info("Cache warming completed", zap.Int("key_count", len(keys)))
	return nil
}

// warmBatch warms a batch of keys
func (cw *CacheWarmerImpl) warmBatch(ctx context.Context, keys []string) {
	for _, key := range keys {
		select {
		case <-ctx.Done():
			return
		default:
			cw.warmSingleKey(ctx, key)
		}
	}
}

// warmSingleKey warms a single key by checking if it exists in L1, if not, fetch from L2
func (cw *CacheWarmerImpl) warmSingleKey(ctx context.Context, key string) {
	// Check if key exists in L1 cache
	exists, err := cw.manager.l1Cache.Exists(ctx, key)
	if err != nil {
		cw.logger.Debug("Failed to check L1 cache existence", zap.String("key", key), zap.Error(err))
		return
	}

	if exists {
		// Key already in L1, no need to warm
		return
	}

	// Try to get from L2 cache and populate L1
	value, err := cw.manager.l2Cache.Get(ctx, key)
	if err != nil {
		if err != ErrCacheNotFound {
			cw.logger.Debug("Failed to get from L2 cache during warming", zap.String("key", key), zap.Error(err))
		}
		return
	}

	// Populate L1 cache
	if err := cw.manager.l1Cache.Set(ctx, key, value, cw.manager.config.DefaultTTL); err != nil {
		cw.logger.Debug("Failed to populate L1 cache during warming", zap.String("key", key), zap.Error(err))
		return
	}

	cw.logger.Debug("Cache key warmed", zap.String("key", key))
}

// WarmPattern preloads cache with keys matching a pattern
func (cw *CacheWarmerImpl) WarmPattern(ctx context.Context, pattern string) error {
	// This would require scanning Redis for keys matching the pattern
	// For now, we'll implement a basic version
	cw.logger.Info("Warming cache by pattern", zap.String("pattern", pattern))

	// Get Redis client from L2 cache
	if redisCache, ok := cw.manager.l2Cache.(*RedisCacheImpl); ok {
		client := redisCache.GetClient()
		
		// Scan for keys matching pattern
		iter := client.Scan(ctx, 0, pattern, 0).Iterator()
		var keys []string
		
		for iter.Next(ctx) {
			key := iter.Val()
			// Remove prefix if it exists
			if len(key) > len(redisCache.prefix) && key[:len(redisCache.prefix)] == redisCache.prefix {
				key = key[len(redisCache.prefix):]
			}
			keys = append(keys, key)
		}
		
		if err := iter.Err(); err != nil {
			cw.logger.Error("Failed to scan keys for pattern warming", zap.String("pattern", pattern), zap.Error(err))
			return err
		}

		return cw.WarmKeys(ctx, keys)
	}

	cw.logger.Warn("Pattern warming not supported for current L2 cache implementation")
	return nil
}

// ScheduleWarming schedules periodic cache warming for specified keys
func (cw *CacheWarmerImpl) ScheduleWarming(ctx context.Context, keys []string, interval time.Duration) error {
	if len(keys) == 0 || interval <= 0 {
		return fmt.Errorf("invalid parameters for scheduled warming")
	}

	jobID := fmt.Sprintf("warm_%d", time.Now().UnixNano())
	
	cw.mu.Lock()
	cw.warmJobs[jobID] = &WarmJob{
		Keys:     keys,
		Interval: interval,
		LastRun:  time.Time{},
		Active:   true,
	}
	cw.mu.Unlock()

	cw.logger.Info("Scheduled cache warming", 
		zap.String("job_id", jobID),
		zap.Int("key_count", len(keys)),
		zap.Duration("interval", interval))

	// Start warming scheduler if not already running
	cw.startWarmingScheduler()

	return nil
}

// startWarmingScheduler starts the background warming scheduler
func (cw *CacheWarmerImpl) startWarmingScheduler() {
	if cw.warmTicker != nil {
		return // Already running
	}

	cw.warmTicker = time.NewTicker(1 * time.Minute) // Check every minute

	go func() {
		for {
			select {
			case <-cw.warmTicker.C:
				cw.processWarmingJobs()
			case <-cw.stopChan:
				return
			}
		}
	}()
}

// processWarmingJobs processes scheduled warming jobs
func (cw *CacheWarmerImpl) processWarmingJobs() {
	cw.mu.RLock()
	jobs := make(map[string]*WarmJob)
	for id, job := range cw.warmJobs {
		if job.Active {
			jobs[id] = job
		}
	}
	cw.mu.RUnlock()

	now := time.Now()
	for jobID, job := range jobs {
		if job.LastRun.IsZero() || now.Sub(job.LastRun) >= job.Interval {
			go func(id string, j *WarmJob) {
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
				defer cancel()

				if err := cw.WarmKeys(ctx, j.Keys); err != nil {
					cw.logger.Error("Scheduled warming failed", zap.String("job_id", id), zap.Error(err))
					return
				}

				cw.mu.Lock()
				if warmJob, exists := cw.warmJobs[id]; exists {
					warmJob.LastRun = now
				}
				cw.mu.Unlock()

				cw.logger.Debug("Scheduled warming completed", zap.String("job_id", id))
			}(jobID, job)
		}
	}
}

// StopWarming stops all warming operations
func (cw *CacheWarmerImpl) StopWarming(ctx context.Context) error {
	cw.mu.Lock()
	for jobID, job := range cw.warmJobs {
		job.Active = false
		cw.logger.Info("Stopped warming job", zap.String("job_id", jobID))
	}
	cw.mu.Unlock()

	if cw.warmTicker != nil {
		cw.warmTicker.Stop()
		cw.warmTicker = nil
	}

	close(cw.stopChan)
	cw.logger.Info("Cache warming stopped")
	return nil
}

// GetWarmingStats returns statistics about warming operations
func (cw *CacheWarmerImpl) GetWarmingStats() map[string]interface{} {
	cw.mu.RLock()
	defer cw.mu.RUnlock()

	stats := make(map[string]interface{})
	stats["active_jobs"] = len(cw.warmJobs)
	
	activeJobs := 0
	for _, job := range cw.warmJobs {
		if job.Active {
			activeJobs++
		}
	}
	stats["running_jobs"] = activeJobs

	return stats
}

// PredictiveWarming implements intelligent cache warming based on access patterns
func (cw *CacheWarmerImpl) PredictiveWarming(ctx context.Context) error {
	// This is a placeholder for predictive warming logic
	// In a real implementation, you would:
	// 1. Analyze access patterns from logs or metrics
	// 2. Identify frequently accessed keys
	// 3. Predict which keys are likely to be accessed soon
	// 4. Warm those keys proactively

	cw.logger.Info("Predictive warming not yet implemented")
	return nil
}