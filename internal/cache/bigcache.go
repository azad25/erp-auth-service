package cache

import (
	"context"
	"encoding/json"
	"sync/atomic"
	"time"

	"github.com/allegro/bigcache/v3"
	"go.uber.org/zap"
)

// BigCacheImpl implements Cache interface using BigCache for L1 caching
type BigCacheImpl struct {
	cache   *bigcache.BigCache
	logger  *zap.Logger
	config  BigCacheConfig
	stats   *cacheStats
	name    string
}

// cacheStats tracks cache performance metrics
type cacheStats struct {
	hits      int64
	misses    int64
	sets      int64
	deletes   int64
	evictions int64
}

// NewBigCache creates a new BigCache instance
func NewBigCache(name string, config BigCacheConfig, logger *zap.Logger) (*BigCacheImpl, error) {
	bigCacheConfig := bigcache.Config{
		Shards:             config.Shards,
		LifeWindow:         config.LifeWindow,
		CleanWindow:        config.CleanWindow,
		MaxEntriesInWindow: config.MaxEntriesInWindow,
		MaxEntrySize:       config.MaxEntrySize,
		HardMaxCacheSize:   config.HardMaxCacheSize,
		Verbose:            config.Verbose,
		OnRemove: func(key string, entry []byte) {
			// Track evictions - this will be handled by the cache instance
		},
	}

	cache, err := bigcache.New(context.Background(), bigCacheConfig)
	if err != nil {
		return nil, NewCacheErrorWithCause("bigcache_init_failed", "failed to initialize BigCache", err)
	}

	return &BigCacheImpl{
		cache:  cache,
		logger: logger.With(zap.String("cache", name)),
		config: config,
		stats:  &cacheStats{},
		name:   name,
	}, nil
}

// Get retrieves a value from BigCache
func (b *BigCacheImpl) Get(ctx context.Context, key string) ([]byte, error) {
	if key == "" {
		return nil, ErrInvalidKey
	}

	entry, err := b.cache.Get(key)
	if err != nil {
		atomic.AddInt64(&b.stats.misses, 1)
		if err == bigcache.ErrEntryNotFound {
			return nil, ErrCacheNotFound
		}
		b.logger.Error("BigCache get failed", zap.String("key", key), zap.Error(err))
		return nil, NewCacheErrorWithCause("bigcache_get_failed", "failed to get from BigCache", err)
	}

	// Check if entry has expired (BigCache doesn't handle TTL per entry)
	var cacheEntry CacheEntry
	if err := json.Unmarshal(entry, &cacheEntry); err != nil {
		atomic.AddInt64(&b.stats.misses, 1)
		b.logger.Error("Failed to unmarshal cache entry", zap.String("key", key), zap.Error(err))
		return nil, NewCacheErrorWithCause("unmarshal_failed", "failed to unmarshal cache entry", err)
	}

	// Check TTL
	if cacheEntry.TTL > 0 && time.Since(cacheEntry.CreatedAt) > cacheEntry.TTL {
		// Entry expired, remove it
		b.cache.Delete(key)
		atomic.AddInt64(&b.stats.misses, 1)
		return nil, ErrCacheExpired
	}

	// Update access time and hit count
	cacheEntry.AccessedAt = time.Now()
	atomic.AddInt64(&cacheEntry.HitCount, 1)

	// Re-store with updated metadata
	updatedEntry, _ := json.Marshal(cacheEntry)
	b.cache.Set(key, updatedEntry)

	atomic.AddInt64(&b.stats.hits, 1)
	return cacheEntry.Value, nil
}

// Set stores a value in BigCache
func (b *BigCacheImpl) Set(ctx context.Context, key string, value []byte, ttl time.Duration) error {
	if key == "" {
		return ErrInvalidKey
	}
	if value == nil {
		return ErrInvalidValue
	}

	cacheEntry := CacheEntry{
		Key:       key,
		Value:     value,
		TTL:       ttl,
		CreatedAt: time.Now(),
		AccessedAt: time.Now(),
		HitCount:  0,
		Version:   1,
	}

	entryBytes, err := json.Marshal(cacheEntry)
	if err != nil {
		b.logger.Error("Failed to marshal cache entry", zap.String("key", key), zap.Error(err))
		return NewCacheErrorWithCause("marshal_failed", "failed to marshal cache entry", err)
	}

	if err := b.cache.Set(key, entryBytes); err != nil {
		b.logger.Error("BigCache set failed", zap.String("key", key), zap.Error(err))
		return NewCacheErrorWithCause("bigcache_set_failed", "failed to set in BigCache", err)
	}

	atomic.AddInt64(&b.stats.sets, 1)
	b.logger.Debug("Set cache entry", zap.String("key", key), zap.Duration("ttl", ttl))
	return nil
}

// Delete removes a value from BigCache
func (b *BigCacheImpl) Delete(ctx context.Context, key string) error {
	if key == "" {
		return ErrInvalidKey
	}

	if err := b.cache.Delete(key); err != nil {
		if err == bigcache.ErrEntryNotFound {
			return ErrCacheNotFound
		}
		b.logger.Error("BigCache delete failed", zap.String("key", key), zap.Error(err))
		return NewCacheErrorWithCause("bigcache_delete_failed", "failed to delete from BigCache", err)
	}

	atomic.AddInt64(&b.stats.deletes, 1)
	b.logger.Debug("Deleted cache entry", zap.String("key", key))
	return nil
}

// Exists checks if a key exists in BigCache
func (b *BigCacheImpl) Exists(ctx context.Context, key string) (bool, error) {
	if key == "" {
		return false, ErrInvalidKey
	}

	_, err := b.cache.Get(key)
	if err != nil {
		if err == bigcache.ErrEntryNotFound {
			return false, nil
		}
		return false, NewCacheErrorWithCause("bigcache_exists_failed", "failed to check existence in BigCache", err)
	}

	return true, nil
}

// Clear removes all entries from BigCache
func (b *BigCacheImpl) Clear(ctx context.Context) error {
	if err := b.cache.Reset(); err != nil {
		b.logger.Error("BigCache clear failed", zap.Error(err))
		return NewCacheErrorWithCause("bigcache_clear_failed", "failed to clear BigCache", err)
	}

	// Reset stats
	atomic.StoreInt64(&b.stats.hits, 0)
	atomic.StoreInt64(&b.stats.misses, 0)
	atomic.StoreInt64(&b.stats.sets, 0)
	atomic.StoreInt64(&b.stats.deletes, 0)
	atomic.StoreInt64(&b.stats.evictions, 0)

	b.logger.Info("Cleared BigCache")
	return nil
}

// Stats returns cache performance statistics
func (b *BigCacheImpl) Stats() CacheStats {
	bigCacheStats := b.cache.Stats()
	hits := atomic.LoadInt64(&b.stats.hits)
	misses := atomic.LoadInt64(&b.stats.misses)
	total := hits + misses

	var hitRatio float64
	if total > 0 {
		hitRatio = float64(hits) / float64(total)
	}

	return CacheStats{
		Hits:        hits,
		Misses:      misses,
		Sets:        atomic.LoadInt64(&b.stats.sets),
		Deletes:     atomic.LoadInt64(&b.stats.deletes),
		Evictions:   atomic.LoadInt64(&b.stats.evictions),
		Size:        int64(bigCacheStats.Hits + bigCacheStats.Misses), // Approximate
		HitRatio:    hitRatio,
		LastUpdated: time.Now(),
	}
}

// Close closes the BigCache instance
func (b *BigCacheImpl) Close() error {
	if err := b.cache.Close(); err != nil {
		b.logger.Error("Failed to close BigCache", zap.Error(err))
		return NewCacheErrorWithCause("bigcache_close_failed", "failed to close BigCache", err)
	}

	b.logger.Info("BigCache closed")
	return nil
}

// GetCapacity returns the current capacity information
func (b *BigCacheImpl) GetCapacity() (int, int) {
	stats := b.cache.Stats()
	return int(stats.Hits + stats.Misses), b.config.HardMaxCacheSize
}

// Len returns the number of entries in cache
func (b *BigCacheImpl) Len() int {
	return b.cache.Len()
}