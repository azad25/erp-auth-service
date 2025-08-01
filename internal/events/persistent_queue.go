package events

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

// PersistentQueue provides local file-based persistence for events
type PersistentQueue struct {
	storagePath string
	logger      *zap.Logger
	mu          sync.RWMutex
}

// NewPersistentQueue creates a new persistent queue
func NewPersistentQueue(storagePath string, logger *zap.Logger) (*PersistentQueue, error) {
	// Create storage directory if it doesn't exist
	if err := os.MkdirAll(storagePath, 0755); err != nil {
		return nil, fmt.Errorf("failed to create storage directory: %w", err)
	}
	
	return &PersistentQueue{
		storagePath: storagePath,
		logger:      logger,
	}, nil
}

// Enqueue adds an event to persistent storage
func (pq *PersistentQueue) Enqueue(event *QueuedEvent) error {
	pq.mu.Lock()
	defer pq.mu.Unlock()
	
	filename := fmt.Sprintf("%s.json", event.ID.String())
	filepath := filepath.Join(pq.storagePath, filename)
	
	data, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("failed to marshal event: %w", err)
	}
	
	if err := os.WriteFile(filepath, data, 0644); err != nil {
		return fmt.Errorf("failed to write event to file: %w", err)
	}
	
	pq.logger.Debug("Event persisted to disk",
		zap.String("event_id", event.ID.String()),
		zap.String("filepath", filepath))
	
	return nil
}

// Remove removes an event from persistent storage
func (pq *PersistentQueue) Remove(eventID uuid.UUID) error {
	pq.mu.Lock()
	defer pq.mu.Unlock()
	
	filename := fmt.Sprintf("%s.json", eventID.String())
	filepath := filepath.Join(pq.storagePath, filename)
	
	if err := os.Remove(filepath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove event file: %w", err)
	}
	
	return nil
}

// GetAll retrieves all persisted events
func (pq *PersistentQueue) GetAll() ([]*QueuedEvent, error) {
	pq.mu.RLock()
	defer pq.mu.RUnlock()
	
	var events []*QueuedEvent
	
	err := filepath.WalkDir(pq.storagePath, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		
		if d.IsDir() || filepath.Ext(path) != ".json" {
			return nil
		}
		
		data, err := os.ReadFile(path)
		if err != nil {
			pq.logger.Warn("Failed to read event file", zap.String("path", path), zap.Error(err))
			return nil // Continue processing other files
		}
		
		var event QueuedEvent
		if err := json.Unmarshal(data, &event); err != nil {
			pq.logger.Warn("Failed to unmarshal event", zap.String("path", path), zap.Error(err))
			return nil // Continue processing other files
		}
		
		events = append(events, &event)
		return nil
	})
	
	if err != nil {
		return nil, fmt.Errorf("failed to walk storage directory: %w", err)
	}
	
	return events, nil
}

// GetRetryableEvents retrieves events that are ready for retry
func (pq *PersistentQueue) GetRetryableEvents() ([]*QueuedEvent, error) {
	allEvents, err := pq.GetAll()
	if err != nil {
		return nil, err
	}
	
	var retryableEvents []*QueuedEvent
	now := time.Now()
	
	for _, event := range allEvents {
		if event.Status == EventStatusFailed && now.After(event.NextRetry) {
			retryableEvents = append(retryableEvents, event)
		}
	}
	
	return retryableEvents, nil
}

// GetCount returns the number of persisted events
func (pq *PersistentQueue) GetCount() (int, error) {
	pq.mu.RLock()
	defer pq.mu.RUnlock()
	
	count := 0
	err := filepath.WalkDir(pq.storagePath, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		
		if !d.IsDir() && filepath.Ext(path) == ".json" {
			count++
		}
		
		return nil
	})
	
	return count, err
}

// Cleanup removes old events from persistent storage
func (pq *PersistentQueue) Cleanup(maxAge time.Duration) error {
	pq.mu.Lock()
	defer pq.mu.Unlock()
	
	cutoff := time.Now().Add(-maxAge)
	removed := 0
	
	err := filepath.WalkDir(pq.storagePath, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		
		if d.IsDir() || filepath.Ext(path) != ".json" {
			return nil
		}
		
		info, err := d.Info()
		if err != nil {
			return nil // Continue processing
		}
		
		if info.ModTime().Before(cutoff) {
			if err := os.Remove(path); err != nil {
				pq.logger.Warn("Failed to remove old event file", zap.String("path", path), zap.Error(err))
			} else {
				removed++
			}
		}
		
		return nil
	})
	
	if removed > 0 {
		pq.logger.Info("Cleaned up old persisted events", zap.Int("removed", removed))
	}
	
	return err
}

// Close closes the persistent queue
func (pq *PersistentQueue) Close() error {
	// No resources to close for file-based storage
	return nil
}