package worker

import (
	"sync"
	"sync/atomic"

	"github.com/gruntwork-io/terragrunt/internal/errors"
)

// Task represents a unit of work that can be executed
type Task func() error

// WorkerPool manages concurrent task execution with a configurable number of workers
type WorkerPool struct {
	semaphore   chan struct{}
	resultChan  chan error
	doneChan    chan struct{}
	errorsSlice []error
	wg          sync.WaitGroup
	maxWorkers  int
	mu          sync.Mutex
	resultMu    sync.Mutex // Additional mutex for error collection
	isStopping  atomic.Bool
	isRunning   bool
}

// NewWorkerPool creates a new worker pool with the specified maximum number of concurrent workers
func NewWorkerPool(maxWorkers int) *WorkerPool {
	if maxWorkers <= 0 {
		maxWorkers = 1
	}

	return &WorkerPool{
		maxWorkers:  maxWorkers,
		semaphore:   make(chan struct{}, maxWorkers),
		resultChan:  make(chan error),
		doneChan:    make(chan struct{}),
		isRunning:   false,
		errorsSlice: make([]error, 0),
	}
}

// Start initializes the worker pool
func (wp *WorkerPool) Start() {
	wp.mu.Lock()
	if wp.isRunning {
		wp.mu.Unlock()
		return
	}

	wp.isRunning = true
	wp.isStopping.Store(false)

	// Recreate the channels if they've been closed
	wp.resultChan = make(chan error)
	wp.doneChan = make(chan struct{})
	wp.semaphore = make(chan struct{}, wp.maxWorkers)

	// Clear previous errors
	wp.errorsSlice = make([]error, 0)

	wp.mu.Unlock()

	// Start the error collector
	go wp.collectResults()
}

// collectResults collects the errors from the result channel
func (wp *WorkerPool) collectResults() {
	for {
		select {
		case err, ok := <-wp.resultChan:
			if !ok {
				return
			}

			if err != nil {
				wp.resultMu.Lock()
				wp.errorsSlice = append(wp.errorsSlice, err)
				wp.resultMu.Unlock()
			}
		case <-wp.doneChan:
			return
		}
	}
}

// Submit adds a new task and starts a goroutine to execute it when a worker is available
func (wp *WorkerPool) Submit(task Task) {
	wp.mu.Lock()
	if !wp.isRunning {
		wp.mu.Unlock()
		wp.Start()
	} else {
		wp.mu.Unlock()
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

		// Only send result if the pool is not stopping
		if !wp.isStopping.Load() {
			select {
			case <-wp.doneChan:
				// Pool is stopping, track the error directly
				if err != nil {
					wp.resultMu.Lock()
					wp.errorsSlice = append(wp.errorsSlice, err)
					wp.resultMu.Unlock()
				}
			case wp.resultChan <- err:
				// Successfully sent the error
			}
		} else if err != nil {
			// Pool is stopping but we still want to track the error
			wp.resultMu.Lock()
			wp.errorsSlice = append(wp.errorsSlice, err)
			wp.resultMu.Unlock()
		}
	}()
}

// Wait blocks until all tasks are completed
func (wp *WorkerPool) Wait() error {
	// Wait for all tasks to complete
	wp.wg.Wait()

	// Collect all errors
	wp.resultMu.Lock()
	defer wp.resultMu.Unlock()

	errs := &errors.MultiError{}

	for _, err := range wp.errorsSlice {
		if err == nil {
			continue
		}

		errs = errs.Append(err)
	}

	return errs.ErrorOrNil()
}

// Stop shuts down the worker pool after current tasks are completed
func (wp *WorkerPool) Stop() {
	wp.mu.Lock()
	defer wp.mu.Unlock()

	if wp.isRunning {
		// Mark as stopping to prevent writes to resultChan
		wp.isStopping.Store(true)

		close(wp.doneChan)
		close(wp.resultChan)
		wp.isRunning = false
	}
}

// GracefulStop waits for all tasks to complete before stopping the pool
func (wp *WorkerPool) GracefulStop() error {
	// Wait for all tasks to complete
	err := wp.Wait()

	// Then stop the pool
	wp.Stop()

	return err
}

// GetMaxWorkers returns the current maximum number of concurrent workers
func (wp *WorkerPool) GetMaxWorkers() int {
	wp.mu.Lock()
	defer wp.mu.Unlock()

	return wp.maxWorkers
}
