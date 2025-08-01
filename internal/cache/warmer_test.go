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

func setupTestCacheWarmer(t *testing.T) (*CacheWarmerImpl, *CacheManagerImpl, *miniredis.Miniredis) {
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
		WarmupBatchSize:   10,
	}

	logger := zaptest.NewLogger(t)
	manager, err := NewCacheManager(config, logger)
	require.NoError(t, err)

	warmer := manager.warmer.(*CacheWarmerImpl)
	return warmer, manager, mr
}

func TestCacheWarmer_WarmKeys(t *testing.T) {
	warmer, manager, mr := setupTestCacheWarmer(t)
	defer manager.Close()
	defer mr.Close()

	ctx := context.Background()

	t.Run("Warm keys from L2 to L1", func(t *testing.T) {
		// Set data in L2 cache only
		keys := []string{"warm-key-1", "warm-key-2", "warm-key-3"}
		for _, key := range keys {
			value := []byte(fmt.Sprintf("warm-value-%s", key))
			err := manager.l2Cache.Set(ctx, key, value, 5*time.Minute)
			require.NoError(t, err)
		}

		// Verify keys are not in L1 cache
		for _, key := range keys {
			_, err := manager.l1Cache.Get(ctx, key)
			assert.Equal(t, ErrCacheNotFound, err)
		}

		// Warm the keys
		err := warmer.WarmKeys(ctx, keys)
		require.NoError(t, err)

		// Give time for warming to complete
		time.Sleep(100 * time.Millisecond)

		// Verify keys are now in L1 cache
		for _, key := range keys {
			value, err := manager.l1Cache.Get(ctx, key)
			require.NoError(t, err)
			expectedValue := []byte(fmt.Sprintf("warm-value-%s", key))
			assert.Equal(t, expectedValue, value)
		}
	})

	t.Run("Warm empty key list", func(t *testing.T) {
		err := warmer.WarmKeys(ctx, []string{})
		assert.NoError(t, err)
	})

	t.Run("Warm keys already in L1", func(t *testing.T) {
		key := "already-in-l1"
		value := []byte("already-in-l1-value")

		// Set in both caches
		err := manager.Set(ctx, key, value, 5*time.Minute)
		require.NoError(t, err)

		// Warm should not cause issues
		err = warmer.WarmKeys(ctx, []string{key})
		assert.NoError(t, err)

		// Value should still be accessible
		retrieved, err := manager.Get(ctx, key)
		require.NoError(t, err)
		assert.Equal(t, value, retrieved)
	})

	t.Run("Warm non-existent keys", func(t *testing.T) {
		keys := []string{"non-existent-1", "non-existent-2"}
		
		// Should not error even if keys don't exist
		err := warmer.WarmKeys(ctx, keys)
		assert.NoError(t, err)
	})

	t.Run("Warm with context cancellation", func(t *testing.T) {
		// Create a context that will be cancelled
		cancelCtx, cancel := context.WithCancel(ctx)
		
		// Set up many keys to warm
		keys := make([]string, 100)
		for i := 0; i < 100; i++ {
			keys[i] = fmt.Sprintf("cancel-key-%d", i)
			value := []byte(fmt.Sprintf("cancel-value-%d", i))
			err := manager.l2Cache.Set(ctx, keys[i], value, 5*time.Minute)
			require.NoError(t, err)
		}

		// Cancel the context immediately
		cancel()

		// Warming should handle cancellation gracefully
		err := warmer.WarmKeys(cancelCtx, keys)
		assert.NoError(t, err)
	})
}

func TestCacheWarmer_WarmPattern(t *testing.T) {
	warmer, manager, mr := setupTestCacheWarmer(t)
	defer manager.Close()
	defer mr.Close()

	ctx := context.Background()

	t.Run("Warm keys by pattern", func(t *testing.T) {
		// Set data with pattern in L2 cache
		testData := map[string][]byte{
			"user:1":    []byte("user1"),
			"user:2":    []byte("user2"),
			"user:3":    []byte("user3"),
			"session:1": []byte("session1"),
			"other":     []byte("other"),
		}

		for key, value := range testData {
			err := manager.l2Cache.Set(ctx, key, value, 5*time.Minute)
			require.NoError(t, err)
		}

		// Warm user pattern
		err := warmer.WarmPattern(ctx, "user:*")
		require.NoError(t, err)

		// Give time for warming to complete
		time.Sleep(200 * time.Millisecond)

		// Verify user keys are in L1 cache
		for key, expectedValue := range testData {
			if key == "user:1" || key == "user:2" || key == "user:3" {
				value, err := manager.l1Cache.Get(ctx, key)
				require.NoError(t, err, "Key %s should be in L1 cache", key)
				assert.Equal(t, expectedValue, value)
			}
		}

		// Verify non-matching keys are not in L1 cache
		_, err = manager.l1Cache.Get(ctx, "session:1")
		assert.Equal(t, ErrCacheNotFound, err)
		_, err = manager.l1Cache.Get(ctx, "other")
		assert.Equal(t, ErrCacheNotFound, err)
	})

	t.Run("Warm empty pattern", func(t *testing.T) {
		err := warmer.WarmPattern(ctx, "")
		assert.NoError(t, err)
	})

	t.Run("Warm pattern with no matches", func(t *testing.T) {
		err := warmer.WarmPattern(ctx, "nonexistent:*")
		assert.NoError(t, err)
	})
}

func TestCacheWarmer_ScheduleWarming(t *testing.T) {
	warmer, manager, mr := setupTestCacheWarmer(t)
	defer manager.Close()
	defer mr.Close()

	ctx := context.Background()

	t.Run("Schedule warming with valid parameters", func(t *testing.T) {
		keys := []string{"scheduled-key-1", "scheduled-key-2"}
		interval := 100 * time.Millisecond

		// Set data in L2 cache
		for _, key := range keys {
			value := []byte(fmt.Sprintf("scheduled-value-%s", key))
			err := manager.l2Cache.Set(ctx, key, value, 5*time.Minute)
			require.NoError(t, err)
		}

		// Schedule warming
		err := warmer.ScheduleWarming(ctx, keys, interval)
		assert.NoError(t, err)

		// Wait for at least one warming cycle
		time.Sleep(200 * time.Millisecond)

		// Verify keys are warmed
		for _, key := range keys {
			_, err := manager.l1Cache.Get(ctx, key)
			assert.NoError(t, err, "Key %s should be warmed", key)
		}

		// Stop warming to clean up
		warmer.StopWarming(ctx)
	})

	t.Run("Schedule warming with empty keys", func(t *testing.T) {
		err := warmer.ScheduleWarming(ctx, []string{}, 1*time.Second)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "invalid parameters")
	})

	t.Run("Schedule warming with zero interval", func(t *testing.T) {
		keys := []string{"test-key"}
		err := warmer.ScheduleWarming(ctx, keys, 0)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "invalid parameters")
	})

	t.Run("Schedule warming with negative interval", func(t *testing.T) {
		keys := []string{"test-key"}
		err := warmer.ScheduleWarming(ctx, keys, -1*time.Second)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "invalid parameters")
	})
}

func TestCacheWarmer_GetWarmingStats(t *testing.T) {
	warmer, manager, mr := setupTestCacheWarmer(t)
	defer manager.Close()
	defer mr.Close()

	ctx := context.Background()

	t.Run("Get initial stats", func(t *testing.T) {
		stats := warmer.GetWarmingStats()
		assert.Equal(t, 0, stats["active_jobs"])
		assert.Equal(t, 0, stats["running_jobs"])
	})

	t.Run("Get stats after scheduling jobs", func(t *testing.T) {
		keys1 := []string{"stats-key-1"}
		keys2 := []string{"stats-key-2"}

		err := warmer.ScheduleWarming(ctx, keys1, 1*time.Hour)
		require.NoError(t, err)

		err = warmer.ScheduleWarming(ctx, keys2, 1*time.Hour)
		require.NoError(t, err)

		stats := warmer.GetWarmingStats()
		assert.Equal(t, 2, stats["active_jobs"])
		assert.Equal(t, 2, stats["running_jobs"])

		// Clean up
		warmer.StopWarming(ctx)
	})
}

func TestCacheWarmer_StopWarming(t *testing.T) {
	warmer, manager, mr := setupTestCacheWarmer(t)
	defer manager.Close()
	defer mr.Close()

	ctx := context.Background()

	t.Run("Stop warming", func(t *testing.T) {
		keys := []string{"stop-key-1", "stop-key-2"}
		
		// Schedule warming
		err := warmer.ScheduleWarming(ctx, keys, 100*time.Millisecond)
		require.NoError(t, err)

		// Verify jobs are active
		stats := warmer.GetWarmingStats()
		assert.True(t, stats["active_jobs"].(int) > 0)

		// Stop warming
		err = warmer.StopWarming(ctx)
		assert.NoError(t, err)

		// Verify jobs are stopped
		stats = warmer.GetWarmingStats()
		assert.Equal(t, 0, stats["running_jobs"])
	})
}

func TestCacheWarmer_PredictiveWarming(t *testing.T) {
	warmer, manager, mr := setupTestCacheWarmer(t)
	defer manager.Close()
	defer mr.Close()

	ctx := context.Background()

	t.Run("Predictive warming placeholder", func(t *testing.T) {
		// This is currently a placeholder implementation
		err := warmer.PredictiveWarming(ctx)
		assert.NoError(t, err)
	})
}

func TestCacheWarmer_Concurrent(t *testing.T) {
	warmer, manager, mr := setupTestCacheWarmer(t)
	defer manager.Close()
	defer mr.Close()

	ctx := context.Background()

	t.Run("Concurrent warming operations", func(t *testing.T) {
		numGoroutines := 10
		keysPerGoroutine := 10

		// Set up data in L2 cache
		allKeys := make([]string, 0, numGoroutines*keysPerGoroutine)
		for i := 0; i < numGoroutines; i++ {
			for j := 0; j < keysPerGoroutine; j++ {
				key := fmt.Sprintf("concurrent-warm-key-%d-%d", i, j)
				value := []byte(fmt.Sprintf("concurrent-warm-value-%d-%d", i, j))
				err := manager.l2Cache.Set(ctx, key, value, 5*time.Minute)
				require.NoError(t, err)
				allKeys = append(allKeys, key)
			}
		}

		// Perform concurrent warming
		var wg sync.WaitGroup
		wg.Add(numGoroutines)

		for i := 0; i < numGoroutines; i++ {
			go func(id int) {
				defer wg.Done()
				
				keys := make([]string, keysPerGoroutine)
				for j := 0; j < keysPerGoroutine; j++ {
					keys[j] = fmt.Sprintf("concurrent-warm-key-%d-%d", id, j)
				}
				
				err := warmer.WarmKeys(ctx, keys)
				assert.NoError(t, err)
			}(i)
		}

		wg.Wait()

		// Give time for all warming operations to complete
		time.Sleep(200 * time.Millisecond)

		// Verify all keys are warmed
		for _, key := range allKeys {
			_, err := manager.l1Cache.Get(ctx, key)
			assert.NoError(t, err, "Key %s should be warmed", key)
		}
	})
}

func BenchmarkCacheWarmer_WarmKeys(b *testing.B) {
	warmer, manager, mr := setupTestCacheWarmer(&testing.T{})
	defer manager.Close()
	defer mr.Close()

	ctx := context.Background()

	// Pre-populate L2 cache
	keys := make([]string, 1000)
	for i := 0; i < 1000; i++ {
		key := fmt.Sprintf("benchmark-warm-key-%d", i)
		value := []byte(fmt.Sprintf("benchmark-warm-value-%d", i))
		keys[i] = key
		manager.l2Cache.Set(ctx, key, value, 5*time.Minute)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// Clear L1 cache before each iteration
		manager.l1Cache.Clear(ctx)
		
		// Warm a subset of keys
		startIdx := (i * 100) % 900
		endIdx := startIdx + 100
		warmer.WarmKeys(ctx, keys[startIdx:endIdx])
	}
}