package worker_test

import (
	"errors"
	"sync/atomic"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/panicreport"
	"github.com/gruntwork-io/terragrunt/internal/worker"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAllTasksCompleteWithoutErrors(t *testing.T) {
	t.Parallel()

	wp := worker.NewWorkerPool(5)
	defer wp.Stop()

	var counter atomic.Int32

	// Submit 10 tasks that increment a counter
	for range 10 {
		wp.Submit(func() error {
			counter.Add(1)
			return nil
		})
	}

	// Wait for all tasks to complete
	errs := wp.Wait()
	require.NoError(t, errs)

	assert.Equal(t, int32(10), counter.Load())
}

func TestSubmitLessAllTasksCompleteWithoutErrors(t *testing.T) {
	t.Parallel()

	wp := worker.NewWorkerPool(10)
	defer wp.Stop()

	var counter atomic.Int32

	for range 5 {
		wp.Submit(func() error {
			counter.Add(1)
			return nil
		})
	}

	// Wait for all tasks to complete
	errs := wp.Wait()
	require.NoError(t, errs)

	assert.Equal(t, int32(5), counter.Load())
}

func TestSomeTasksReturnErrors(t *testing.T) {
	t.Parallel()

	wp := worker.NewWorkerPool(3)
	defer wp.Stop()

	var successCount atomic.Int32

	// Submit tasks, half of which return an error
	for i := range 10 {
		wp.Submit(func() error {
			if i%2 == 0 {
				return errors.New("mock error")
			}

			successCount.Add(1)

			return nil
		})
	}

	errs := wp.Wait()
	require.Error(t, errs)

	unwrapper, ok := errs.(interface{ Unwrap() []error })
	require.True(t, ok, "expected joined error, got %T", errs)
	require.Len(
		t,
		unwrapper.Unwrap(),
		5,
		"expected exactly 5 errors, got %d",
		len(unwrapper.Unwrap()),
	)

	assert.Equal(t, int32(5), successCount.Load())
}

func TestStopAndRestart(t *testing.T) {
	t.Parallel()

	wp := worker.NewWorkerPool(2)

	var counter atomic.Int32

	// Submit some tasks
	for range 5 {
		wp.Submit(func() error {
			counter.Add(1)
			return nil
		})
	}

	// Wait for all tasks to complete and stop the pool
	err := wp.Wait()
	require.NoError(t, err)
	wp.Stop()

	finalCount := counter.Load()
	require.Equal(t, int32(5), finalCount, "expected counter to be 5")

	// Create a new worker pool instead of assuming restart
	wp = worker.NewWorkerPool(2)
	defer wp.Stop()

	// Submit new tasks
	for range 3 {
		wp.Submit(func() error {
			counter.Add(1)
			return nil
		})
	}

	errs := wp.Wait()
	require.NoError(t, errs)

	finalCountAfterRestart := counter.Load()
	require.Equal(t, int32(8), finalCountAfterRestart, "expected counter to be 8")
}

func TestPanicTaskReturnsReportableError(t *testing.T) {
	t.Parallel()

	wp := worker.NewWorkerPool(1)
	defer wp.Stop()

	wp.Submit(func() error {
		panic("worker panic")
	})

	err := wp.Wait()
	require.Error(t, err)
	assert.True(t, panicreport.IsPanic(err), "returned error should be classified as a panic")

	msg, stack := panicreport.PanicDetails(err)
	assert.Equal(t, "worker panic", msg)
	assert.Contains(t, string(stack), "worker_test.go")
}

func TestParallelSubmitsAndWaits(t *testing.T) {
	t.Parallel()

	wp := worker.NewWorkerPool(4)

	t.Cleanup(func() { wp.Stop() })

	var totalCount atomic.Int32

	t.Run("parallelTaskSubmit1", func(t *testing.T) {
		t.Parallel()

		localWp := worker.NewWorkerPool(4) // Create a new worker pool per subtest
		defer localWp.Stop()

		for range 10 {
			localWp.Submit(func() error {
				totalCount.Add(1)
				return nil
			})
		}

		err := localWp.Wait()
		require.NoError(t, err)
	})

	t.Run("parallelTaskSubmit2", func(t *testing.T) {
		t.Parallel()

		localWp := worker.NewWorkerPool(4) // Create another fresh worker pool
		defer localWp.Stop()

		for range 15 {
			localWp.Submit(func() error {
				totalCount.Add(1)
				return nil
			})
		}

		err := localWp.Wait()
		require.NoError(t, err)
	})
}

func TestValidateParallelSubmits(t *testing.T) {
	t.Parallel()

	wp := worker.NewWorkerPool(1)
	defer wp.Stop()

	var totalCount atomic.Int32

	// Submit 5 tasks
	for range 5 {
		wp.Submit(func() error {
			totalCount.Add(1)
			return nil
		})
	}

	errs := wp.Wait()
	require.NoError(t, errs)

	assert.Equal(t, int32(5), totalCount.Load())
}
