package cache

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync/atomic"
	"time"

	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

// RedisCacheImpl implements Cache interface using Redis cluster for L2 caching
type RedisCacheImpl struct {
	client  redis.UniversalClient
	logger  *zap.Logger
	config  RedisClusterConfig
	stats   *cacheStats
	name    string
	prefix  string
}

// NewRedisCache creates a new Redis cache instance
func NewRedisCache(name string, config RedisClusterConfig, logger *zap.Logger) (*RedisCacheImpl, error) {
	var client redis.UniversalClient

	if len(config.Addrs) > 1 {
		// Use cluster client for multiple addresses
		client = redis.NewClusterClient(&redis.ClusterOptions{
			Addrs:        config.Addrs,
			Password:     config.Password,
			PoolSize:     config.PoolSize,
			MinIdleConns: config.MinIdleConns,
			PoolTimeout:  config.PoolTimeout,
			ReadTimeout:  config.ReadTimeout,
			WriteTimeout: config.WriteTimeout,
			DialTimeout:  config.DialTimeout,
		})
	} else {
		// Use single client for single address
		client = redis.NewClient(&redis.Options{
			Addr:         config.Addrs[0],
			Password:     config.Password,
			DB:           config.DB,
			PoolSize:     config.PoolSize,
			MinIdleConns: config.MinIdleConns,
			PoolTimeout:  config.PoolTimeout,
			ReadTimeout:  config.ReadTimeout,
			WriteTimeout: config.WriteTimeout,
			DialTimeout:  config.DialTimeout,
		})
	}

	// Test connection
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := client.Ping(ctx).Err(); err != nil {
		return nil, NewCacheErrorWithCause("redis_connection_failed", "failed to connect to Redis", err)
	}

	return &RedisCacheImpl{
		client: client,
		logger: logger.With(zap.String("cache", name)),
		config: config,
		stats:  &cacheStats{},
		name:   name,
		prefix: fmt.Sprintf("%s:", name),
	}, nil
}

// Get retrieves a value from Redis
func (r *RedisCacheImpl) Get(ctx context.Context, key string) ([]byte, error) {
	if key == "" {
		return nil, ErrInvalidKey
	}

	fullKey := r.prefix + key
	result, err := r.client.Get(ctx, fullKey).Result()
	if err != nil {
		atomic.AddInt64(&r.stats.misses, 1)
		if err == redis.Nil {
			return nil, ErrCacheNotFound
		}
		r.logger.Error("Redis get failed", zap.String("key", key), zap.Error(err))
		return nil, NewCacheErrorWithCause("redis_get_failed", "failed to get from Redis", err)
	}

	var cacheEntry CacheEntry
	if err := json.Unmarshal([]byte(result), &cacheEntry); err != nil {
		atomic.AddInt64(&r.stats.misses, 1)
		r.logger.Error("Failed to unmarshal cache entry", zap.String("key", key), zap.Error(err))
		return nil, NewCacheErrorWithCause("unmarshal_failed", "failed to unmarshal cache entry", err)
	}

	// Update access metadata
	cacheEntry.AccessedAt = time.Now()
	atomic.AddInt64(&cacheEntry.HitCount, 1)

	// Update entry in Redis with new metadata
	updatedEntry, _ := json.Marshal(cacheEntry)
	r.client.Set(ctx, fullKey, updatedEntry, time.Until(cacheEntry.CreatedAt.Add(cacheEntry.TTL)))

	atomic.AddInt64(&r.stats.hits, 1)
	return cacheEntry.Value, nil
}

// Set stores a value in Redis
func (r *RedisCacheImpl) Set(ctx context.Context, key string, value []byte, ttl time.Duration) error {
	if key == "" {
		return ErrInvalidKey
	}
	if value == nil {
		return ErrInvalidValue
	}

	cacheEntry := CacheEntry{
		Key:        key,
		Value:      value,
		TTL:        ttl,
		CreatedAt:  time.Now(),
		AccessedAt: time.Now(),
		HitCount:   0,
		Version:    1,
	}

	entryBytes, err := json.Marshal(cacheEntry)
	if err != nil {
		r.logger.Error("Failed to marshal cache entry", zap.String("key", key), zap.Error(err))
		return NewCacheErrorWithCause("marshal_failed", "failed to marshal cache entry", err)
	}

	fullKey := r.prefix + key
	if err := r.client.Set(ctx, fullKey, entryBytes, ttl).Err(); err != nil {
		r.logger.Error("Redis set failed", zap.String("key", key), zap.Error(err))
		return NewCacheErrorWithCause("redis_set_failed", "failed to set in Redis", err)
	}

	atomic.AddInt64(&r.stats.sets, 1)
	r.logger.Debug("Set cache entry", zap.String("key", key), zap.Duration("ttl", ttl))
	return nil
}

// Delete removes a value from Redis
func (r *RedisCacheImpl) Delete(ctx context.Context, key string) error {
	if key == "" {
		return ErrInvalidKey
	}

	fullKey := r.prefix + key
	result, err := r.client.Del(ctx, fullKey).Result()
	if err != nil {
		r.logger.Error("Redis delete failed", zap.String("key", key), zap.Error(err))
		return NewCacheErrorWithCause("redis_delete_failed", "failed to delete from Redis", err)
	}

	if result == 0 {
		return ErrCacheNotFound
	}

	atomic.AddInt64(&r.stats.deletes, 1)
	r.logger.Debug("Deleted cache entry", zap.String("key", key))
	return nil
}

// Exists checks if a key exists in Redis
func (r *RedisCacheImpl) Exists(ctx context.Context, key string) (bool, error) {
	if key == "" {
		return false, ErrInvalidKey
	}

	fullKey := r.prefix + key
	result, err := r.client.Exists(ctx, fullKey).Result()
	if err != nil {
		r.logger.Error("Redis exists failed", zap.String("key", key), zap.Error(err))
		return false, NewCacheErrorWithCause("redis_exists_failed", "failed to check existence in Redis", err)
	}

	return result > 0, nil
}

// Clear removes all entries with the cache prefix from Redis
func (r *RedisCacheImpl) Clear(ctx context.Context) error {
	pattern := r.prefix + "*"
	
	// Use SCAN to find all keys with the prefix
	iter := r.client.Scan(ctx, 0, pattern, 0).Iterator()
	var keys []string
	
	for iter.Next(ctx) {
		keys = append(keys, iter.Val())
	}
	
	if err := iter.Err(); err != nil {
		r.logger.Error("Redis scan failed", zap.Error(err))
		return NewCacheErrorWithCause("redis_scan_failed", "failed to scan Redis keys", err)
	}

	if len(keys) > 0 {
		if err := r.client.Del(ctx, keys...).Err(); err != nil {
			r.logger.Error("Redis clear failed", zap.Error(err))
			return NewCacheErrorWithCause("redis_clear_failed", "failed to clear Redis cache", err)
		}
	}

	// Reset stats
	atomic.StoreInt64(&r.stats.hits, 0)
	atomic.StoreInt64(&r.stats.misses, 0)
	atomic.StoreInt64(&r.stats.sets, 0)
	atomic.StoreInt64(&r.stats.deletes, 0)
	atomic.StoreInt64(&r.stats.evictions, 0)

	r.logger.Info("Cleared Redis cache", zap.Int("keys_deleted", len(keys)))
	return nil
}

// Stats returns cache performance statistics
func (r *RedisCacheImpl) Stats() CacheStats {
	hits := atomic.LoadInt64(&r.stats.hits)
	misses := atomic.LoadInt64(&r.stats.misses)
	total := hits + misses

	var hitRatio float64
	if total > 0 {
		hitRatio = float64(hits) / float64(total)
	}

	// Get approximate size from Redis
	var size int64
	if info, err := r.client.Info(context.Background(), "memory").Result(); err == nil {
		// Parse memory info to get approximate size
		lines := strings.Split(info, "\r\n")
		for _, line := range lines {
			if strings.HasPrefix(line, "used_memory:") {
				// This is approximate - includes all Redis memory usage
				size = 0 // We can't easily get per-prefix memory usage
				break
			}
		}
	}

	return CacheStats{
		Hits:        hits,
		Misses:      misses,
		Sets:        atomic.LoadInt64(&r.stats.sets),
		Deletes:     atomic.LoadInt64(&r.stats.deletes),
		Evictions:   atomic.LoadInt64(&r.stats.evictions),
		Size:        size,
		HitRatio:    hitRatio,
		LastUpdated: time.Now(),
	}
}

// Close closes the Redis client
func (r *RedisCacheImpl) Close() error {
	if err := r.client.Close(); err != nil {
		r.logger.Error("Failed to close Redis client", zap.Error(err))
		return NewCacheErrorWithCause("redis_close_failed", "failed to close Redis client", err)
	}

	r.logger.Info("Redis cache closed")
	return nil
}

// DeletePattern removes all keys matching a pattern
func (r *RedisCacheImpl) DeletePattern(ctx context.Context, pattern string) error {
	fullPattern := r.prefix + pattern
	
	iter := r.client.Scan(ctx, 0, fullPattern, 0).Iterator()
	var keys []string
	
	for iter.Next(ctx) {
		keys = append(keys, iter.Val())
	}
	
	if err := iter.Err(); err != nil {
		return NewCacheErrorWithCause("redis_scan_failed", "failed to scan Redis keys", err)
	}

	if len(keys) > 0 {
		if err := r.client.Del(ctx, keys...).Err(); err != nil {
			return NewCacheErrorWithCause("redis_delete_pattern_failed", "failed to delete pattern from Redis", err)
		}
		atomic.AddInt64(&r.stats.deletes, int64(len(keys)))
	}

	r.logger.Debug("Deleted keys by pattern", zap.String("pattern", pattern), zap.Int("count", len(keys)))
	return nil
}

// SetWithVersion stores a value with version for optimistic locking
func (r *RedisCacheImpl) SetWithVersion(ctx context.Context, key string, value []byte, ttl time.Duration, version int64) error {
	if key == "" {
		return ErrInvalidKey
	}

	cacheEntry := CacheEntry{
		Key:        key,
		Value:      value,
		TTL:        ttl,
		CreatedAt:  time.Now(),
		AccessedAt: time.Now(),
		HitCount:   0,
		Version:    version,
	}

	entryBytes, err := json.Marshal(cacheEntry)
	if err != nil {
		return NewCacheErrorWithCause("marshal_failed", "failed to marshal cache entry", err)
	}

	fullKey := r.prefix + key
	if err := r.client.Set(ctx, fullKey, entryBytes, ttl).Err(); err != nil {
		return NewCacheErrorWithCause("redis_set_failed", "failed to set in Redis", err)
	}

	atomic.AddInt64(&r.stats.sets, 1)
	return nil
}

// GetClient returns the underlying Redis client for advanced operations
func (r *RedisCacheImpl) GetClient() redis.UniversalClient {
	return r.client
}