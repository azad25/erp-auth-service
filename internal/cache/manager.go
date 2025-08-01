package cache

import (
	"context"
	"fmt"
	"sync"
	"time"

	"go.uber.org/zap"
)

// CacheManagerImpl implements CacheManager interface
type CacheManagerImpl struct {
	l1Cache     Cache // BigCache
	l2Cache     Cache // Redis
	caches      map[string]Cache
	strategy    CacheStrategy
	config      CacheConfig
	logger      *zap.Logger
	warmer      CacheWarmer
	invalidator CacheInvalidator
	mu          sync.RWMutex
	syncTicker  *time.Ticker
	stopChan    chan struct{}
}

// NewCacheManager creates a new cache manager with multi-level caching
func NewCacheManager(config CacheConfig, logger *zap.Logger) (*CacheManagerImpl, error) {
	// Initialize L1 cache (BigCache)
	l1Cache, err := NewBigCache("l1", config.BigCacheConfig, logger)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize L1 cache: %w", err)
	}

	// Initialize L2 cache (Redis)
	l2Cache, err := NewRedisCache("l2", config.RedisConfig, logger)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize L2 cache: %w", err)
	}

	manager := &CacheManagerImpl{
		l1Cache:  l1Cache,
		l2Cache:  l2Cache,
		caches:   make(map[string]Cache),
		strategy: WriteThrough, // Default strategy
		config:   config,
		logger:   logger.With(zap.String("component", "cache_manager")),
		stopChan: make(chan struct{}),
	}

	// Register default caches
	manager.caches["l1"] = l1Cache
	manager.caches["l2"] = l2Cache

	// Initialize cache warmer and invalidator
	manager.warmer = NewCacheWarmerImpl(manager, logger)
	manager.invalidator = NewCacheInvalidatorImpl(manager, logger)

	// Start background sync if enabled
	if config.SyncInterval > 0 {
		manager.startSync()
	}

	return manager, nil
}

// Get retrieves a value using multi-level caching strategy
func (cm *CacheManagerImpl) Get(ctx context.Context, key string) ([]byte, error) {
	if key == "" {
		return nil, ErrInvalidKey
	}

	// Try L1 cache first
	if value, err := cm.l1Cache.Get(ctx, key); err == nil {
		cm.logger.Debug("Cache hit L1", zap.String("key", key))
		return value, nil
	}

	// Try L2 cache
	value, err := cm.l2Cache.Get(ctx, key)
	if err != nil {
		if err == ErrCacheNotFound {
			cm.logger.Debug("Cache miss L1+L2", zap.String("key", key))
			return nil, ErrCacheNotFound
		}
		return nil, err
	}

	// Found in L2, populate L1 cache asynchronously
	go func() {
		if err := cm.l1Cache.Set(context.Background(), key, value, cm.config.DefaultTTL); err != nil {
			cm.logger.Warn("Failed to populate L1 cache", zap.String("key", key), zap.Error(err))
		}
	}()

	cm.logger.Debug("Cache hit L2", zap.String("key", key))
	return value, nil
}

// Set stores a value using the configured caching strategy
func (cm *CacheManagerImpl) Set(ctx context.Context, key string, value []byte, ttl time.Duration) error {
	if key == "" {
		return ErrInvalidKey
	}
	if value == nil {
		return ErrInvalidValue
	}

	if ttl == 0 {
		ttl = cm.config.DefaultTTL
	}

	switch cm.strategy {
	case WriteThrough:
		return cm.setWriteThrough(ctx, key, value, ttl)
	case WriteBack:
		return cm.setWriteBack(ctx, key, value, ttl)
	case ReadThrough:
		return cm.setReadThrough(ctx, key, value, ttl)
	case CacheAside:
		return cm.setCacheAside(ctx, key, value, ttl)
	default:
		return cm.setWriteThrough(ctx, key, value, ttl)
	}
}

// setWriteThrough implements write-through caching strategy
func (cm *CacheManagerImpl) setWriteThrough(ctx context.Context, key string, value []byte, ttl time.Duration) error {
	// Write to both L1 and L2 simultaneously
	errChan := make(chan error, 2)

	go func() {
		errChan <- cm.l1Cache.Set(ctx, key, value, ttl)
	}()

	go func() {
		errChan <- cm.l2Cache.Set(ctx, key, value, ttl)
	}()

	// Wait for both operations to complete
	var errors []error
	for i := 0; i < 2; i++ {
		if err := <-errChan; err != nil {
			errors = append(errors, err)
		}
	}

	if len(errors) > 0 {
		cm.logger.Error("Write-through cache set failed", zap.String("key", key), zap.Errors("errors", errors))
		return errors[0] // Return first error
	}

	cm.logger.Debug("Write-through cache set", zap.String("key", key))
	return nil
}

// setWriteBack implements write-back caching strategy
func (cm *CacheManagerImpl) setWriteBack(ctx context.Context, key string, value []byte, ttl time.Duration) error {
	// Write to L1 immediately
	if err := cm.l1Cache.Set(ctx, key, value, ttl); err != nil {
		return err
	}

	// Write to L2 asynchronously
	go func() {
		if err := cm.l2Cache.Set(context.Background(), key, value, ttl); err != nil {
			cm.logger.Warn("Write-back L2 cache set failed", zap.String("key", key), zap.Error(err))
		}
	}()

	cm.logger.Debug("Write-back cache set", zap.String("key", key))
	return nil
}

// setReadThrough implements read-through caching strategy
func (cm *CacheManagerImpl) setReadThrough(ctx context.Context, key string, value []byte, ttl time.Duration) error {
	// For read-through, we typically set in both caches
	return cm.setWriteThrough(ctx, key, value, ttl)
}

// setCacheAside implements cache-aside strategy
func (cm *CacheManagerImpl) setCacheAside(ctx context.Context, key string, value []byte, ttl time.Duration) error {
	// Application manages cache, so we set in both
	return cm.setWriteThrough(ctx, key, value, ttl)
}

// Delete removes a value from all cache levels
func (cm *CacheManagerImpl) Delete(ctx context.Context, key string) error {
	if key == "" {
		return ErrInvalidKey
	}

	errChan := make(chan error, 2)

	go func() {
		errChan <- cm.l1Cache.Delete(ctx, key)
	}()

	go func() {
		errChan <- cm.l2Cache.Delete(ctx, key)
	}()

	// Collect errors but don't fail if key doesn't exist
	var errors []error
	for i := 0; i < 2; i++ {
		if err := <-errChan; err != nil && err != ErrCacheNotFound {
			errors = append(errors, err)
		}
	}

	if len(errors) > 0 {
		cm.logger.Error("Cache delete failed", zap.String("key", key), zap.Errors("errors", errors))
		return errors[0]
	}

	cm.logger.Debug("Cache delete", zap.String("key", key))
	return nil
}

// Exists checks if a key exists in any cache level
func (cm *CacheManagerImpl) Exists(ctx context.Context, key string) (bool, error) {
	if key == "" {
		return false, ErrInvalidKey
	}

	// Check L1 first
	if exists, err := cm.l1Cache.Exists(ctx, key); err == nil && exists {
		return true, nil
	}

	// Check L2
	return cm.l2Cache.Exists(ctx, key)
}

// Clear removes all entries from all cache levels
func (cm *CacheManagerImpl) Clear(ctx context.Context) error {
	errChan := make(chan error, 2)

	go func() {
		errChan <- cm.l1Cache.Clear(ctx)
	}()

	go func() {
		errChan <- cm.l2Cache.Clear(ctx)
	}()

	var errors []error
	for i := 0; i < 2; i++ {
		if err := <-errChan; err != nil {
			errors = append(errors, err)
		}
	}

	if len(errors) > 0 {
		cm.logger.Error("Cache clear failed", zap.Errors("errors", errors))
		return errors[0]
	}

	cm.logger.Info("Cache cleared")
	return nil
}

// GetFromLevel retrieves a value from a specific cache level
func (cm *CacheManagerImpl) GetFromLevel(ctx context.Context, key string, level CacheLevel) ([]byte, error) {
	switch level {
	case L1Cache:
		return cm.l1Cache.Get(ctx, key)
	case L2Cache:
		return cm.l2Cache.Get(ctx, key)
	default:
		return nil, fmt.Errorf("unsupported cache level: %d", level)
	}
}

// SetToLevel stores a value in a specific cache level
func (cm *CacheManagerImpl) SetToLevel(ctx context.Context, key string, value []byte, ttl time.Duration, level CacheLevel) error {
	switch level {
	case L1Cache:
		return cm.l1Cache.Set(ctx, key, value, ttl)
	case L2Cache:
		return cm.l2Cache.Set(ctx, key, value, ttl)
	default:
		return fmt.Errorf("unsupported cache level: %d", level)
	}
}

// InvalidateKey removes a key from all cache levels
func (cm *CacheManagerImpl) InvalidateKey(ctx context.Context, key string) error {
	return cm.invalidator.InvalidateKey(ctx, key)
}

// InvalidatePattern removes all keys matching a pattern from all cache levels
func (cm *CacheManagerImpl) InvalidatePattern(ctx context.Context, pattern string) error {
	return cm.invalidator.InvalidatePattern(ctx, pattern)
}

// WarmCache preloads cache with specified keys
func (cm *CacheManagerImpl) WarmCache(ctx context.Context, keys []string) error {
	return cm.warmer.WarmKeys(ctx, keys)
}

// Stats returns combined cache statistics
func (cm *CacheManagerImpl) Stats() CacheStats {
	l1Stats := cm.l1Cache.Stats()
	l2Stats := cm.l2Cache.Stats()

	totalHits := l1Stats.Hits + l2Stats.Hits
	totalMisses := l1Stats.Misses + l2Stats.Misses
	total := totalHits + totalMisses

	var hitRatio float64
	if total > 0 {
		hitRatio = float64(totalHits) / float64(total)
	}

	return CacheStats{
		Hits:        totalHits,
		Misses:      totalMisses,
		Sets:        l1Stats.Sets + l2Stats.Sets,
		Deletes:     l1Stats.Deletes + l2Stats.Deletes,
		Evictions:   l1Stats.Evictions + l2Stats.Evictions,
		Size:        l1Stats.Size + l2Stats.Size,
		HitRatio:    hitRatio,
		LastUpdated: time.Now(),
	}
}

// GetCache returns a specific cache instance by name
func (cm *CacheManagerImpl) GetCache(name string) Cache {
	cm.mu.RLock()
	defer cm.mu.RUnlock()
	return cm.caches[name]
}

// RegisterCache registers a new cache instance
func (cm *CacheManagerImpl) RegisterCache(name string, cache Cache) error {
	cm.mu.Lock()
	defer cm.mu.Unlock()
	
	if _, exists := cm.caches[name]; exists {
		return fmt.Errorf("cache %s already registered", name)
	}
	
	cm.caches[name] = cache
	cm.logger.Info("Cache registered", zap.String("name", name))
	return nil
}

// SetStrategy sets the caching strategy
func (cm *CacheManagerImpl) SetStrategy(strategy CacheStrategy) {
	cm.strategy = strategy
	cm.logger.Info("Cache strategy changed", zap.Int("strategy", int(strategy)))
}

// GetStrategy returns the current caching strategy
func (cm *CacheManagerImpl) GetStrategy() CacheStrategy {
	return cm.strategy
}

// Sync synchronizes cache levels
func (cm *CacheManagerImpl) Sync(ctx context.Context) error {
	// This is a placeholder for cache synchronization logic
	// In a real implementation, you might sync L1 and L2 caches
	cm.logger.Debug("Cache sync completed")
	return nil
}

// startSync starts background cache synchronization
func (cm *CacheManagerImpl) startSync() {
	cm.syncTicker = time.NewTicker(cm.config.SyncInterval)
	
	go func() {
		for {
			select {
			case <-cm.syncTicker.C:
				if err := cm.Sync(context.Background()); err != nil {
					cm.logger.Error("Cache sync failed", zap.Error(err))
				}
			case <-cm.stopChan:
				return
			}
		}
	}()
}

// Close closes all cache instances and stops background processes
func (cm *CacheManagerImpl) Close() error {
	// Stop background processes
	if cm.syncTicker != nil {
		cm.syncTicker.Stop()
	}
	close(cm.stopChan)

	// Close all caches
	var errors []error
	
	if err := cm.l1Cache.Close(); err != nil {
		errors = append(errors, err)
	}
	
	if err := cm.l2Cache.Close(); err != nil {
		errors = append(errors, err)
	}

	if len(errors) > 0 {
		return errors[0]
	}

	cm.logger.Info("Cache manager closed")
	return nil
}