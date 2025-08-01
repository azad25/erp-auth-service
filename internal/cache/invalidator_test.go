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

func setupTestCacheInvalidator(t *testing.T) (*CacheInvalidatorImpl, *CacheManagerImpl, *miniredis.Miniredis) {
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
		InvalidationDelay: 50 * time.Millisecond,
	}

	logger := zaptest.NewLogger(t)
	manager, err := NewCacheManager(config, logger)
	require.NoError(t, err)

	invalidator := manager.invalidator.(*CacheInvalidatorImpl)
	return invalidator, manager, mr
}

func TestCacheInvalidator_InvalidateKey(t *testing.T) {
	invalidator, manager, mr := setupTestCacheInvalidator(t)
	defer manager.Close()
	defer mr.Close()

	ctx := context.Background()

	t.Run("Invalidate existing key", func(t *testing.T) {
		key := "invalidate-key"
		value := []byte("invalidate-value")

		// Set in both caches
		err := manager.Set(ctx, key, value, 5*time.Minute)
		require.NoError(t, err)

		// Verify key exists
		exists, err := manager.Exists(ctx, key)
		require.NoError(t, err)
		assert.True(t, exists)

		// Invalidate key
		err = invalidator.InvalidateKey(ctx, key)
		require.NoError(t, err)

		// Verify key is gone from both caches
		_, err = manager.Get(ctx, key)
		assert.Equal(t, ErrCacheNotFound, err)
	})

	t.Run("Invalidate non-existent key", func(t *testing.T) {
		err := invalidator.InvalidateKey(ctx, "non-existent")
		assert.NoError(t, err) // Should not error for non-existent keys
	})

	t.Run("Invalidate empty key", func(t *testing.T) {
		err := invalidator.InvalidateKey(ctx, "")
		assert.Equal(t, ErrInvalidKey, err)
	})
}

func TestCacheInvalidator_InvalidatePattern(t *testing.T) {
	invalidator, manager, mr := setupTestCacheInvalidator(t)
	defer manager.Close()
	defer mr.Close()

	ctx := context.Background()

	t.Run("Invalidate keys by pattern", func(t *testing.T) {
		// Set data with different patterns
		testData := map[string][]byte{
			"user:1":    []byte("user1"),
			"user:2":    []byte("user2"),
			"user:3":    []byte("user3"),
			"session:1": []byte("session1"),
			"session:2": []byte("session2"),
			"other":     []byte("other"),
		}

		for key, value := range testData {
			err := manager.Set(ctx, key, value, 5*time.Minute)
			require.NoError(t, err)
		}

		// Invalidate user pattern
		err := invalidator.InvalidatePattern(ctx, "user:*")
		require.NoError(t, err)

		// Verify user keys are invalidated
		for key := range testData {
			if key == "user:1" || key == "user:2" || key == "user:3" {
				_, err := manager.Get(ctx, key)
				assert.Equal(t, ErrCacheNotFound, err, "Key %s should be invalidated", key)
			}
		}

		// Verify other keys still exist
		_, err = manager.Get(ctx, "session:1")
		assert.NoError(t, err)
		_, err = manager.Get(ctx, "session:2")
		assert.NoError(t, err)
		_, err = manager.Get(ctx, "other")
		assert.NoError(t, err)
	})

	t.Run("Invalidate empty pattern", func(t *testing.T) {
		err := invalidator.InvalidatePattern(ctx, "")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "pattern cannot be empty")
	})

	t.Run("Invalidate pattern with no matches", func(t *testing.T) {
		err := invalidator.InvalidatePattern(ctx, "nonexistent:*")
		assert.NoError(t, err)
	})
}

func TestCacheInvalidator_InvalidateByTags(t *testing.T) {
	invalidator, manager, mr := setupTestCacheInvalidator(t)
	defer manager.Close()
	defer mr.Close()

	ctx := context.Background()

	t.Run("Invalidate keys by tags", func(t *testing.T) {
		// Set up test data
		testData := map[string][]byte{
			"user:1":    []byte("user1"),
			"user:2":    []byte("user2"),
			"profile:1": []byte("profile1"),
			"profile:2": []byte("profile2"),
			"other":     []byte("other"),
		}

		for key, value := range testData {
			err := manager.Set(ctx, key, value, 5*time.Minute)
			require.NoError(t, err)
		}

		// Tag keys
		invalidator.TagKey("user:1", []string{"user", "user:1"})
		invalidator.TagKey("user:2", []string{"user", "user:2"})
		invalidator.TagKey("profile:1", []string{"profile", "user:1"})
		invalidator.TagKey("profile:2", []string{"profile", "user:2"})
		invalidator.TagKey("other", []string{"misc"})

		// Invalidate by user:1 tag
		err := invalidator.InvalidateByTags(ctx, []string{"user:1"})
		require.NoError(t, err)

		// Verify tagged keys are invalidated
		_, err = manager.Get(ctx, "user:1")
		assert.Equal(t, ErrCacheNotFound, err)
		_, err = manager.Get(ctx, "profile:1")
		assert.Equal(t, ErrCacheNotFound, err)

		// Verify other keys still exist
		_, err = manager.Get(ctx, "user:2")
		assert.NoError(t, err)
		_, err = manager.Get(ctx, "profile:2")
		assert.NoError(t, err)
		_, err = manager.Get(ctx, "other")
		assert.NoError(t, err)
	})

	t.Run("Invalidate by multiple tags", func(t *testing.T) {
		// Set up test data
		key1, key2, key3 := "multi-tag-1", "multi-tag-2", "multi-tag-3"
		value := []byte("multi-tag-value")

		for _, key := range []string{key1, key2, key3} {
			err := manager.Set(ctx, key, value, 5*time.Minute)
			require.NoError(t, err)
		}

		// Tag keys
		invalidator.TagKey(key1, []string{"tag1", "tag2"})
		invalidator.TagKey(key2, []string{"tag2", "tag3"})
		invalidator.TagKey(key3, []string{"tag3"})

		// Invalidate by multiple tags
		err := invalidator.InvalidateByTags(ctx, []string{"tag1", "tag3"})
		require.NoError(t, err)

		// Verify keys with matching tags are invalidated
		_, err = manager.Get(ctx, key1)
		assert.Equal(t, ErrCacheNotFound, err)
		_, err = manager.Get(ctx, key3)
		assert.Equal(t, ErrCacheNotFound, err)

		// Verify key with only tag2 still exists
		_, err = manager.Get(ctx, key2)
		assert.NoError(t, err)
	})

	t.Run("Invalidate by empty tags", func(t *testing.T) {
		err := invalidator.InvalidateByTags(ctx, []string{})
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "tags cannot be empty")
	})

	t.Run("Invalidate by non-existent tags", func(t *testing.T) {
		err := invalidator.InvalidateByTags(ctx, []string{"non-existent-tag"})
		assert.NoError(t, err) // Should not error for non-existent tags
	})
}

func TestCacheInvalidator_ScheduleInvalidation(t *testing.T) {
	invalidator, manager, mr := setupTestCacheInvalidator(t)
	defer manager.Close()
	defer mr.Close()

	ctx := context.Background()

	t.Run("Schedule key invalidation", func(t *testing.T) {
		key := "scheduled-key"
		value := []byte("scheduled-value")

		// Set key
		err := manager.Set(ctx, key, value, 5*time.Minute)
		require.NoError(t, err)

		// Schedule invalidation
		delay := 100 * time.Millisecond
		err = invalidator.ScheduleInvalidation(ctx, key, delay)
		require.NoError(t, err)

		// Key should still exist immediately
		_, err = manager.Get(ctx, key)
		assert.NoError(t, err)

		// Wait for scheduled invalidation
		time.Sleep(200 * time.Millisecond)

		// Key should be invalidated
		_, err = manager.Get(ctx, key)
		assert.Equal(t, ErrCacheNotFound, err)
	})

	t.Run("Schedule invalidation with empty key", func(t *testing.T) {
		err := invalidator.ScheduleInvalidation(ctx, "", 1*time.Second)
		assert.Equal(t, ErrInvalidKey, err)
	})

	t.Run("Schedule invalidation with negative delay", func(t *testing.T) {
		err := invalidator.ScheduleInvalidation(ctx, "key", -1*time.Second)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "delay cannot be negative")
	})

	t.Run("Schedule invalidation with zero delay", func(t *testing.T) {
		key := "zero-delay-key"
		value := []byte("zero-delay-value")

		err := manager.Set(ctx, key, value, 5*time.Minute)
		require.NoError(t, err)

		err = invalidator.ScheduleInvalidation(ctx, key, 0)
		require.NoError(t, err)

		// Should be invalidated almost immediately
		time.Sleep(50 * time.Millisecond)
		_, err = manager.Get(ctx, key)
		assert.Equal(t, ErrCacheNotFound, err)
	})
}

func TestCacheInvalidator_SchedulePatternInvalidation(t *testing.T) {
	invalidator, manager, mr := setupTestCacheInvalidator(t)
	defer manager.Close()
	defer mr.Close()

	ctx := context.Background()

	t.Run("Schedule pattern invalidation", func(t *testing.T) {
		// Set up test data
		testData := map[string][]byte{
			"pattern:1": []byte("value1"),
			"pattern:2": []byte("value2"),
			"other:1":   []byte("other1"),
		}

		for key, value := range testData {
			err := manager.Set(ctx, key, value, 5*time.Minute)
			require.NoError(t, err)
		}

		// Schedule pattern invalidation
		delay := 100 * time.Millisecond
		err := invalidator.SchedulePatternInvalidation(ctx, "pattern:*", delay)
		require.NoError(t, err)

		// Keys should still exist immediately
		for key := range testData {
			_, err := manager.Get(ctx, key)
			assert.NoError(t, err)
		}

		// Wait for scheduled invalidation
		time.Sleep(200 * time.Millisecond)

		// Pattern keys should be invalidated
		_, err = manager.Get(ctx, "pattern:1")
		assert.Equal(t, ErrCacheNotFound, err)
		_, err = manager.Get(ctx, "pattern:2")
		assert.Equal(t, ErrCacheNotFound, err)

		// Other keys should still exist
		_, err = manager.Get(ctx, "other:1")
		assert.NoError(t, err)
	})

	t.Run("Schedule pattern invalidation with empty pattern", func(t *testing.T) {
		err := invalidator.SchedulePatternInvalidation(ctx, "", 1*time.Second)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "pattern cannot be empty")
	})

	t.Run("Schedule pattern invalidation with negative delay", func(t *testing.T) {
		err := invalidator.SchedulePatternInvalidation(ctx, "pattern:*", -1*time.Second)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "delay cannot be negative")
	})
}

func TestCacheInvalidator_ScheduleTagInvalidation(t *testing.T) {
	invalidator, manager, mr := setupTestCacheInvalidator(t)
	defer manager.Close()
	defer mr.Close()

	ctx := context.Background()

	t.Run("Schedule tag invalidation", func(t *testing.T) {
		// Set up test data
		keys := []string{"tag-key-1", "tag-key-2", "other-key"}
		for _, key := range keys {
			value := []byte(fmt.Sprintf("value-%s", key))
			err := manager.Set(ctx, key, value, 5*time.Minute)
			require.NoError(t, err)
		}

		// Tag keys
		invalidator.TagKey("tag-key-1", []string{"test-tag"})
		invalidator.TagKey("tag-key-2", []string{"test-tag"})
		invalidator.TagKey("other-key", []string{"other-tag"})

		// Schedule tag invalidation
		delay := 100 * time.Millisecond
		err := invalidator.ScheduleTagInvalidation(ctx, []string{"test-tag"}, delay)
		require.NoError(t, err)

		// Keys should still exist immediately
		for _, key := range keys {
			_, err := manager.Get(ctx, key)
			assert.NoError(t, err)
		}

		// Wait for scheduled invalidation
		time.Sleep(200 * time.Millisecond)

		// Tagged keys should be invalidated
		_, err = manager.Get(ctx, "tag-key-1")
		assert.Equal(t, ErrCacheNotFound, err)
		_, err = manager.Get(ctx, "tag-key-2")
		assert.Equal(t, ErrCacheNotFound, err)

		// Other key should still exist
		_, err = manager.Get(ctx, "other-key")
		assert.NoError(t, err)
	})

	t.Run("Schedule tag invalidation with empty tags", func(t *testing.T) {
		err := invalidator.ScheduleTagInvalidation(ctx, []string{}, 1*time.Second)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "tags cannot be empty")
	})

	t.Run("Schedule tag invalidation with negative delay", func(t *testing.T) {
		err := invalidator.ScheduleTagInvalidation(ctx, []string{"tag"}, -1*time.Second)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "delay cannot be negative")
	})
}

func TestCacheInvalidator_TagKey(t *testing.T) {
	invalidator, manager, mr := setupTestCacheInvalidator(t)
	defer manager.Close()
	defer mr.Close()

	t.Run("Tag key with single tag", func(t *testing.T) {
		key := "single-tag-key"
		tag := "single-tag"

		invalidator.TagKey(key, []string{tag})

		// Verify key is tagged
		taggedKeys := invalidator.GetTaggedKeys(tag)
		assert.Contains(t, taggedKeys, key)
	})

	t.Run("Tag key with multiple tags", func(t *testing.T) {
		key := "multi-tag-key"
		tags := []string{"tag1", "tag2", "tag3"}

		invalidator.TagKey(key, tags)

		// Verify key is tagged with all tags
		for _, tag := range tags {
			taggedKeys := invalidator.GetTaggedKeys(tag)
			assert.Contains(t, taggedKeys, key)
		}
	})

	t.Run("Tag same key multiple times", func(t *testing.T) {
		key := "duplicate-tag-key"
		tag := "duplicate-tag"

		// Tag the same key multiple times
		invalidator.TagKey(key, []string{tag})
		invalidator.TagKey(key, []string{tag})
		invalidator.TagKey(key, []string{tag})

		// Should only appear once
		taggedKeys := invalidator.GetTaggedKeys(tag)
		count := 0
		for _, taggedKey := range taggedKeys {
			if taggedKey == key {
				count++
			}
		}
		assert.Equal(t, 1, count)
	})

	t.Run("Tag empty key", func(t *testing.T) {
		// Should not panic or error
		invalidator.TagKey("", []string{"tag"})
		
		taggedKeys := invalidator.GetTaggedKeys("tag")
		assert.Empty(t, taggedKeys)
	})

	t.Run("Tag key with empty tags", func(t *testing.T) {
		// Should not panic or error
		invalidator.TagKey("key", []string{})
		
		// No tags should be created
		stats := invalidator.GetInvalidationStats()
		// This test just ensures no panic occurs
		assert.NotNil(t, stats)
	})
}

func TestCacheInvalidator_GetTaggedKeys(t *testing.T) {
	invalidator, manager, mr := setupTestCacheInvalidator(t)
	defer manager.Close()
	defer mr.Close()

	t.Run("Get tagged keys for existing tag", func(t *testing.T) {
		keys := []string{"tagged-key-1", "tagged-key-2", "tagged-key-3"}
		tag := "test-tag"

		for _, key := range keys {
			invalidator.TagKey(key, []string{tag})
		}

		taggedKeys := invalidator.GetTaggedKeys(tag)
		assert.Len(t, taggedKeys, len(keys))
		
		for _, key := range keys {
			assert.Contains(t, taggedKeys, key)
		}
	})

	t.Run("Get tagged keys for non-existent tag", func(t *testing.T) {
		taggedKeys := invalidator.GetTaggedKeys("non-existent-tag")
		assert.Nil(t, taggedKeys)
	})

	t.Run("Get tagged keys returns copy", func(t *testing.T) {
		key := "copy-test-key"
		tag := "copy-test-tag"

		invalidator.TagKey(key, []string{tag})
		
		taggedKeys1 := invalidator.GetTaggedKeys(tag)
		taggedKeys2 := invalidator.GetTaggedKeys(tag)
		
		// Should be different slices (copies) but with same content
		assert.Equal(t, taggedKeys1, taggedKeys2)
	})
}

func TestCacheInvalidator_GetInvalidationStats(t *testing.T) {
	invalidator, manager, mr := setupTestCacheInvalidator(t)
	defer manager.Close()
	defer mr.Close()

	ctx := context.Background()

	t.Run("Get initial stats", func(t *testing.T) {
		stats := invalidator.GetInvalidationStats()
		assert.Equal(t, 0, stats["scheduled_jobs"])
		assert.Equal(t, 0, stats["tagged_keys_count"])
		assert.Equal(t, 0, stats["total_tagged_keys"])
		assert.Equal(t, 0, stats["active_jobs"])
	})

	t.Run("Get stats after operations", func(t *testing.T) {
		// Tag some keys
		invalidator.TagKey("stats-key-1", []string{"stats-tag-1"})
		invalidator.TagKey("stats-key-2", []string{"stats-tag-1", "stats-tag-2"})
		invalidator.TagKey("stats-key-3", []string{"stats-tag-2"})

		// Schedule some invalidations
		err := invalidator.ScheduleInvalidation(ctx, "stats-key-1", 1*time.Hour)
		require.NoError(t, err)
		err = invalidator.SchedulePatternInvalidation(ctx, "stats:*", 1*time.Hour)
		require.NoError(t, err)

		stats := invalidator.GetInvalidationStats()
		assert.Equal(t, 2, stats["scheduled_jobs"])
		assert.Equal(t, 2, stats["tagged_keys_count"]) // 2 unique tags
		assert.Equal(t, 4, stats["total_tagged_keys"])  // 4 total key-tag associations
		assert.Equal(t, 2, stats["active_jobs"])
	})
}

func TestCacheInvalidator_ClearScheduledInvalidations(t *testing.T) {
	invalidator, manager, mr := setupTestCacheInvalidator(t)
	defer manager.Close()
	defer mr.Close()

	ctx := context.Background()

	t.Run("Clear scheduled invalidations", func(t *testing.T) {
		// Schedule some invalidations
		err := invalidator.ScheduleInvalidation(ctx, "clear-key-1", 1*time.Hour)
		require.NoError(t, err)
		err = invalidator.ScheduleInvalidation(ctx, "clear-key-2", 1*time.Hour)
		require.NoError(t, err)

		// Verify jobs are scheduled
		stats := invalidator.GetInvalidationStats()
		assert.Equal(t, 2, stats["scheduled_jobs"])

		// Clear scheduled invalidations
		invalidator.ClearScheduledInvalidations()

		// Verify jobs are cleared
		stats = invalidator.GetInvalidationStats()
		assert.Equal(t, 0, stats["scheduled_jobs"])
	})
}

func TestCacheInvalidator_Stop(t *testing.T) {
	invalidator, manager, mr := setupTestCacheInvalidator(t)
	defer manager.Close()
	defer mr.Close()

	t.Run("Stop invalidator", func(t *testing.T) {
		// This should not panic or error
		invalidator.Stop()
		
		// Verify it can be called multiple times
		invalidator.Stop()
	})
}

func TestCacheInvalidator_Concurrent(t *testing.T) {
	invalidator, manager, mr := setupTestCacheInvalidator(t)
	defer manager.Close()
	defer mr.Close()

	ctx := context.Background()

	t.Run("Concurrent invalidation operations", func(t *testing.T) {
		numGoroutines := 20
		keysPerGoroutine := 10

		// Set up test data
		allKeys := make([]string, 0, numGoroutines*keysPerGoroutine)
		for i := 0; i < numGoroutines; i++ {
			for j := 0; j < keysPerGoroutine; j++ {
				key := fmt.Sprintf("concurrent-key-%d-%d", i, j)
				value := []byte(fmt.Sprintf("concurrent-value-%d-%d", i, j))
				err := manager.Set(ctx, key, value, 5*time.Minute)
				require.NoError(t, err)
				allKeys = append(allKeys, key)
			}
		}

		// Perform concurrent invalidations
		var wg sync.WaitGroup
		wg.Add(numGoroutines)

		for i := 0; i < numGoroutines; i++ {
			go func(id int) {
				defer wg.Done()
				
				for j := 0; j < keysPerGoroutine; j++ {
					key := fmt.Sprintf("concurrent-key-%d-%d", id, j)
					
					// Tag key
					tag := fmt.Sprintf("tag-%d", id)
					invalidator.TagKey(key, []string{tag})
					
					// Invalidate key
					err := invalidator.InvalidateKey(ctx, key)
					assert.NoError(t, err)
				}
			}(i)
		}

		wg.Wait()

		// Verify all keys are invalidated
		for _, key := range allKeys {
			_, err := manager.Get(ctx, key)
			assert.Equal(t, ErrCacheNotFound, err, "Key %s should be invalidated", key)
		}
	})
}

func BenchmarkCacheInvalidator_InvalidateKey(b *testing.B) {
	invalidator, manager, mr := setupTestCacheInvalidator(&testing.T{})
	defer manager.Close()
	defer mr.Close()

	ctx := context.Background()

	// Pre-populate cache
	keys := make([]string, 1000)
	for i := 0; i < 1000; i++ {
		key := fmt.Sprintf("benchmark-key-%d", i)
		value := []byte(fmt.Sprintf("benchmark-value-%d", i))
		keys[i] = key
		manager.Set(ctx, key, value, 5*time.Minute)
	}

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			key := keys[i%1000]
			invalidator.InvalidateKey(ctx, key)
			i++
		}
	})
}