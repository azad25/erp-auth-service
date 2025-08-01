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

func setupTestCacheManager(t *testing.T) (*CacheManagerImpl, *miniredis.Miniredis) {
	mr, err := miniredis.Run()
	require.NoError(t, err)

	config := CacheConfig{
		BigCacheConfig: BigCacheConfig{
			Shards:             256,
			LifeWindow:         10 * time.Minute,
			CleanWindow:        5 * time.Minute,
			MaxEntriesInWindow: 1000,
			MaxEntrySize:       500,
			HardMaxCacheSize:   8,
			Verbose:            false,
		},
		RedisConfig: RedisClusterConfig{
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
		},
		DefaultTTL:        5 * time.Minute,
		MaxRetries:        3,
		RetryDelay:        100 * time.Millisecond,
		EnableMetrics:     true,
		MetricsInterval:   30 * time.Second,
		SyncInterval:      0, // Disable sync for tests
		WarmupEnabled:     true,
		WarmupBatchSize:   100,
		InvalidationDelay: 100 * time.Millisecond,
	}

	logger := zaptest.NewLogger(t)
	manager, err := NewCacheManager(config, logger)
	require.NoError(t, err)

	return manager, mr
}

func TestCacheManager_Get(t *testing.T) {
	manager, mr := setupTestCacheManager(t)
	defer manager.Close()
	defer mr.Close()

	ctx := context.Background()

	t.Run("Get from empty cache", func(t *testing.T) {
		_, err := manager.Get(ctx, "non-existent")
		assert.Equal(t, ErrCacheNotFound, err)
	})

	t.Run("Get from L2 cache and populate L1", func(t *testing.T) {
		key := "l2-key"
		value := []byte("l2-value")

		// Set directly in L2 cache
		err := manager.l2Cache.Set(ctx, key, value, 5*time.Minute)
		require.NoError(t, err)

		// Get should retrieve from L2 and populate L1
		retrieved, err := manager.Get(ctx, key)
		require.NoError(t, err)
		assert.Equal(t, value, retrieved)

		// Give time for async L1 population
		time.Sleep(100 * time.Millisecond)

		// Verify it's now in L1 cache
		l1Value, err := manager.l1Cache.Get(ctx, key)
		require.NoError(t, err)
		assert.Equal(t, value, l1Value)
	})

	t.Run("Get from L1 cache (hit)", func(t *testing.T) {
		key := "l1-key"
		value := []byte("l1-value")

		// Set in both caches
		err := manager.Set(ctx, key, value, 5*time.Minute)
		require.NoError(t, err)

		// Get should hit L1 cache
		retrieved, err := manager.Get(ctx, key)
		require.NoError(t, err)
		assert.Equal(t, value, retrieved)
	})

	t.Run("Get with empty key", func(t *testing.T) {
		_, err := manager.Get(ctx, "")
		assert.Equal(t, ErrInvalidKey, err)
	})
}

func TestCacheManager_Set(t *testing.T) {
	manager, mr := setupTestCacheManager(t)
	defer manager.Close()
	defer mr.Close()

	ctx := context.Background()

	t.Run("Set with write-through strategy", func(t *testing.T) {
		manager.SetStrategy(WriteThrough)
		
		key := "write-through-key"
		value := []byte("write-through-value")

		err := manager.Set(ctx, key, value, 5*time.Minute)
		require.NoError(t, err)

		// Verify in both caches
		l1Value, err := manager.l1Cache.Get(ctx, key)
		require.NoError(t, err)
		assert.Equal(t, value, l1Value)

		l2Value, err := manager.l2Cache.Get(ctx, key)
		require.NoError(t, err)
		assert.Equal(t, value, l2Value)
	})

	t.Run("Set with write-back strategy", func(t *testing.T) {
		manager.SetStrategy(WriteBack)
		
		key := "write-back-key"
		value := []byte("write-back-value")

		err := manager.Set(ctx, key, value, 5*time.Minute)
		require.NoError(t, err)

		// Should be in L1 immediately
		l1Value, err := manager.l1Cache.Get(ctx, key)
		require.NoError(t, err)
		assert.Equal(t, value, l1Value)

		// Give time for async L2 write
		time.Sleep(100 * time.Millisecond)

		// Should eventually be in L2
		l2Value, err := manager.l2Cache.Get(ctx, key)
		require.NoError(t, err)
		assert.Equal(t, value, l2Value)
	})

	t.Run("Set with empty key", func(t *testing.T) {
		err := manager.Set(ctx, "", []byte("value"), 5*time.Minute)
		assert.Equal(t, ErrInvalidKey, err)
	})

	t.Run("Set with nil value", func(t *testing.T) {
		err := manager.Set(ctx, "key", nil, 5*time.Minute)
		assert.Equal(t, ErrInvalidValue, err)
	})

	t.Run("Set with zero TTL uses default", func(t *testing.T) {
		key := "zero-ttl-key"
		value := []byte("zero-ttl-value")

		err := manager.Set(ctx, key, value, 0)
		require.NoError(t, err)

		// Should be retrievable
		retrieved, err := manager.Get(ctx, key)
		require.NoError(t, err)
		assert.Equal(t, value, retrieved)
	})
}

func TestCacheManager_Delete(t *testing.T) {
	manager, mr := setupTestCacheManager(t)
	defer manager.Close()
	defer mr.Close()

	ctx := context.Background()

	t.Run("Delete existing key", func(t *testing.T) {
		key := "delete-key"
		value := []byte("delete-value")

		// Set in both caches
		err := manager.Set(ctx, key, value, 5*time.Minute)
		require.NoError(t, err)

		// Delete
		err = manager.Delete(ctx, key)
		require.NoError(t, err)

		// Verify deleted from both caches
		_, err = manager.l1Cache.Get(ctx, key)
		assert.Equal(t, ErrCacheNotFound, err)

		_, err = manager.l2Cache.Get(ctx, key)
		assert.Equal(t, ErrCacheNotFound, err)
	})

	t.Run("Delete non-existent key", func(t *testing.T) {
		err := manager.Delete(ctx, "non-existent")
		assert.NoError(t, err) // Should not error for non-existent keys
	})

	t.Run("Delete empty key", func(t *testing.T) {
		err := manager.Delete(ctx, "")
		assert.Equal(t, ErrInvalidKey, err)
	})
}

func TestCacheManager_Exists(t *testing.T) {
	manager, mr := setupTestCacheManager(t)
	defer manager.Close()
	defer mr.Close()

	ctx := context.Background()

	t.Run("Exists in L1 cache", func(t *testing.T) {
		key := "exists-l1-key"
		value := []byte("exists-l1-value")

		err := manager.l1Cache.Set(ctx, key, value, 5*time.Minute)
		require.NoError(t, err)

		exists, err := manager.Exists(ctx, key)
		require.NoError(t, err)
		assert.True(t, exists)
	})

	t.Run("Exists in L2 cache only", func(t *testing.T) {
		key := "exists-l2-key"
		value := []byte("exists-l2-value")

		err := manager.l2Cache.Set(ctx, key, value, 5*time.Minute)
		require.NoError(t, err)

		exists, err := manager.Exists(ctx, key)
		require.NoError(t, err)
		assert.True(t, exists)
	})

	t.Run("Does not exist", func(t *testing.T) {
		exists, err := manager.Exists(ctx, "non-existent")
		require.NoError(t, err)
		assert.False(t, exists)
	})

	t.Run("Exists with empty key", func(t *testing.T) {
		exists, err := manager.Exists(ctx, "")
		assert.Equal(t, ErrInvalidKey, err)
		assert.False(t, exists)
	})
}

func TestCacheManager_Clear(t *testing.T) {
	manager, mr := setupTestCacheManager(t)
	defer manager.Close()
	defer mr.Close()

	ctx := context.Background()

	// Add some entries
	for i := 0; i < 5; i++ {
		key := fmt.Sprintf("clear-key-%d", i)
		value := []byte(fmt.Sprintf("clear-value-%d", i))
		err := manager.Set(ctx, key, value, 5*time.Minute)
		require.NoError(t, err)
	}

	// Clear all caches
	err := manager.Clear(ctx)
	require.NoError(t, err)

	// Verify all entries are gone
	for i := 0; i < 5; i++ {
		key := fmt.Sprintf("clear-key-%d", i)
		_, err := manager.Get(ctx, key)
		assert.Equal(t, ErrCacheNotFound, err)
	}
}

func TestCacheManager_GetFromLevel(t *testing.T) {
	manager, mr := setupTestCacheManager(t)
	defer manager.Close()
	defer mr.Close()

	ctx := context.Background()

	key := "level-key"
	value := []byte("level-value")

	// Set in L2 only
	err := manager.l2Cache.Set(ctx, key, value, 5*time.Minute)
	require.NoError(t, err)

	t.Run("Get from L1 (should not exist)", func(t *testing.T) {
		_, err := manager.GetFromLevel(ctx, key, L1Cache)
		assert.Equal(t, ErrCacheNotFound, err)
	})

	t.Run("Get from L2 (should exist)", func(t *testing.T) {
		retrieved, err := manager.GetFromLevel(ctx, key, L2Cache)
		require.NoError(t, err)
		assert.Equal(t, value, retrieved)
	})

	t.Run("Get from unsupported level", func(t *testing.T) {
		_, err := manager.GetFromLevel(ctx, key, L3Cache)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "unsupported cache level")
	})
}

func TestCacheManager_SetToLevel(t *testing.T) {
	manager, mr := setupTestCacheManager(t)
	defer manager.Close()
	defer mr.Close()

	ctx := context.Background()

	key := "level-set-key"
	value := []byte("level-set-value")

	t.Run("Set to L1 only", func(t *testing.T) {
		err := manager.SetToLevel(ctx, key, value, 5*time.Minute, L1Cache)
		require.NoError(t, err)

		// Should exist in L1
		retrieved, err := manager.l1Cache.Get(ctx, key)
		require.NoError(t, err)
		assert.Equal(t, value, retrieved)

		// Should not exist in L2
		_, err = manager.l2Cache.Get(ctx, key)
		assert.Equal(t, ErrCacheNotFound, err)
	})

	t.Run("Set to L2 only", func(t *testing.T) {
		key2 := "level-set-key-2"
		err := manager.SetToLevel(ctx, key2, value, 5*time.Minute, L2Cache)
		require.NoError(t, err)

		// Should exist in L2
		retrieved, err := manager.l2Cache.Get(ctx, key2)
		require.NoError(t, err)
		assert.Equal(t, value, retrieved)

		// Should not exist in L1
		_, err = manager.l1Cache.Get(ctx, key2)
		assert.Equal(t, ErrCacheNotFound, err)
	})

	t.Run("Set to unsupported level", func(t *testing.T) {
		err := manager.SetToLevel(ctx, key, value, 5*time.Minute, L3Cache)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "unsupported cache level")
	})
}

func TestCacheManager_InvalidateKey(t *testing.T) {
	manager, mr := setupTestCacheManager(t)
	defer manager.Close()
	defer mr.Close()

	ctx := context.Background()

	key := "invalidate-key"
	value := []byte("invalidate-value")

	// Set in both caches
	err := manager.Set(ctx, key, value, 5*time.Minute)
	require.NoError(t, err)

	// Invalidate
	err = manager.InvalidateKey(ctx, key)
	require.NoError(t, err)

	// Should be gone from both caches
	_, err = manager.Get(ctx, key)
	assert.Equal(t, ErrCacheNotFound, err)
}

func TestCacheManager_WarmCache(t *testing.T) {
	manager, mr := setupTestCacheManager(t)
	defer manager.Close()
	defer mr.Close()

	ctx := context.Background()

	// Set some data in L2 cache only
	keys := []string{"warm-key-1", "warm-key-2", "warm-key-3"}
	for _, key := range keys {
		value := []byte(fmt.Sprintf("warm-value-%s", key))
		err := manager.l2Cache.Set(ctx, key, value, 5*time.Minute)
		require.NoError(t, err)
	}

	// Warm cache
	err := manager.WarmCache(ctx, keys)
	require.NoError(t, err)

	// Give time for warming to complete
	time.Sleep(200 * time.Millisecond)

	// Verify keys are now in L1 cache
	for _, key := range keys {
		_, err := manager.l1Cache.Get(ctx, key)
		assert.NoError(t, err, "Key %s should be in L1 cache after warming", key)
	}
}

func TestCacheManager_Stats(t *testing.T) {
	manager, mr := setupTestCacheManager(t)
	defer manager.Close()
	defer mr.Close()

	ctx := context.Background()

	// Initial stats
	stats := manager.Stats()
	assert.Equal(t, int64(0), stats.Hits)
	assert.Equal(t, int64(0), stats.Misses)

	// Perform some operations
	key := "stats-key"
	value := []byte("stats-value")

	// Set (should increment sets)
	err := manager.Set(ctx, key, value, 5*time.Minute)
	require.NoError(t, err)

	// Get (should increment hits)
	_, err = manager.Get(ctx, key)
	require.NoError(t, err)

	// Get non-existent (should increment misses)
	_, err = manager.Get(ctx, "non-existent")
	assert.Equal(t, ErrCacheNotFound, err)

	// Check updated stats
	stats = manager.Stats()
	assert.True(t, stats.Hits > 0)
	assert.True(t, stats.Misses > 0)
	assert.True(t, stats.Sets > 0)
}

func TestCacheManager_RegisterCache(t *testing.T) {
	manager, mr := setupTestCacheManager(t)
	defer manager.Close()
	defer mr.Close()

	// Create a mock cache
	mockCache := &MockCache{}

	t.Run("Register new cache", func(t *testing.T) {
		err := manager.RegisterCache("mock", mockCache)
		assert.NoError(t, err)

		// Verify it's registered
		registered := manager.GetCache("mock")
		assert.Equal(t, mockCache, registered)
	})

	t.Run("Register duplicate cache", func(t *testing.T) {
		err := manager.RegisterCache("mock", mockCache)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "already registered")
	})
}

func TestCacheManager_Strategy(t *testing.T) {
	manager, mr := setupTestCacheManager(t)
	defer manager.Close()
	defer mr.Close()

	// Test default strategy
	assert.Equal(t, WriteThrough, manager.GetStrategy())

	// Test setting strategy
	manager.SetStrategy(WriteBack)
	assert.Equal(t, WriteBack, manager.GetStrategy())

	manager.SetStrategy(ReadThrough)
	assert.Equal(t, ReadThrough, manager.GetStrategy())

	manager.SetStrategy(CacheAside)
	assert.Equal(t, CacheAside, manager.GetStrategy())
}

func TestCacheManager_Concurrent(t *testing.T) {
	manager, mr := setupTestCacheManager(t)
	defer manager.Close()
	defer mr.Close()

	ctx := context.Background()
	numGoroutines := 50
	numOperations := 10

	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			defer wg.Done()
			
			for j := 0; j < numOperations; j++ {
				key := fmt.Sprintf("concurrent-key-%d-%d", id, j)
				value := []byte(fmt.Sprintf("concurrent-value-%d-%d", id, j))
				
				// Set
				err := manager.Set(ctx, key, value, 5*time.Minute)
				assert.NoError(t, err)
				
				// Get
				retrieved, err := manager.Get(ctx, key)
				assert.NoError(t, err)
				assert.Equal(t, value, retrieved)
				
				// Delete
				err = manager.Delete(ctx, key)
				assert.NoError(t, err)
			}
		}(i)
	}

	wg.Wait()
}

// MockCache is a simple mock implementation for testing
type MockCache struct {
	data map[string][]byte
	mu   sync.RWMutex
}

func (m *MockCache) Get(ctx context.Context, key string) ([]byte, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	
	if m.data == nil {
		return nil, ErrCacheNotFound
	}
	
	if value, exists := m.data[key]; exists {
		return value, nil
	}
	return nil, ErrCacheNotFound
}

func (m *MockCache) Set(ctx context.Context, key string, value []byte, ttl time.Duration) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	if m.data == nil {
		m.data = make(map[string][]byte)
	}
	m.data[key] = value
	return nil
}

func (m *MockCache) Delete(ctx context.Context, key string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	if m.data == nil {
		return ErrCacheNotFound
	}
	
	if _, exists := m.data[key]; !exists {
		return ErrCacheNotFound
	}
	
	delete(m.data, key)
	return nil
}

func (m *MockCache) Exists(ctx context.Context, key string) (bool, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	
	if m.data == nil {
		return false, nil
	}
	
	_, exists := m.data[key]
	return exists, nil
}

func (m *MockCache) Clear(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	m.data = make(map[string][]byte)
	return nil
}

func (m *MockCache) Stats() CacheStats {
	return CacheStats{}
}

func (m *MockCache) Close() error {
	return nil
}

func BenchmarkCacheManager_Set(b *testing.B) {
	manager, mr := setupTestCacheManager(&testing.T{})
	defer manager.Close()
	defer mr.Close()

	ctx := context.Background()
	value := []byte("benchmark-value")

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			key := fmt.Sprintf("benchmark-key-%d", i)
			manager.Set(ctx, key, value, 5*time.Minute)
			i++
		}
	})
}

func BenchmarkCacheManager_Get(b *testing.B) {
	manager, mr := setupTestCacheManager(&testing.T{})
	defer manager.Close()
	defer mr.Close()

	ctx := context.Background()
	value := []byte("benchmark-value")

	// Pre-populate cache
	for i := 0; i < 1000; i++ {
		key := fmt.Sprintf("benchmark-key-%d", i)
		manager.Set(ctx, key, value, 5*time.Minute)
	}

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			key := fmt.Sprintf("benchmark-key-%d", i%1000)
			manager.Get(ctx, key)
			i++
		}
	})
}