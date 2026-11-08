package generate

import (
	"errors"
	"testing"
	"testing/synctest"

	"github.com/gruntwork-io/terragrunt/internal/worker"
	"github.com/stretchr/testify/require"
)

func TestStackTasksWaitsForQueuedWork(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		pool := worker.NewWorkerPool(1)
		defer pool.Stop()
		tasks := &stackTasks{pool: pool}
		release := make(chan struct{})
		done := make(chan error, 1)
		failure := errors.New("copy failed")
		tasks.Submit(func() error { <-release; return failure })
		go func() { done <- tasks.Wait() }()
		synctest.Wait()
		select {
		case <-done:
			t.Fatal("published a result before the copy finished")
		default:
		}
		close(release)
		require.ErrorIs(t, <-done, failure)
		require.NoError(t, pool.Wait())
	})
}

func TestStackTasksKeepsFailuresWithTheirStack(t *testing.T) {
	pool := worker.NewWorkerPool(1)
	defer pool.Stop()
	failed, successful := &stackTasks{pool: pool}, &stackTasks{pool: pool}
	failed.Submit(func() error { panic("copy panicked") })
	successful.Submit(func() error { return nil })
	require.ErrorContains(t, failed.Wait(), "copy panicked")
	require.NoError(t, successful.Wait())
	require.NoError(t, pool.Wait())
}
