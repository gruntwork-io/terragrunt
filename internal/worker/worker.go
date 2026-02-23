// Package worker provides a concurrent task execution system with a configurable number of workers.
//
// It allows for controlled parallel execution of tasks while managing resources efficiently through
// a semaphore-based worker pool. Key features include:
//
// - Configurable maximum number of concurrent workers
// - Non-blocking task submission
// - Graceful shutdown capabilities
// - Error collection and aggregation
// - Thread-safe operations
//
// The Pool struct manages a pool of workers that can execute tasks concurrently while
// limiting the number of goroutines running simultaneously. This prevents resource exhaustion
// while maximizing throughput.
//
// This implementation is particularly useful for scenarios where you need to process many
// independent tasks with controlled parallelism, such as in infrastructure management tools,
// batch processing systems, or any application requiring concurrent execution with resource
// constraints.
package worker

import (
	"sync"
	"sync/atomic"

	"github.com/gruntwork-io/terragrunt/internal/errors"
)

// Task represents a unit of work that can be executed
type Task func() error

// Pool manages concurrent task execution with a configurable number of workers
type Pool struct {
	semaphore   chan struct{}
	allErrors   *errors.MultiError
	wg          sync.WaitGroup
	maxWorkers  int
	mu          sync.RWMutex
	allErrorsMu sync.RWMutex
	isStopping  atomic.Bool
	isRunning   bool
}

// NewWorkerPool creates a new worker pool with the specified maximum number of concurrent workers
func NewWorkerPool(maxWorkers int) *Pool {
	if maxWorkers <= 0 {
		maxWorkers = 1
	}

	return &Pool{
		maxWorkers: maxWorkers,
		semaphore:  make(chan struct{}, maxWorkers),
		isRunning:  false,
		allErrors:  &errors.MultiError{},
	}
}

// Start initializes the worker pool
func (wp *Pool) Start() {
	wp.mu.Lock()

	if wp.isRunning {
		wp.mu.Unlock()
		return
	}

	wp.isRunning = true
	wp.isStopping.Store(false)

	wp.semaphore = make(chan struct{}, wp.maxWorkers)

	// Reset allErrors
	wp.allErrorsMu.Lock()
	wp.allErrors = &errors.MultiError{}
	wp.allErrorsMu.Unlock()

	wp.mu.Unlock()
}

// appendError safely appends an error to allErrors
func (wp *Pool) appendError(err error) {
	if err == nil {
		return
	}

	wp.allErrorsMu.Lock()
	wp.allErrors = wp.allErrors.Append(err)
	wp.allErrorsMu.Unlock()
}

// Submit adds a new task and starts a goroutine to execute it when a worker is available
func (wp *Pool) Submit(task Task) {
	wp.mu.RLock()
	notRunning := !wp.isRunning
	wp.mu.RUnlock()

	if notRunning {
		wp.Start()
	}

	// Don't submit new tasks if the pool is stopping
	if wp.isStopping.Load() {
		return
	}

	wp.wg.Add(1)

	// Start a new goroutine for each task, but limit concurrency with semaphore
	go func() {
		defer wp.wg.Done()

		wp.semaphore <- struct{}{}

		defer func() { <-wp.semaphore }()

		err := task()
		if err != nil {
			wp.appendError(err)
		}
	}()
}

// Wait blocks until all tasks are completed and returns any errors
func (wp *Pool) Wait() error {
	// Wait for all tasks to complete
	wp.wg.Wait()

	// Get all collected errors
	wp.allErrorsMu.RLock()
	result := wp.allErrors.ErrorOrNil()
	wp.allErrorsMu.RUnlock()

	return result
}

// Stop shuts down the worker pool after current tasks are completed
func (wp *Pool) Stop() {
	wp.mu.Lock()
	defer wp.mu.Unlock()

	if wp.isRunning {
		// Mark as stopping to prevent new task submissions
		wp.isStopping.Store(true)

		go func() {
			wp.wg.Wait()

			wp.mu.Lock()
			wp.isRunning = false
			wp.mu.Unlock()
		}()
	}
}

// GracefulStop waits for all tasks to complete before stopping the pool
func (wp *Pool) GracefulStop() error {
	// Mark as stopping to prevent new task submissions
	wp.isStopping.Store(true)

	// Wait for all tasks to complete and capture any errors
	err := wp.Wait()

	// Now fully stop the pool
	wp.mu.Lock()
	defer wp.mu.Unlock()

	if wp.isRunning {
		wp.isRunning = false
	}

	return err
}

// IsRunning returns whether the pool is currently running
func (wp *Pool) IsRunning() bool {
	wp.mu.RLock()
	defer wp.mu.RUnlock()

	return wp.isRunning
}

// IsStopping returns whether the pool is in the process of stopping
func (wp *Pool) IsStopping() bool {
	return wp.isStopping.Load()
}
