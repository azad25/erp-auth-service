package cache

import (
	"context"
	"time"
)

// CacheLevel represents different cache levels
type CacheLevel int

const (
	L1Cache CacheLevel = iota // In-memory BigCache
	L2Cache                   // Redis cluster
	L3Cache                   // Database (fallback)
)

// CacheStrategy defines caching behavior
type CacheStrategy int

const (
	WriteThrough CacheStrategy = iota // Write to cache and storage simultaneously
	WriteBack                         // Write to cache first, storage later
	ReadThrough                       // Read from cache, fallback to storage
	CacheAside                        // Application manages cache
)

// CacheEntry represents a cached item with metadata
type CacheEntry struct {
	Key        string
	Value      []byte
	TTL        time.Duration
	CreatedAt  time.Time
	AccessedAt time.Time
	HitCount   int64
	Version    int64
}

// CacheStats provides cache performance metrics
type CacheStats struct {
	Hits        int64
	Misses      int64
	Sets        int64
	Deletes     int64
	Evictions   int64
	Size        int64
	HitRatio    float64
	LastUpdated time.Time
}

// Cache defines the interface for all cache implementations
type Cache interface {
	Get(ctx context.Context, key string) ([]byte, error)
	Set(ctx context.Context, key string, value []byte, ttl time.Duration) error
	Delete(ctx context.Context, key string) error
	Exists(ctx context.Context, key string) (bool, error)
	Clear(ctx context.Context) error
	Stats() CacheStats
	Close() error
}

// MultiLevelCache defines interface for multi-level caching
type MultiLevelCache interface {
	Cache
	GetFromLevel(ctx context.Context, key string, level CacheLevel) ([]byte, error)
	SetToLevel(ctx context.Context, key string, value []byte, ttl time.Duration, level CacheLevel) error
	InvalidateKey(ctx context.Context, key string) error
	InvalidatePattern(ctx context.Context, pattern string) error
	WarmCache(ctx context.Context, keys []string) error
}

// CacheManager manages multiple cache instances and strategies
type CacheManager interface {
	MultiLevelCache
	GetCache(name string) Cache
	RegisterCache(name string, cache Cache) error
	SetStrategy(strategy CacheStrategy)
	GetStrategy() CacheStrategy
	Sync(ctx context.Context) error
}

// CacheWarmer handles proactive cache warming
type CacheWarmer interface {
	WarmKeys(ctx context.Context, keys []string) error
	WarmPattern(ctx context.Context, pattern string) error
	ScheduleWarming(ctx context.Context, keys []string, interval time.Duration) error
	StopWarming(ctx context.Context) error
}

// CacheInvalidator handles cache invalidation
type CacheInvalidator interface {
	InvalidateKey(ctx context.Context, key string) error
	InvalidatePattern(ctx context.Context, pattern string) error
	InvalidateByTags(ctx context.Context, tags []string) error
	ScheduleInvalidation(ctx context.Context, key string, delay time.Duration) error
}

// CacheConfig holds configuration for cache instances
type CacheConfig struct {
	// BigCache (L1) configuration
	BigCacheConfig BigCacheConfig

	// Redis (L2) configuration
	RedisConfig RedisClusterConfig

	// General cache settings
	DefaultTTL      time.Duration
	MaxRetries      int
	RetryDelay      time.Duration
	EnableMetrics   bool
	MetricsInterval time.Duration
	SyncInterval    time.Duration
	// WarmupInterval controls how often scheduled warming jobs are checked/run.
	// If zero, the warmer will default to a conservative interval to avoid
	// frequent work that can cause high CPU and logging noise.
	WarmupInterval    time.Duration
	WarmupEnabled     bool
	WarmupBatchSize   int
	InvalidationDelay time.Duration
}

// BigCacheConfig configuration for BigCache
type BigCacheConfig struct {
	Shards             int
	LifeWindow         time.Duration
	CleanWindow        time.Duration
	MaxEntriesInWindow int
	MaxEntrySize       int
	HardMaxCacheSize   int
	Verbose            bool
}

// RedisClusterConfig configuration for Redis cluster
type RedisClusterConfig struct {
	Addrs              []string
	Password           string
	DB                 int
	PoolSize           int
	MinIdleConns       int
	MaxConnAge         time.Duration
	PoolTimeout        time.Duration
	IdleTimeout        time.Duration
	IdleCheckFrequency time.Duration
	ReadTimeout        time.Duration
	WriteTimeout       time.Duration
	DialTimeout        time.Duration
}

// Error types for cache operations
var (
	ErrCacheNotFound    = NewCacheError("cache_not_found", "cache entry not found")
	ErrCacheExpired     = NewCacheError("cache_expired", "cache entry expired")
	ErrCacheUnavailable = NewCacheError("cache_unavailable", "cache service unavailable")
	ErrInvalidKey       = NewCacheError("invalid_key", "invalid cache key")
	ErrInvalidValue     = NewCacheError("invalid_value", "invalid cache value")
	ErrCacheFull        = NewCacheError("cache_full", "cache is full")
)

// CacheError represents cache-specific errors
type CacheError struct {
	Code    string
	Message string
	Cause   error
}

func (e *CacheError) Error() string {
	if e.Cause != nil {
		return e.Message + ": " + e.Cause.Error()
	}
	return e.Message
}

func NewCacheError(code, message string) *CacheError {
	return &CacheError{
		Code:    code,
		Message: message,
	}
}

func NewCacheErrorWithCause(code, message string, cause error) *CacheError {
	return &CacheError{
		Code:    code,
		Message: message,
		Cause:   cause,
	}
}
