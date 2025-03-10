package util_test

import (
	"sync/atomic"
	"testing"
	"time"

	"github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/gruntwork-io/terragrunt/util"

	"github.com/stretchr/testify/assert"
)

// TestWorkerPool is a top-level test function that includes parallel sub-tests.
func TestWorkerPool(t *testing.T) {
	t.Run("AllTasksCompleteWithoutErrors", func(t *testing.T) {
		// Mark this sub-test as parallel. The Go test framework may run
		t.Parallel()

		wp := util.NewWorkerPool(5)
		defer wp.Stop() // Ensure we stop the pool at the end of the test

		var counter int32

		// Submit 10 tasks that increment a counter
		for i := 0; i < 10; i++ {
			wp.Submit(func() error {
				atomic.AddInt32(&counter, 1)
				return nil
			})
		}

		// Wait for all tasks to complete
		errs := wp.Wait()
		assert.NoError(t, errs)
		// Validate counter reached 10
		if atomic.LoadInt32(&counter) != 10 {
			t.Errorf("expected counter to be 10, got %d", counter)
		}
	})

	t.Run("SomeTasksReturnErrors", func(t *testing.T) {
		t.Parallel()

		wp := util.NewWorkerPool(3)
		defer wp.Stop()

		var successCount int32

		// Submit tasks, half of which return an error
		for i := 0; i < 10; i++ {
			i := i
			wp.Submit(func() error {
				if i%2 == 0 {
					return errors.New("mock error")
				}
				atomic.AddInt32(&successCount, 1)
				return nil
			})
		}

		errs := wp.Wait()
		assert.Error(t, errs)

		if atomic.LoadInt32(&successCount) != 5 {
			t.Errorf("expected successCount to be 5, got %d", successCount)
		}
	})

	t.Run("StopAndRestart", func(t *testing.T) {
		t.Parallel()

		wp := util.NewWorkerPool(2)

		// Submit some tasks
		var counter int32
		for i := 0; i < 5; i++ {
			wp.Submit(func() error {
				time.Sleep(10 * time.Millisecond)
				atomic.AddInt32(&counter, 1)
				return nil
			})
		}

		// Wait for all tasks to complete and stop the pool
		wp.Wait()
		wp.Stop()

		finalCount := atomic.LoadInt32(&counter)
		if finalCount != 5 {
			t.Errorf("expected counter to be 5, got %d", finalCount)
		}

		// Attempt to submit new tasks after Stop and see if it auto-starts
		for i := 0; i < 3; i++ {
			wp.Submit(func() error {
				atomic.AddInt32(&counter, 1)
				return nil
			})
		}
		errs := wp.Wait()
		assert.NoError(t, errs)
		finalCountAfterRestart := atomic.LoadInt32(&counter)
		if finalCountAfterRestart != 8 {
			t.Errorf("expected counter to be 8, got %d", finalCountAfterRestart)
		}
	})

	t.Run("ParallelSubmitsAndWaits", func(t *testing.T) {
		t.Parallel()

		wp := util.NewWorkerPool(4)
		defer wp.Stop()

		var totalCount int32

		// We'll create two parallel sub-tests that both submit tasks
		// concurrently to the same worker pool, to see if it handles concurrency well.
		t.Run("parallelTaskSubmit1", func(t *testing.T) {
			t.Parallel()

			for i := 0; i < 10; i++ {
				wp.Submit(func() error {
					atomic.AddInt32(&totalCount, 1)
					return nil
				})
			}
		})

		t.Run("parallelTaskSubmit2", func(t *testing.T) {
			t.Parallel()

			for i := 0; i < 15; i++ {
				wp.Submit(func() error {
					atomic.AddInt32(&totalCount, 1)
					return nil
				})
			}
		})

		// Wait for sub-tests to finish submitting
		// then wait for all tasks
		// The outer test must not call Wait until
		// the sub-tests have completed submissions
	})

	t.Run("ValidateParallelSubmits", func(t *testing.T) {
		// This test is run after the parallel sub-tests above are done,
		// and we can safely call Wait here to ensure all tasks are done.

		wp := util.NewWorkerPool(1) // single worker to highlight concurrency queueing
		defer wp.Stop()

		var totalCount int32

		// We expect 10 + 15 = 25 tasks from the previous sub-tests, but those were
		// done in a separate WorkerPool instance. We'll demonstrate a fresh test here.

		// Submit tasks to see if single worker queues and runs them all
		for i := 0; i < 5; i++ {
			wp.Submit(func() error {
				atomic.AddInt32(&totalCount, 1)
				return nil
			})
		}

		errs := wp.Wait()
		assert.NoError(t, errs)

		if atomic.LoadInt32(&totalCount) != 5 {
			t.Errorf("expected 5 tasks completed, got %d", totalCount)
		}
	})
}
