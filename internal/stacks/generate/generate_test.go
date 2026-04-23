package generate_test

import (
	"context"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gruntwork-io/terragrunt/internal/stacks/generate"
	"github.com/gruntwork-io/terragrunt/pkg/options"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/stretchr/testify/require"
	"golang.org/x/sync/errgroup"
)

func TestStackGeneratorSerializesConcurrentSameWorkingDir(t *testing.T) {
	t.Parallel()

	const numCalls = 8

	var (
		active    atomic.Int32
		maxActive atomic.Int32
	)

	g := generate.NewStackGenerator()
	workingDir := createStackFixture(t)

	g.RegisterOnGenerateHook(workingDir, func(string) {
		currentActive := active.Add(1)
		defer active.Add(-1)

		for {
			observedMax := maxActive.Load()
			if currentActive <= observedMax || maxActive.CompareAndSwap(observedMax, currentActive) {
				break
			}
		}

		time.Sleep(10 * time.Millisecond)
	})
	t.Cleanup(func() { g.UnregisterOnGenerateHook(workingDir) })

	var eg errgroup.Group

	for range numCalls {
		eg.Go(func() error {
			return g.GenerateStacks(
				context.Background(),
				logger.CreateLogger(),
				terragruntOptions(workingDir),
				nil,
			)
		})
	}

	require.NoError(t, eg.Wait())
	require.EqualValues(t, 1, maxActive.Load(), "same working directory must be serialized")
}

func TestStackGeneratorAllowsDifferentWorkingDirsConcurrently(t *testing.T) {
	t.Parallel()

	g := generate.NewStackGenerator()
	workingDir1 := createStackFixture(t)
	workingDir2 := createStackFixture(t)

	firstEntered := make(chan struct{})
	secondEntered := make(chan struct{})
	releaseFirst := make(chan struct{})
	releaseSecond := make(chan struct{})

	g.RegisterOnGenerateHook(workingDir1, func(string) {
		close(firstEntered)
		<-releaseFirst
	})
	t.Cleanup(func() { g.UnregisterOnGenerateHook(workingDir1) })

	g.RegisterOnGenerateHook(workingDir2, func(string) {
		close(secondEntered)
		<-releaseSecond
	})
	t.Cleanup(func() { g.UnregisterOnGenerateHook(workingDir2) })

	errCh1 := make(chan error, 1)

	go func() {
		errCh1 <- g.GenerateStacks(context.Background(), logger.CreateLogger(), terragruntOptions(workingDir1), nil)
	}()

	select {
	case <-firstEntered:
	case <-time.After(2 * time.Second):
		t.Fatal("first working directory never reached generation hook")
	}

	errCh2 := make(chan error, 1)

	go func() {
		errCh2 <- g.GenerateStacks(context.Background(), logger.CreateLogger(), terragruntOptions(workingDir2), nil)
	}()

	select {
	case <-secondEntered:
	case <-time.After(2 * time.Second):
		t.Fatal("independent working directory blocked behind another key")
	}

	close(releaseFirst)
	close(releaseSecond)

	require.NoError(t, <-errCh1)
	require.NoError(t, <-errCh2)
}

func TestPackageLevelGenerateStacksUsesDefaultGeneratorHook(t *testing.T) {
	t.Parallel()

	workingDir := createStackFixture(t)

	var dispatched atomic.Int32

	generate.RegisterOnGenerateHook(workingDir, func(string) {
		dispatched.Add(1)
	})
	t.Cleanup(func() { generate.UnregisterOnGenerateHook(workingDir) })

	require.NoError(t, generate.GenerateStacks(
		context.Background(),
		logger.CreateLogger(),
		terragruntOptions(workingDir),
		nil,
	))
	require.EqualValues(t, 1, dispatched.Load())
}

func createStackFixture(t *testing.T) string {
	t.Helper()

	workingDir := t.TempDir()
	unitDir := filepath.Join(workingDir, "unit")

	require.NoError(t, os.MkdirAll(unitDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(unitDir, "terragrunt.hcl"), []byte(`terraform {
  source = "."
}
`), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(unitDir, "main.tf"), []byte(""), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(workingDir, "terragrunt.stack.hcl"), []byte(`unit "app" {
  source = "./unit"
  path   = "app"
}
`), 0o644))

	return workingDir
}

func terragruntOptions(workingDir string) *options.TerragruntOptions {
	return &options.TerragruntOptions{
		WorkingDir:  workingDir,
		Parallelism: 1,
	}
}
