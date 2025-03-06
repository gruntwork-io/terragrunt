package util

import (
	"sync"

	"github.com/gruntwork-io/terragrunt/internal/errors"
)

// WorkerPool manages a pool of workers for parallel execution.
type WorkerPool struct {
	workers   int
	taskQueue chan func() error
	waitGroup sync.WaitGroup
	errors    []error
	mutex     sync.Mutex
}

// NewWorkerPool creates a new worker pool with the specified number of workers.
func NewWorkerPool(workers int) *WorkerPool {
	return &WorkerPool{
		workers:   workers,
		taskQueue: make(chan func() error, workers),
		errors:    make([]error, 0),
	}
}

// Start initializes the worker pool and begins processing tasks.
func (wp *WorkerPool) Start() {
	for i := 0; i < wp.workers; i++ {
		wp.waitGroup.Add(1)
		go func() {
			defer wp.waitGroup.Done()
			for task := range wp.taskQueue {
				if err := task(); err != nil {
					wp.mutex.Lock()
					wp.errors = append(wp.errors, err)
					wp.mutex.Unlock()
				}
			}
		}()
	}
}

// Submit enqueues a task for execution by the worker pool.
func (wp *WorkerPool) Submit(task func() error) {
	wp.taskQueue <- task
}

// Wait waits for all workers to complete execution.
func (wp *WorkerPool) Wait() {
	close(wp.taskQueue)
	wp.waitGroup.Wait()
}

// Errors returns any errors encountered during execution.
func (wp *WorkerPool) Errors() error {
	wp.mutex.Lock()
	defer wp.mutex.Unlock()
	poolError := &errors.MultiError{}
	for _, err := range wp.errors {
		poolError = poolError.Append(err)
	}
	return poolError.ErrorOrNil()
}
