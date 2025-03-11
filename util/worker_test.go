package util_test

import (
	"sync/atomic"
	"testing"
	"time"

	"github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/gruntwork-io/terragrunt/util"
	"github.com/stretchr/testify/require"
)

func TestAllTasksCompleteWithoutErrors(t *testing.T) {
	t.Parallel()

	wp := util.NewWorkerPool(5)
	defer wp.Stop()

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
	require.NoError(t, errs)

	if atomic.LoadInt32(&counter) != 10 {
		t.Errorf("expected counter to be 10, got %d", counter)
	}
}

func TestSubmitLessAllTasksCompleteWithoutErrors(t *testing.T) {
	t.Parallel()

	wp := util.NewWorkerPool(10)
	defer wp.Stop()

	var counter int32
	for i := 0; i < 5; i++ {
		wp.Submit(func() error {
			atomic.AddInt32(&counter, 1)
			return nil
		})
	}

	// Wait for all tasks to complete
	errs := wp.Wait()
	require.NoError(t, errs)

	if atomic.LoadInt32(&counter) != 5 {
		t.Errorf("expected counter to be 5, got %d", counter)
	}
}

func TestSomeTasksReturnErrors(t *testing.T) {
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
	require.Error(t, errs)

	if atomic.LoadInt32(&successCount) != 5 {
		t.Errorf("expected successCount to be 5, got %d", successCount)
	}
}

func TestStopAndRestart(t *testing.T) {
	t.Parallel()

	wp := util.NewWorkerPool(2)

	var counter int32

	// Submit some tasks
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
	require.NoError(t, errs)

	finalCountAfterRestart := atomic.LoadInt32(&counter)
	if finalCountAfterRestart != 8 {
		t.Errorf("expected counter to be 8, got %d", finalCountAfterRestart)
	}
}

func TestParallelSubmitsAndWaits(t *testing.T) {
	t.Parallel()

	wp := util.NewWorkerPool(4)
	t.Cleanup(func() {
		wp.Stop()
	})

	var totalCount int32

	// Two parallel sub-tests that submit tasks to the same worker pool
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

}

func TestValidateParallelSubmits(t *testing.T) {
	t.Parallel()
	wp := util.NewWorkerPool(1)
	defer wp.Stop()

	var totalCount int32

	// Submit 5 tasks
	for i := 0; i < 5; i++ {
		wp.Submit(func() error {
			atomic.AddInt32(&totalCount, 1)
			return nil
		})
	}

	errs := wp.Wait()
	require.NoError(t, errs)

	if atomic.LoadInt32(&totalCount) != 5 {
		t.Errorf("expected totalCount to be 5, got %d", totalCount)
	}
}
