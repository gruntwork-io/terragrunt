package util

import (
	"sync"

	"github.com/gruntwork-io/terragrunt/internal/errors"
)

// Task represents a unit of work that can be executed
type Task func() error

// WorkerPool manages concurrent task execution
type WorkerPool struct {
	resultChan  chan error
	wg          sync.WaitGroup
	errorsSlice []error
	mu          sync.Mutex // Mutex to protect errorsSlice
	isRunning   bool
}

// NewWorkerPool creates a new worker pool
func NewWorkerPool() *WorkerPool {
	return &WorkerPool{
		resultChan:  make(chan error),
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

	// Recreate the result channel if it's been closed
	wp.resultChan = make(chan error)

	wp.mu.Unlock()

	// Start the error collector
	go wp.collectResults()
}

// collectResults collects the errors from the result channel
func (wp *WorkerPool) collectResults() {
	for err := range wp.resultChan {
		if err != nil {
			wp.mu.Lock()
			wp.errorsSlice = append(wp.errorsSlice, err)
			wp.mu.Unlock()
		}
	}
}

// Submit adds a new task and immediately starts a goroutine to execute it
func (wp *WorkerPool) Submit(task Task) {
	if !wp.isRunning {
		wp.Start()
	}
	wp.wg.Add(1)

	// Start a new goroutine for each task
	go func() {
		err := task()
		wp.resultChan <- err
		wp.wg.Done()
	}()
}

// Wait blocks until all tasks are completed
func (wp *WorkerPool) Wait() error {
	wp.wg.Wait()

	wp.mu.Lock()
	defer wp.mu.Unlock()

	var errors *errors.MultiError

	for _, err := range wp.errorsSlice {
		if err == nil {
			continue
		}
		errors = errors.Append(errors, err)
	}

	return errors.ErrorOrNil()
}

// Stop shuts down the worker pool after current tasks are completed
func (wp *WorkerPool) Stop() {
	wp.mu.Lock()
	defer wp.mu.Unlock()

	if wp.isRunning {
		close(wp.resultChan)
		wp.isRunning = false
	}
}
