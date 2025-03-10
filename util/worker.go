package util

import (
	"sync"

	"github.com/gruntwork-io/terragrunt/internal/errors"
)

// Task represents a unit of work that can be executed by the worker pool
type Task func() error

// WorkerPool manages a pool of workers for concurrent task execution
type WorkerPool struct {
	numWorkers  int
	taskChan    chan Task
	resultChan  chan error
	wg          sync.WaitGroup
	isRunning   bool
	errorsSlice []error
	mu          sync.Mutex // Mutex to protect errorsSlice
}

// NewWorkerPool creates a new worker pool with the specified number of workers
func NewWorkerPool(numWorkers int) *WorkerPool {
	if numWorkers <= 0 {
		numWorkers = 1 // Ensure at least one worker
	}

	return &WorkerPool{
		numWorkers:  numWorkers,
		taskChan:    make(chan Task),
		resultChan:  make(chan error),
		isRunning:   false,
		errorsSlice: make([]error, 0),
	}
}

// Start initializes and starts the worker pool
func (wp *WorkerPool) Start() {
	wp.mu.Lock()
	if wp.isRunning {
		wp.mu.Unlock()
		return
	}
	wp.isRunning = true

	// Recreate the channels if they've been closed
	wp.taskChan = make(chan Task)
	wp.resultChan = make(chan error)

	wp.mu.Unlock()

	// Start the error collector
	go wp.collectResults()

	// Start the workers
	for i := 0; i < wp.numWorkers; i++ {
		go wp.worker()
	}
}

// worker is the goroutine that processes tasks
func (wp *WorkerPool) worker() {
	for task := range wp.taskChan {
		err := task()
		wp.resultChan <- err
		wp.wg.Done()
	}
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

// Submit adds a new task to the worker pool
func (wp *WorkerPool) Submit(task Task) {
	if !wp.isRunning {
		wp.Start()
	}
	wp.wg.Add(1)
	wp.taskChan <- task
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
		close(wp.taskChan)
		close(wp.resultChan)
		wp.isRunning = false
	}
}
