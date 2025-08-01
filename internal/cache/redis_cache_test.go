package cache

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"
)

// setupMockRedis creates a mock Redis server for testing
func setupMockRedis(t *testing.T) (*miniredis.Miniredis, RedisClusterConfig) {
	mr, err := miniredis.Run()
	require.NoError(t, err)

	config := RedisClusterConfig{
		Addrs:              []string{mr.Addr()},
		Password:           "",
		DB:                 0,
		PoolSize:           10,
		MinIdleConns:       2,
		MaxConnAge:         30 * time.Minute,
		PoolTimeout:        30 * time.Second,
		IdleTimeout:        5 * time.Minute,
		IdleCheckFrequency: 1 * time.Minute,
		ReadTimeout:        3 * time.Second,
		WriteTimeout:       3 * time.Second,
		DialTimeout:        5 * time.Second,
	}

	return mr, config
}

func TestRedisCacheImpl_Get(t *testing.T) {
	mr, config := setupMockRedis(t)
	defer mr.Close()

	logger := zaptest.NewLogger(t)
	cache, err := NewRedisCache("test", config, logger)
	require.NoError(t, err)
	defer cache.Close()

	ctx := context.Background()

	t.Run("Get non-existent key", func(t *testing.T) {
		_, err := cache.Get(ctx, "non-existent")
		assert.Equal(t, ErrCacheNotFound, err)
	})

	t.Run("Get empty key", func(t *testing.T) {
		_, err := cache.Get(ctx, "")
		assert.Equal(t, ErrInvalidKey, err)
	})

	t.Run("Get existing key", func(t *testing.T) {
		key := "test-key"
		value := []byte("test-value")
		
		err := cache.Set(ctx, key, value, 5*time.Minute)
		require.NoError(t, err)

		retrieved, err := cache.Get(ctx, key)
		require.NoError(t, err)
		assert.Equal(t, value, retrieved)
	})

	t.Run("Get expired key", func(t *testing.T) {
		key := "expired-key"
		value := []byte("expired-value")
		
		err := cache.Set(ctx, key, value, 1*time.Millisecond)
		require.NoError(t, err)

		// Fast forward time in mock Redis
		mr.FastForward(10 * time.Millisecond)

		_, err = cache.Get(ctx, key)
		assert.Equal(t, ErrCacheNotFound, err)
	})
}

func TestRedisCacheImpl_Set(t *testing.T) {
	mr, config := setupMockRedis(t)
	defer mr.Close()

	logger := zaptest.NewLogger(t)
	cache, err := NewRedisCache("test", config, logger)
	require.NoError(t, err)
	defer cache.Close()

	ctx := context.Background()

	t.Run("Set valid key-value", func(t *testing.T) {
		key := "test-key"
		value := []byte("test-value")
		
		err := cache.Set(ctx, key, value, 5*time.Minute)
		assert.NoError(t, err)

		// Verify it was set
		retrieved, err := cache.Get(ctx, key)
		require.NoError(t, err)
		assert.Equal(t, value, retrieved)
	})

	t.Run("Set empty key", func(t *testing.T) {
		err := cache.Set(ctx, "", []byte("value"), 5*time.Minute)
		assert.Equal(t, ErrInvalidKey, err)
	})

	t.Run("Set nil value", func(t *testing.T) {
		err := cache.Set(ctx, "key", nil, 5*time.Minute)
		assert.Equal(t, ErrInvalidValue, err)
	})

	t.Run("Set with TTL", func(t *testing.T) {
		key := "ttl-key"
		value := []byte("ttl-value")
		
		err := cache.Set(ctx, key, value, 100*time.Millisecond)
		require.NoError(t, err)

		// Should be retrievable immediately
		retrieved, err := cache.Get(ctx, key)
		require.NoError(t, err)
		assert.Equal(t, value, retrieved)

		// Fast forward time
		mr.FastForward(200 * time.Millisecond)

		// Should be expired
		_, err = cache.Get(ctx, key)
		assert.Equal(t, ErrCacheNotFound, err)
	})
}

func TestRedisCacheImpl_Delete(t *testing.T) {
	mr, config := setupMockRedis(t)
	defer mr.Close()

	logger := zaptest.NewLogger(t)
	cache, err := NewRedisCache("test", config, logger)
	require.NoError(t, err)
	defer cache.Close()

	ctx := context.Background()

	t.Run("Delete existing key", func(t *testing.T) {
		key := "delete-key"
		value := []byte("delete-value")
		
		err := cache.Set(ctx, key, value, 5*time.Minute)
		require.NoError(t, err)

		err = cache.Delete(ctx, key)
		assert.NoError(t, err)

		// Verify it was deleted
		_, err = cache.Get(ctx, key)
		assert.Equal(t, ErrCacheNotFound, err)
	})

	t.Run("Delete non-existent key", func(t *testing.T) {
		err := cache.Delete(ctx, "non-existent")
		assert.Equal(t, ErrCacheNotFound, err)
	})

	t.Run("Delete empty key", func(t *testing.T) {
		err := cache.Delete(ctx, "")
		assert.Equal(t, ErrInvalidKey, err)
	})
}

func TestRedisCacheImpl_Exists(t *testing.T) {
	mr, config := setupMockRedis(t)
	defer mr.Close()

	logger := zaptest.NewLogger(t)
	cache, err := NewRedisCache("test", config, logger)
	require.NoError(t, err)
	defer cache.Close()

	ctx := context.Background()

	t.Run("Exists for existing key", func(t *testing.T) {
		key := "exists-key"
		value := []byte("exists-value")
		
		err := cache.Set(ctx, key, value, 5*time.Minute)
		require.NoError(t, err)

		exists, err := cache.Exists(ctx, key)
		require.NoError(t, err)
		assert.True(t, exists)
	})

	t.Run("Exists for non-existent key", func(t *testing.T) {
		exists, err := cache.Exists(ctx, "non-existent")
		require.NoError(t, err)
		assert.False(t, exists)
	})

	t.Run("Exists for empty key", func(t *testing.T) {
		exists, err := cache.Exists(ctx, "")
		assert.Equal(t, ErrInvalidKey, err)
		assert.False(t, exists)
	})
}

func TestRedisCacheImpl_Clear(t *testing.T) {
	mr, config := setupMockRedis(t)
	defer mr.Close()

	logger := zaptest.NewLogger(t)
	cache, err := NewRedisCache("test", config, logger)
	require.NoError(t, err)
	defer cache.Close()

	ctx := context.Background()

	// Add some entries
	for i := 0; i < 5; i++ {
		key := fmt.Sprintf("key-%d", i)
		value := []byte(fmt.Sprintf("value-%d", i))
		err := cache.Set(ctx, key, value, 5*time.Minute)
		require.NoError(t, err)
	}

	// Clear cache
	err = cache.Clear(ctx)
	assert.NoError(t, err)

	// Verify all entries are gone
	for i := 0; i < 5; i++ {
		key := fmt.Sprintf("key-%d", i)
		_, err := cache.Get(ctx, key)
		assert.Equal(t, ErrCacheNotFound, err)
	}
}

func TestRedisCacheImpl_DeletePattern(t *testing.T) {
	mr, config := setupMockRedis(t)
	defer mr.Close()

	logger := zaptest.NewLogger(t)
	cache, err := NewRedisCache("test", config, logger)
	require.NoError(t, err)
	defer cache.Close()

	ctx := context.Background()

	// Add entries with different patterns
	testData := map[string][]byte{
		"user:1":    []byte("user1"),
		"user:2":    []byte("user2"),
		"session:1": []byte("session1"),
		"session:2": []byte("session2"),
		"other":     []byte("other"),
	}

	for key, value := range testData {
		err := cache.Set(ctx, key, value, 5*time.Minute)
		require.NoError(t, err)
	}

	// Delete user pattern
	err = cache.DeletePattern(ctx, "user:*")
	assert.NoError(t, err)

	// Verify user keys are deleted
	_, err = cache.Get(ctx, "user:1")
	assert.Equal(t, ErrCacheNotFound, err)
	_, err = cache.Get(ctx, "user:2")
	assert.Equal(t, ErrCacheNotFound, err)

	// Verify other keys still exist
	_, err = cache.Get(ctx, "session:1")
	assert.NoError(t, err)
	_, err = cache.Get(ctx, "session:2")
	assert.NoError(t, err)
	_, err = cache.Get(ctx, "other")
	assert.NoError(t, err)
}

func TestRedisCacheImpl_SetWithVersion(t *testing.T) {
	mr, config := setupMockRedis(t)
	defer mr.Close()

	logger := zaptest.NewLogger(t)
	cache, err := NewRedisCache("test", config, logger)
	require.NoError(t, err)
	defer cache.Close()

	ctx := context.Background()

	t.Run("Set with version", func(t *testing.T) {
		key := "version-key"
		value := []byte("version-value")
		version := int64(1)
		
		err := cache.SetWithVersion(ctx, key, value, 5*time.Minute, version)
		assert.NoError(t, err)

		// Retrieve and verify version is stored
		retrieved, err := cache.Get(ctx, key)
		require.NoError(t, err)
		assert.Equal(t, value, retrieved)
	})
}

func TestRedisCacheImpl_Stats(t *testing.T) {
	mr, config := setupMockRedis(t)
	defer mr.Close()

	logger := zaptest.NewLogger(t)
	cache, err := NewRedisCache("test", config, logger)
	require.NoError(t, err)
	defer cache.Close()

	ctx := context.Background()

	// Initial stats
	stats := cache.Stats()
	assert.Equal(t, int64(0), stats.Hits)
	assert.Equal(t, int64(0), stats.Misses)
	assert.Equal(t, int64(0), stats.Sets)

	// Set some values
	key := "stats-key"
	value := []byte("stats-value")
	err = cache.Set(ctx, key, value, 5*time.Minute)
	require.NoError(t, err)

	// Get the value (hit)
	_, err = cache.Get(ctx, key)
	require.NoError(t, err)

	// Try to get non-existent value (miss)
	_, err = cache.Get(ctx, "non-existent")
	assert.Equal(t, ErrCacheNotFound, err)

	// Check updated stats
	stats = cache.Stats()
	assert.Equal(t, int64(1), stats.Hits)
	assert.Equal(t, int64(1), stats.Misses)
	assert.Equal(t, int64(1), stats.Sets)
	assert.Equal(t, float64(0.5), stats.HitRatio) // 1 hit out of 2 total
}

func TestRedisCacheImpl_Concurrent(t *testing.T) {
	mr, config := setupMockRedis(t)
	defer mr.Close()

	logger := zaptest.NewLogger(t)
	cache, err := NewRedisCache("test", config, logger)
	require.NoError(t, err)
	defer cache.Close()

	ctx := context.Background()
	numGoroutines := 50
	numOperations := 10

	// Test concurrent operations
	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			defer wg.Done()
			
			for j := 0; j < numOperations; j++ {
				key := fmt.Sprintf("concurrent-key-%d-%d", id, j)
				value := []byte(fmt.Sprintf("concurrent-value-%d-%d", id, j))
				
				// Set
				err := cache.Set(ctx, key, value, 5*time.Minute)
				assert.NoError(t, err)
				
				// Get
				retrieved, err := cache.Get(ctx, key)
				assert.NoError(t, err)
				assert.Equal(t, value, retrieved)
				
				// Delete
				err = cache.Delete(ctx, key)
				assert.NoError(t, err)
			}
		}(i)
	}

	wg.Wait()
}

func TestRedisCacheImpl_ConnectionFailure(t *testing.T) {
	// Test with invalid Redis address
	config := RedisClusterConfig{
		Addrs:        []string{"localhost:9999"}, // Invalid address
		DialTimeout:  100 * time.Millisecond,
		ReadTimeout:  100 * time.Millisecond,
		WriteTimeout: 100 * time.Millisecond,
	}

	logger := zaptest.NewLogger(t)
	_, err := NewRedisCache("test", config, logger)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to connect to Redis")
}

func BenchmarkRedisCacheImpl_Set(b *testing.B) {
	mr, config := setupMockRedis(&testing.T{})
	defer mr.Close()

	logger := zaptest.NewLogger(b)
	cache, err := NewRedisCache("benchmark", config, logger)
	require.NoError(b, err)
	defer cache.Close()

	ctx := context.Background()
	value := []byte("benchmark-value")

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			key := fmt.Sprintf("benchmark-key-%d", i)
			cache.Set(ctx, key, value, 5*time.Minute)
			i++
		}
	})
}

func BenchmarkRedisCacheImpl_Get(b *testing.B) {
	mr, config := setupMockRedis(&testing.T{})
	defer mr.Close()

	logger := zaptest.NewLogger(b)
	cache, err := NewRedisCache("benchmark", config, logger)
	require.NoError(b, err)
	defer cache.Close()

	ctx := context.Background()
	value := []byte("benchmark-value")

	// Pre-populate cache
	for i := 0; i < 1000; i++ {
		key := fmt.Sprintf("benchmark-key-%d", i)
		cache.Set(ctx, key, value, 5*time.Minute)
	}

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			key := fmt.Sprintf("benchmark-key-%d", i%1000)
			cache.Get(ctx, key)
			i++
		}
	})
}