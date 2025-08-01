package services

import (
	"context"
	"sync"
	"time"

	"go.uber.org/zap"
)

// WorkerPool manages a pool of workers for CPU-intensive operations
type WorkerPool struct {
	name       string
	size       int
	logger     *zap.Logger
	jobQueue   chan func()
	workers    []*Worker
	wg         sync.WaitGroup
	ctx        context.Context
	cancel     context.CancelFunc
	started    bool
	mu         sync.RWMutex
	
	// Metrics
	metrics *WorkerPoolMetrics
}

// Worker represents a single worker in the pool
type Worker struct {
	id       int
	pool     *WorkerPool
	jobQueue chan func()
	quit     chan bool
	logger   *zap.Logger
}

// WorkerPoolMetrics tracks worker pool performance
type WorkerPoolMetrics struct {
	JobsSubmitted   int64
	JobsCompleted   int64
	JobsInProgress  int64
	ActiveWorkers   int64
	QueueLength     int64
	AverageJobTime  time.Duration
	mu              sync.RWMutex
}

// NewWorkerPool creates a new worker pool
func NewWorkerPool(name string, size int, logger *zap.Logger) *WorkerPool {
	ctx, cancel := context.WithCancel(context.Background())
	
	return &WorkerPool{
		name:     name,
		size:     size,
		logger:   logger.With(zap.String("component", "worker_pool"), zap.String("pool", name)),
		jobQueue: make(chan func(), size*2), // Buffer for jobs
		workers:  make([]*Worker, size),
		ctx:      ctx,
		cancel:   cancel,
		metrics:  &WorkerPoolMetrics{},
	}
}

// Start initializes and starts all workers in the pool
func (wp *WorkerPool) Start() {
	wp.mu.Lock()
	defer wp.mu.Unlock()
	
	if wp.started {
		wp.logger.Warn("Worker pool already started")
		return
	}
	
	wp.logger.Info("Starting worker pool", zap.Int("size", wp.size))
	
	// Create and start workers
	for i := 0; i < wp.size; i++ {
		worker := &Worker{
			id:       i,
			pool:     wp,
			jobQueue: wp.jobQueue,
			quit:     make(chan bool),
			logger:   wp.logger.With(zap.Int("worker_id", i)),
		}
		
		wp.workers[i] = worker
		wp.wg.Add(1)
		go worker.start()
	}
	
	wp.started = true
	wp.updateMetrics(func(m *WorkerPoolMetrics) {
		m.ActiveWorkers = int64(wp.size)
	})
	
	wp.logger.Info("Worker pool started successfully")
}

// Submit submits a job to the worker pool
func (wp *WorkerPool) Submit(job func()) {
	wp.updateMetrics(func(m *WorkerPoolMetrics) {
		m.JobsSubmitted++
		m.QueueLength = int64(len(wp.jobQueue))
	})
	
	select {
	case wp.jobQueue <- job:
		// Job submitted successfully
	case <-wp.ctx.Done():
		wp.logger.Warn("Cannot submit job, worker pool is shutting down")
	default:
		// Queue is full, handle gracefully
		wp.logger.Warn("Worker pool queue is full, job rejected")
	}
}

// SubmitWithTimeout submits a job with a timeout
func (wp *WorkerPool) SubmitWithTimeout(job func(), timeout time.Duration) bool {
	wp.updateMetrics(func(m *WorkerPoolMetrics) {
		m.JobsSubmitted++
		m.QueueLength = int64(len(wp.jobQueue))
	})
	
	select {
	case wp.jobQueue <- job:
		return true
	case <-time.After(timeout):
		wp.logger.Warn("Job submission timed out", zap.Duration("timeout", timeout))
		return false
	case <-wp.ctx.Done():
		wp.logger.Warn("Cannot submit job, worker pool is shutting down")
		return false
	}
}

// Stop gracefully shuts down the worker pool
func (wp *WorkerPool) Stop() {
	wp.mu.Lock()
	defer wp.mu.Unlock()
	
	if !wp.started {
		wp.logger.Warn("Worker pool not started")
		return
	}
	
	wp.logger.Info("Stopping worker pool")
	
	// Signal all workers to stop
	for _, worker := range wp.workers {
		close(worker.quit)
	}
	
	// Cancel context to stop accepting new jobs
	wp.cancel()
	
	// Wait for all workers to finish
	wp.wg.Wait()
	
	// Close job queue
	close(wp.jobQueue)
	
	wp.started = false
	wp.updateMetrics(func(m *WorkerPoolMetrics) {
		m.ActiveWorkers = 0
	})
	
	wp.logger.Info("Worker pool stopped")
}

// GetMetrics returns current worker pool metrics
func (wp *WorkerPool) GetMetrics() WorkerPoolMetrics {
	wp.metrics.mu.RLock()
	defer wp.metrics.mu.RUnlock()
	
	// Update queue length
	wp.metrics.QueueLength = int64(len(wp.jobQueue))
	
	return *wp.metrics
}

// GetStatus returns the current status of the worker pool
func (wp *WorkerPool) GetStatus() map[string]interface{} {
	wp.mu.RLock()
	defer wp.mu.RUnlock()
	
	metrics := wp.GetMetrics()
	
	return map[string]interface{}{
		"name":             wp.name,
		"size":             wp.size,
		"started":          wp.started,
		"jobs_submitted":   metrics.JobsSubmitted,
		"jobs_completed":   metrics.JobsCompleted,
		"jobs_in_progress": metrics.JobsInProgress,
		"active_workers":   metrics.ActiveWorkers,
		"queue_length":     metrics.QueueLength,
		"average_job_time": metrics.AverageJobTime.String(),
	}
}

// updateMetrics safely updates worker pool metrics
func (wp *WorkerPool) updateMetrics(updateFunc func(*WorkerPoolMetrics)) {
	wp.metrics.mu.Lock()
	defer wp.metrics.mu.Unlock()
	updateFunc(wp.metrics)
}

// Worker methods

// start begins the worker's job processing loop
func (w *Worker) start() {
	defer w.pool.wg.Done()
	
	w.logger.Debug("Worker started")
	
	for {
		select {
		case job := <-w.jobQueue:
			if job != nil {
				w.executeJob(job)
			}
		case <-w.quit:
			w.logger.Debug("Worker stopping")
			return
		case <-w.pool.ctx.Done():
			w.logger.Debug("Worker stopping due to context cancellation")
			return
		}
	}
}

// executeJob executes a single job with metrics tracking
func (w *Worker) executeJob(job func()) {
	startTime := time.Now()
	
	w.pool.updateMetrics(func(m *WorkerPoolMetrics) {
		m.JobsInProgress++
	})
	
	defer func() {
		duration := time.Since(startTime)
		
		w.pool.updateMetrics(func(m *WorkerPoolMetrics) {
			m.JobsInProgress--
			m.JobsCompleted++
			
			// Update average job time (simple moving average)
			if m.JobsCompleted == 1 {
				m.AverageJobTime = duration
			} else {
				m.AverageJobTime = (m.AverageJobTime + duration) / 2
			}
		})
		
		// Handle panics gracefully
		if r := recover(); r != nil {
			w.logger.Error("Job panicked", 
				zap.Any("panic", r),
				zap.Duration("duration", duration))
		}
	}()
	
	// Execute the job
	job()
	
	w.logger.Debug("Job completed", 
		zap.Duration("duration", time.Since(startTime)))
}

// WorkerPoolManager manages multiple worker pools
type WorkerPoolManager struct {
	pools  map[string]*WorkerPool
	logger *zap.Logger
	mu     sync.RWMutex
}

// NewWorkerPoolManager creates a new worker pool manager
func NewWorkerPoolManager(logger *zap.Logger) *WorkerPoolManager {
	return &WorkerPoolManager{
		pools:  make(map[string]*WorkerPool),
		logger: logger.With(zap.String("component", "worker_pool_manager")),
	}
}

// CreatePool creates a new worker pool
func (wpm *WorkerPoolManager) CreatePool(name string, size int) *WorkerPool {
	wpm.mu.Lock()
	defer wpm.mu.Unlock()
	
	if _, exists := wpm.pools[name]; exists {
		wpm.logger.Warn("Worker pool already exists", zap.String("name", name))
		return wpm.pools[name]
	}
	
	pool := NewWorkerPool(name, size, wpm.logger)
	wpm.pools[name] = pool
	
	wpm.logger.Info("Worker pool created", 
		zap.String("name", name), 
		zap.Int("size", size))
	
	return pool
}

// GetPool returns a worker pool by name
func (wpm *WorkerPoolManager) GetPool(name string) *WorkerPool {
	wpm.mu.RLock()
	defer wpm.mu.RUnlock()
	
	return wpm.pools[name]
}

// StartAll starts all worker pools
func (wpm *WorkerPoolManager) StartAll() {
	wpm.mu.RLock()
	defer wpm.mu.RUnlock()
	
	for name, pool := range wpm.pools {
		wpm.logger.Info("Starting worker pool", zap.String("name", name))
		pool.Start()
	}
}

// StopAll stops all worker pools
func (wpm *WorkerPoolManager) StopAll() {
	wpm.mu.RLock()
	defer wpm.mu.RUnlock()
	
	for name, pool := range wpm.pools {
		wpm.logger.Info("Stopping worker pool", zap.String("name", name))
		pool.Stop()
	}
}

// GetAllMetrics returns metrics for all worker pools
func (wpm *WorkerPoolManager) GetAllMetrics() map[string]WorkerPoolMetrics {
	wpm.mu.RLock()
	defer wpm.mu.RUnlock()
	
	metrics := make(map[string]WorkerPoolMetrics)
	for name, pool := range wpm.pools {
		metrics[name] = pool.GetMetrics()
	}
	
	return metrics
}

// GetAllStatus returns status for all worker pools
func (wpm *WorkerPoolManager) GetAllStatus() map[string]interface{} {
	wpm.mu.RLock()
	defer wpm.mu.RUnlock()
	
	status := make(map[string]interface{})
	for name, pool := range wpm.pools {
		status[name] = pool.GetStatus()
	}
	
	return status
}