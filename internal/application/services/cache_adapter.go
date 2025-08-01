package services

import (
	"context"
	"encoding/json"
	"time"

	"erp-auth-service/internal/cache"
)

// CacheAdapter adapts the existing cache.CacheManager to SpecializedCacheManager interface
type CacheAdapter struct {
	cacheManager cache.CacheManager
}

// NewCacheAdapter creates a new cache adapter
func NewCacheAdapter(cacheManager cache.CacheManager) *CacheAdapter {
	return &CacheAdapter{
		cacheManager: cacheManager,
	}
}

// Get retrieves a value from cache and deserializes it
func (ca *CacheAdapter) Get(ctx context.Context, key string) (interface{}, error) {
	data, err := ca.cacheManager.Get(ctx, key)
	if err != nil {
		return nil, err
	}
	
	// Try to deserialize as JSON
	var value interface{}
	if err := json.Unmarshal(data, &value); err != nil {
		// If JSON deserialization fails, return as string
		return string(data), nil
	}
	
	return value, nil
}

// Set serializes and stores a value in cache
func (ca *CacheAdapter) Set(ctx context.Context, key string, value interface{}, ttl time.Duration) error {
	// Serialize value to JSON
	data, err := json.Marshal(value)
	if err != nil {
		// If JSON serialization fails, try to convert to string
		if str, ok := value.(string); ok {
			data = []byte(str)
		} else {
			return err
		}
	}
	
	return ca.cacheManager.Set(ctx, key, data, ttl)
}

// Delete removes a key from cache
func (ca *CacheAdapter) Delete(ctx context.Context, key string) error {
	return ca.cacheManager.Delete(ctx, key)
}

// GetMultiple retrieves multiple values from cache
func (ca *CacheAdapter) GetMultiple(ctx context.Context, keys []string) (map[string]interface{}, error) {
	result := make(map[string]interface{})
	
	// Get each key individually since the base cache manager doesn't support bulk operations
	for _, key := range keys {
		value, err := ca.Get(ctx, key)
		if err != nil {
			// Skip missing keys
			continue
		}
		result[key] = value
	}
	
	return result, nil
}

// SetMultiple stores multiple values in cache
func (ca *CacheAdapter) SetMultiple(ctx context.Context, items map[string]interface{}, ttl time.Duration) error {
	// Set each key individually since the base cache manager doesn't support bulk operations
	for key, value := range items {
		if err := ca.Set(ctx, key, value, ttl); err != nil {
			return err
		}
	}
	
	return nil
}