package cache

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"
)

func TestBigCacheImpl_Get(t *testing.T) {
	logger := zaptest.NewLogger(t)
	config := BigCacheConfig{
		Shards:             1024,
		LifeWindow:         10 * time.Minute,
		CleanWindow:        5 * time.Minute,
		MaxEntriesInWindow: 1000,
		MaxEntrySize:       500,
		HardMaxCacheSize:   8,
		Verbose:            false,
	}

	cache, err := NewBigCache("test", config, logger)
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

		// Wait for expiration
		time.Sleep(10 * time.Millisecond)

		_, err = cache.Get(ctx, key)
		assert.Equal(t, ErrCacheExpired, err)
	})
}

func TestBigCacheImpl_Set(t *testing.T) {
	logger := zaptest.NewLogger(t)
	config := BigCacheConfig{
		Shards:             1024,
		LifeWindow:         10 * time.Minute,
		CleanWindow:        5 * time.Minute,
		MaxEntriesInWindow: 1000,
		MaxEntrySize:       500,
		HardMaxCacheSize:   8,
		Verbose:            false,
	}

	cache, err := NewBigCache("test", config, logger)
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

	t.Run("Set with zero TTL", func(t *testing.T) {
		key := "zero-ttl-key"
		value := []byte("zero-ttl-value")
		
		err := cache.Set(ctx, key, value, 0)
		assert.NoError(t, err)

		// Should still be retrievable
		retrieved, err := cache.Get(ctx, key)
		require.NoError(t, err)
		assert.Equal(t, value, retrieved)
	})
}

func TestBigCacheImpl_Delete(t *testing.T) {
	logger := zaptest.NewLogger(t)
	config := BigCacheConfig{
		Shards:             1024,
		LifeWindow:         10 * time.Minute,
		CleanWindow:        5 * time.Minute,
		MaxEntriesInWindow: 1000,
		MaxEntrySize:       500,
		HardMaxCacheSize:   8,
		Verbose:            false,
	}

	cache, err := NewBigCache("test", config, logger)
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

func TestBigCacheImpl_Exists(t *testing.T) {
	logger := zaptest.NewLogger(t)
	config := BigCacheConfig{
		Shards:             1024,
		LifeWindow:         10 * time.Minute,
		CleanWindow:        5 * time.Minute,
		MaxEntriesInWindow: 1000,
		MaxEntrySize:       500,
		HardMaxCacheSize:   8,
		Verbose:            false,
	}

	cache, err := NewBigCache("test", config, logger)
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

func TestBigCacheImpl_Clear(t *testing.T) {
	logger := zaptest.NewLogger(t)
	config := BigCacheConfig{
		Shards:             1024,
		LifeWindow:         10 * time.Minute,
		CleanWindow:        5 * time.Minute,
		MaxEntriesInWindow: 1000,
		MaxEntrySize:       500,
		HardMaxCacheSize:   8,
		Verbose:            false,
	}

	cache, err := NewBigCache("test", config, logger)
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

func TestBigCacheImpl_Stats(t *testing.T) {
	logger := zaptest.NewLogger(t)
	config := BigCacheConfig{
		Shards:             1024,
		LifeWindow:         10 * time.Minute,
		CleanWindow:        5 * time.Minute,
		MaxEntriesInWindow: 1000,
		MaxEntrySize:       500,
		HardMaxCacheSize:   8,
		Verbose:            false,
	}

	cache, err := NewBigCache("test", config, logger)
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

func TestBigCacheImpl_Concurrent(t *testing.T) {
	logger := zaptest.NewLogger(t)
	config := BigCacheConfig{
		Shards:             1024,
		LifeWindow:         10 * time.Minute,
		CleanWindow:        5 * time.Minute,
		MaxEntriesInWindow: 1000,
		MaxEntrySize:       500,
		HardMaxCacheSize:   8,
		Verbose:            false,
	}

	cache, err := NewBigCache("test", config, logger)
	require.NoError(t, err)
	defer cache.Close()

	ctx := context.Background()
	numGoroutines := 100
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

func BenchmarkBigCacheImpl_Set(b *testing.B) {
	logger := zaptest.NewLogger(b)
	config := BigCacheConfig{
		Shards:             1024,
		LifeWindow:         10 * time.Minute,
		CleanWindow:        5 * time.Minute,
		MaxEntriesInWindow: 1000,
		MaxEntrySize:       500,
		HardMaxCacheSize:   8,
		Verbose:            false,
	}

	cache, err := NewBigCache("benchmark", config, logger)
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

func BenchmarkBigCacheImpl_Get(b *testing.B) {
	logger := zaptest.NewLogger(b)
	config := BigCacheConfig{
		Shards:             1024,
		LifeWindow:         10 * time.Minute,
		CleanWindow:        5 * time.Minute,
		MaxEntriesInWindow: 1000,
		MaxEntrySize:       500,
		HardMaxCacheSize:   8,
		Verbose:            false,
	}

	cache, err := NewBigCache("benchmark", config, logger)
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