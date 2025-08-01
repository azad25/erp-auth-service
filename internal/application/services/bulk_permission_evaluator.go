package services

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

// BulkPermissionEvaluator handles bulk permission evaluations with concurrency optimization
type BulkPermissionEvaluator struct {
	permissionService *PermissionService
	logger           *zap.Logger
	workerPool       *WorkerPool
	
	// Configuration
	maxWorkers     int
	batchSize      int
	evaluationTimeout time.Duration
}

// BulkEvaluationJob represents a single evaluation job in the bulk processor
type BulkEvaluationJob struct {
	UserID      uuid.UUID
	Request     PermissionCheckRequest
	ResultIndex int
	ResultChan  chan<- BulkEvaluationResult
}

// BulkEvaluationResult represents the result of a bulk evaluation job
type BulkEvaluationResult struct {
	Index  int
	Result PermissionEvaluationResult
	Error  error
}

// NewBulkPermissionEvaluator creates a new bulk permission evaluator
func NewBulkPermissionEvaluator(permissionService *PermissionService, logger *zap.Logger) *BulkPermissionEvaluator {
	evaluator := &BulkPermissionEvaluator{
		permissionService: permissionService,
		logger:           logger.With(zap.String("component", "bulk_permission_evaluator")),
		maxWorkers:       10, // Configurable
		batchSize:        50, // Configurable
		evaluationTimeout: 5 * time.Second,
	}

	// Initialize worker pool
	evaluator.workerPool = NewWorkerPool("bulk_permission_evaluator", evaluator.maxWorkers, logger)

	return evaluator
}

// EvaluateBulk performs bulk permission evaluation with concurrency optimization
func (bpe *BulkPermissionEvaluator) EvaluateBulk(ctx context.Context, request *BulkPermissionRequest) (*BulkPermissionResponse, error) {
	if len(request.Permissions) == 0 {
		return &BulkPermissionResponse{
			UserID:  request.UserID,
			Results: []PermissionEvaluationResult{},
		}, nil
	}

	// Create result channel and slice
	resultChan := make(chan BulkEvaluationResult, len(request.Permissions))
	results := make([]PermissionEvaluationResult, len(request.Permissions))

	// Start worker pool
	bpe.workerPool.Start()
	defer bpe.workerPool.Stop()

	// Submit jobs to worker pool
	for i, permReq := range request.Permissions {
		evalJob := BulkEvaluationJob{
			UserID:      request.UserID,
			Request:     permReq,
			ResultIndex: i,
			ResultChan:  resultChan,
		}

		// Create job function that captures the evaluation job
		job := func() {
			bpe.processEvaluationJobFunc(ctx, evalJob)
		}

		bpe.workerPool.Submit(job)
	}

	// Collect results
	var wg sync.WaitGroup
	wg.Add(1)
	
	go func() {
		defer wg.Done()
		for i := 0; i < len(request.Permissions); i++ {
			select {
			case result := <-resultChan:
				if result.Error != nil {
					results[result.Index] = PermissionEvaluationResult{
						Allowed:     false,
						Reason:      fmt.Sprintf("evaluation error: %v", result.Error),
						EvaluatedAt: time.Now(),
					}
				} else {
					results[result.Index] = result.Result
				}
			case <-ctx.Done():
				// Fill remaining results with timeout errors
				for j := i; j < len(request.Permissions); j++ {
					results[j] = PermissionEvaluationResult{
						Allowed:     false,
						Reason:      "evaluation timeout",
						EvaluatedAt: time.Now(),
					}
				}
				return
			}
		}
	}()

	// Wait for all results with timeout
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		bpe.logger.Debug("Bulk permission evaluation completed",
			zap.String("user_id", request.UserID.String()),
			zap.Int("permission_count", len(request.Permissions)))
	case <-time.After(bpe.evaluationTimeout):
		bpe.logger.Warn("Bulk permission evaluation timeout",
			zap.String("user_id", request.UserID.String()),
			zap.Int("permission_count", len(request.Permissions)))
	case <-ctx.Done():
		return nil, ctx.Err()
	}

	return &BulkPermissionResponse{
		UserID:  request.UserID,
		Results: results,
	}, nil
}

// processEvaluationJobFunc processes a single evaluation job as a function
func (bpe *BulkPermissionEvaluator) processEvaluationJobFunc(ctx context.Context, evalJob BulkEvaluationJob) {
	// Create evaluation context with timeout
	evalCtx, cancel := context.WithTimeout(ctx, 1*time.Second)
	defer cancel()

	// Perform permission evaluation
	result, err := bpe.permissionService.evaluatePermission(
		evalCtx,
		evalJob.UserID,
		evalJob.Request.Resource,
		evalJob.Request.Action,
		evalJob.Request.Scope,
	)

	// Send result
	bulkResult := BulkEvaluationResult{
		Index: evalJob.ResultIndex,
		Error: err,
	}

	if result != nil {
		bulkResult.Result = *result
	}

	select {
	case evalJob.ResultChan <- bulkResult:
	case <-ctx.Done():
		// Context cancelled, ignore
	}
}

// EvaluateBulkWithBatching performs bulk evaluation with batching for very large requests
func (bpe *BulkPermissionEvaluator) EvaluateBulkWithBatching(ctx context.Context, request *BulkPermissionRequest) (*BulkPermissionResponse, error) {
	if len(request.Permissions) <= bpe.batchSize {
		return bpe.EvaluateBulk(ctx, request)
	}

	// Process in batches
	allResults := make([]PermissionEvaluationResult, len(request.Permissions))
	
	for i := 0; i < len(request.Permissions); i += bpe.batchSize {
		end := i + bpe.batchSize
		if end > len(request.Permissions) {
			end = len(request.Permissions)
		}

		// Create batch request
		batchRequest := &BulkPermissionRequest{
			UserID:      request.UserID,
			Permissions: request.Permissions[i:end],
			Context:     request.Context,
		}

		// Process batch
		batchResponse, err := bpe.EvaluateBulk(ctx, batchRequest)
		if err != nil {
			return nil, fmt.Errorf("batch evaluation failed: %w", err)
		}

		// Copy results
		copy(allResults[i:end], batchResponse.Results)

		bpe.logger.Debug("Processed permission batch",
			zap.String("user_id", request.UserID.String()),
			zap.Int("batch_start", i),
			zap.Int("batch_end", end))
	}

	return &BulkPermissionResponse{
		UserID:  request.UserID,
		Results: allResults,
	}, nil
}

// OptimizedBulkEvaluate performs optimized bulk evaluation with caching and deduplication
func (bpe *BulkPermissionEvaluator) OptimizedBulkEvaluate(ctx context.Context, request *BulkPermissionRequest) (*BulkPermissionResponse, error) {
	// Deduplicate permission requests
	uniqueRequests, indexMap := bpe.deduplicateRequests(request.Permissions)
	
	bpe.logger.Debug("Deduplicated permission requests",
		zap.Int("original_count", len(request.Permissions)),
		zap.Int("unique_count", len(uniqueRequests)))

	// Create deduplicated request
	deduplicatedRequest := &BulkPermissionRequest{
		UserID:      request.UserID,
		Permissions: uniqueRequests,
		Context:     request.Context,
	}

	// Evaluate unique permissions
	response, err := bpe.EvaluateBulk(ctx, deduplicatedRequest)
	if err != nil {
		return nil, err
	}

	// Map results back to original request order
	results := make([]PermissionEvaluationResult, len(request.Permissions))
	for originalIndex, uniqueIndex := range indexMap {
		results[originalIndex] = response.Results[uniqueIndex]
	}

	return &BulkPermissionResponse{
		UserID:  request.UserID,
		Results: results,
	}, nil
}

// deduplicateRequests removes duplicate permission requests and returns mapping
func (bpe *BulkPermissionEvaluator) deduplicateRequests(requests []PermissionCheckRequest) ([]PermissionCheckRequest, []int) {
	seen := make(map[string]int)
	unique := make([]PermissionCheckRequest, 0, len(requests))
	indexMap := make([]int, len(requests))

	for i, req := range requests {
		key := fmt.Sprintf("%s:%s:%s", req.Resource, req.Action, req.Scope)
		
		if uniqueIndex, exists := seen[key]; exists {
			indexMap[i] = uniqueIndex
		} else {
			uniqueIndex := len(unique)
			seen[key] = uniqueIndex
			unique = append(unique, req)
			indexMap[i] = uniqueIndex
		}
	}

	return unique, indexMap
}

// GetStats returns statistics about bulk evaluation performance
func (bpe *BulkPermissionEvaluator) GetStats() map[string]interface{} {
	return map[string]interface{}{
		"max_workers":        bpe.maxWorkers,
		"batch_size":         bpe.batchSize,
		"evaluation_timeout": bpe.evaluationTimeout.String(),
		"worker_pool_stats":  bpe.workerPool.GetMetrics(),
	}
}

// Close gracefully shuts down the bulk evaluator
func (bpe *BulkPermissionEvaluator) Close() error {
	bpe.workerPool.Stop()
	return nil
}