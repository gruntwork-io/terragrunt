package generate_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	rewrite "github.com/gruntwork-io/terragrunt/internal/config/rewrite"
	"github.com/gruntwork-io/terragrunt/internal/experiment"
	"github.com/gruntwork-io/terragrunt/internal/filter"
	"github.com/gruntwork-io/terragrunt/internal/stacks/generate"
	"github.com/gruntwork-io/terragrunt/internal/vexec"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/gruntwork-io/terragrunt/pkg/options"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func coordinatorOptions(t *testing.T) (*options.TerragruntOptions, string) {
	t.Helper()

	root := t.TempDir()
	for _, dir := range []string{"consumer", "producer", "sibling"} {
		require.NoError(t, os.MkdirAll(filepath.Join(root, dir, ".terragrunt-stack"), 0755))
		require.NoError(t, os.WriteFile(filepath.Join(root, dir, "terragrunt.stack.hcl"), nil, 0600))
		require.NoError(t, os.WriteFile(filepath.Join(root, dir, ".terragrunt-stack", "old"), []byte("old"), 0600))
	}

	opts := options.NewTerragruntOptions(vexec.NewOSExec())
	opts.WorkingDir = filepath.Join(root, "consumer")
	opts.SourceUpdate = true
	require.NoError(t, opts.Experiments.EnableExperiment(experiment.DependencyStackGeneration))

	return opts, filepath.Join(root, "producer", "terragrunt.stack.hcl")
}

func TestCoordinatorChecksEligibilityBeforeCleaning(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name, boundary, query              string
		disabled, optOut, allowed, invalid bool
	}{
		{name: "enabled", allowed: true},
		{name: "experiment disabled", disabled: true},
		{name: "generation disabled", optOut: true},
		{name: "outside relative limit", boundary: "."},
		{name: "inside relative limit", boundary: "..", allowed: true},
		{name: "outside absolute limit", boundary: "{consumer}"},
		{name: "inline limit overrides flag", boundary: ".", query: "(..)...consumer", allowed: true},
		{name: "invalid limit", boundary: "./absent", invalid: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			opts, path := coordinatorOptions(t)

			opts.NoStackGenerate = tc.optOut
			if tc.disabled {
				opts.Experiments = experiment.NewExperiments()
			}

			opts.DiscoveryBoundary = tc.boundary
			if tc.boundary == "{consumer}" {
				opts.DiscoveryBoundary = opts.WorkingDir
			}

			if tc.query != "" {
				var err error

				opts.Filters, err = filter.ParseFilterQueries(logger.CreateLogger(), []string{tc.query})
				require.NoError(t, err)
			}

			c := generate.NewCoordinator(vfs.NewOSFS(), new(rewrite.Store), opts)
			calls := 0

			err := c.Generate(t.Context(), path, func(context.Context) error {
				calls++

				assert.NoFileExists(t, filepath.Join(filepath.Dir(path), ".terragrunt-stack", "old"))

				return nil
			})
			if tc.invalid {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}

			if tc.allowed {
				assert.Equal(t, 1, calls)
			} else {
				assert.Zero(t, calls)
				assert.FileExists(t, filepath.Join(filepath.Dir(path), ".terragrunt-stack", "old"))
			}

			assert.FileExists(t, filepath.Join(filepath.Dir(opts.WorkingDir), "sibling", ".terragrunt-stack", "old"))
		})
	}
}

func TestCoordinatorReusesGenerationWithoutCleaningAgain(t *testing.T) {
	t.Parallel()
	opts, path := coordinatorOptions(t)
	c := generate.NewCoordinator(vfs.NewOSFS(), new(rewrite.Store), opts)
	marker := filepath.Join(filepath.Dir(path), ".terragrunt-stack", "new")
	calls := 0
	work := func(context.Context) error {
		calls++

		require.NoError(t, os.MkdirAll(filepath.Dir(marker), 0755))

		return os.WriteFile(marker, []byte("new"), 0600)
	}
	require.NoError(t, c.Generate(t.Context(), path, work))
	relative, err := filepath.Rel(opts.WorkingDir, path)
	require.NoError(t, err)
	require.NoError(t, c.Generate(t.Context(), relative, work))
	assert.Equal(t, 1, calls)
	assert.FileExists(t, marker)

	fresh := generate.NewCoordinator(vfs.NewOSFS(), new(rewrite.Store), opts)
	require.NoError(t, fresh.Generate(t.Context(), path, work))
	assert.Equal(t, 2, calls, "a new command must regenerate")
}

func TestCoordinatorSharesConcurrentResults(t *testing.T) {
	t.Parallel()

	for _, failed := range []bool{false, true} {
		name := "success"
		if failed {
			name = "failure"
		}

		t.Run(name, func(t *testing.T) {
			t.Parallel()
			opts, path := coordinatorOptions(t)
			c := generate.NewCoordinator(vfs.NewOSFS(), new(rewrite.Store), opts)

			var (
				calls atomic.Int32
				want  error
			)
			if failed {
				want = errors.New("generation failed")
			}

			start := make(chan struct{})

			var wg sync.WaitGroup
			for range 12 {
				wg.Go(func() {
					<-start

					err := c.Generate(t.Context(), path, func(context.Context) error {
						calls.Add(1)
						return want
					})
					assert.ErrorIs(t, err, want)
				})
			}

			close(start)
			wg.Wait()
			assert.EqualValues(t, 1, calls.Load())
		})
	}
}

func TestCoordinatorDetectsRecursiveCycle(t *testing.T) {
	t.Parallel()
	opts, a := coordinatorOptions(t)
	b := filepath.Join(opts.WorkingDir, "terragrunt.stack.hcl")
	c := generate.NewCoordinator(vfs.NewOSFS(), new(rewrite.Store), opts)

	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()

	err := c.Generate(ctx, a, func(ctx context.Context) error {
		return c.Generate(ctx, b, func(ctx context.Context) error {
			return c.Generate(ctx, a, func(context.Context) error { assert.Fail(t, "cycle ran twice"); return nil })
		})
	})
	require.ErrorContains(t, err, "stack dependency cycle")
	assert.Contains(t, err.Error(), a)
	assert.Contains(t, err.Error(), b)
	assert.NoError(t, ctx.Err())
}

func TestCoordinatorDetectsCycleBetweenConcurrentRoots(t *testing.T) {
	t.Parallel()
	opts, a := coordinatorOptions(t)
	b := filepath.Join(opts.WorkingDir, "terragrunt.stack.hcl")
	c := generate.NewCoordinator(vfs.NewOSFS(), new(rewrite.Store), opts)

	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()

	var started sync.WaitGroup
	started.Add(2)

	results := make(chan error, 2)

	for _, pair := range [][2]string{{a, b}, {b, a}} {
		go func() {
			results <- c.Generate(ctx, pair[0], func(ctx context.Context) error {
				started.Done()
				started.Wait()

				return c.Generate(ctx, pair[1], func(context.Context) error { assert.Fail(t, "cycle ran twice"); return nil })
			})
		}()
	}

	for range 2 {
		require.ErrorContains(t, <-results, "stack dependency cycle")
	}

	assert.NoError(t, ctx.Err())
}

func TestCoordinatorCancelledWaiterDoesNotCancelGeneration(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		opts, path := coordinatorOptions(t)
		c := generate.NewCoordinator(vfs.NewOSFS(), new(rewrite.Store), opts)
		started, release := make(chan struct{}), make(chan struct{})

		done := make(chan error, 1)
		go func() {
			done <- c.Generate(t.Context(), path, func(context.Context) error {
				close(started)
				<-release

				return nil
			})
		}()

		<-started

		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()

		waiter := make(chan error, 1)
		go func() {
			waiter <- c.Generate(ctx, path, func(context.Context) error { assert.Fail(t, "duplicate generation"); return nil })
		}()

		synctest.Wait()
		cancel()
		require.ErrorIs(t, <-waiter, context.Canceled)

		select {
		case <-done:
			require.FailNow(t, "waiter cancelled the owner")
		default:
		}

		close(release)
		require.NoError(t, <-done)
		require.NoError(
			t,
			c.Generate(t.Context(), path, func(context.Context) error { assert.Fail(t, "duplicate generation"); return nil }),
		)
	})
}

func TestCoordinatorSharesPanicsAsErrors(t *testing.T) {
	t.Parallel()
	opts, path := coordinatorOptions(t)
	c := generate.NewCoordinator(vfs.NewOSFS(), new(rewrite.Store), opts)
	err := c.Generate(t.Context(), path, func(context.Context) error { panic("generation panic") })
	require.ErrorContains(t, err, "generation panic")
	again := c.Generate(
		t.Context(),
		path,
		func(context.Context) error { assert.Fail(t, "retried failed generation"); return nil },
	)
	assert.Equal(t, err, again)
}

func TestCoordinatorPreservesFilesWhenCleanupIsDisabled(t *testing.T) {
	t.Parallel()

	for _, command := range []string{"plan", "destroy"} {
		t.Run(command, func(t *testing.T) {
			t.Parallel()
			opts, path := coordinatorOptions(t)
			opts.TofuCommand = command
			opts.SourceUpdate = command == "destroy"
			c := generate.NewCoordinator(vfs.NewOSFS(), new(rewrite.Store), opts)
			calls := 0

			require.NoError(t, c.Generate(t.Context(), path, func(context.Context) error {
				calls++

				assert.FileExists(t, filepath.Join(filepath.Dir(path), ".terragrunt-stack", "old"))

				return nil
			}))
			assert.Equal(t, 1, calls)
		})
	}
}

func TestCoordinatorRetainsOriginalDiscoveryDirectory(t *testing.T) {
	t.Parallel()
	opts, path := coordinatorOptions(t)
	opts.DiscoveryBoundary = "."
	c := generate.NewCoordinator(vfs.NewOSFS(), new(rewrite.Store), opts)
	opts.WorkingDir = filepath.Dir(path)
	require.NoError(t, c.Generate(t.Context(), path, func(context.Context) error {
		assert.Fail(t, "dependency outside original directory was generated")
		return nil
	}))
	assert.FileExists(t, filepath.Join(filepath.Dir(path), ".terragrunt-stack", "old"))
}

func TestCoordinatorResolvesSymlinksBeforeCheckingLimits(t *testing.T) {
	t.Parallel()
	opts, path := coordinatorOptions(t)
	opts.DiscoveryBoundary = "."
	alias := filepath.Join(opts.WorkingDir, "alias")
	require.NoError(t, os.Symlink(filepath.Dir(path), alias))

	c := generate.NewCoordinator(vfs.NewOSFS(), new(rewrite.Store), opts)
	require.NoError(t, c.Generate(t.Context(), filepath.Join(alias, "terragrunt.stack.hcl"), func(context.Context) error {
		assert.Fail(t, "symlink escaped discovery limit")
		return nil
	}))
	assert.FileExists(t, filepath.Join(filepath.Dir(path), ".terragrunt-stack", "old"))
}

func TestCoordinatorReusesNestedGeneration(t *testing.T) {
	t.Parallel()
	opts, parent := coordinatorOptions(t)
	c := generate.NewCoordinator(vfs.NewOSFS(), new(rewrite.Store), opts)
	child := filepath.Join(filepath.Dir(parent), ".terragrunt-stack", "child", "terragrunt.stack.hcl")
	marker := filepath.Join(filepath.Dir(child), ".terragrunt-stack", "unit")
	childWork := func(context.Context) error {
		require.NoError(t, os.MkdirAll(filepath.Dir(marker), 0755))
		return os.WriteFile(marker, []byte("generated"), 0600)
	}

	require.NoError(t, c.Generate(t.Context(), parent, func(ctx context.Context) error {
		require.NoError(t, os.MkdirAll(filepath.Dir(child), 0755))
		require.NoError(t, os.WriteFile(child, nil, 0600))

		return c.Generate(ctx, child, childWork)
	}))
	require.NoError(t, c.Generate(t.Context(), child, func(context.Context) error {
		assert.Fail(t, "nested stack generated twice")
		return nil
	}))
	assert.FileExists(t, marker)
}

func TestCoordinatorPreservesCompletedChildWhenParentIsRequestedLater(t *testing.T) {
	t.Parallel()
	opts, parent := coordinatorOptions(t)
	c := generate.NewCoordinator(vfs.NewOSFS(), new(rewrite.Store), opts)
	child := filepath.Join(filepath.Dir(parent), ".terragrunt-stack", "child", "terragrunt.stack.hcl")
	unit := filepath.Join(filepath.Dir(child), ".terragrunt-stack", "unit", "terragrunt.hcl")
	require.NoError(t, os.MkdirAll(filepath.Dir(child), 0755))
	require.NoError(t, os.WriteFile(child, nil, 0600))

	calls := 0
	childWork := func(context.Context) error {
		calls++

		if err := os.MkdirAll(filepath.Dir(unit), 0755); err != nil {
			return err
		}

		return os.WriteFile(unit, []byte("inputs = { marker = \"generated\" }"), 0600)
	}
	require.NoError(t, c.Generate(t.Context(), child, childWork))
	require.FileExists(t, unit)

	parentErr := c.Generate(t.Context(), parent, func(ctx context.Context) error {
		if err := os.MkdirAll(filepath.Dir(child), 0755); err != nil {
			return err
		}

		if err := os.WriteFile(child, nil, 0600); err != nil {
			return err
		}

		return c.Generate(ctx, child, childWork)
	})
	require.ErrorContains(t, parentErr, "generate parent stacks first")
	assert.Contains(t, parentErr.Error(), parent)
	assert.Contains(t, parentErr.Error(), child)

	require.NoError(t, c.Generate(t.Context(), child, childWork))
	assert.Equal(t, 1, calls, "a completed child must remain reusable without regeneration")
	assert.FileExists(t, unit, "cached success must still refer to generated files after the parent request")
}

func TestCoordinatorPreservesActiveChildWhenParentIsRequested(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		opts, parent := coordinatorOptions(t)
		c := generate.NewCoordinator(vfs.NewOSFS(), new(rewrite.Store), opts)
		child := filepath.Join(filepath.Dir(parent), ".terragrunt-stack", "child", "terragrunt.stack.hcl")
		unit := filepath.Join(filepath.Dir(child), ".terragrunt-stack", "unit", "terragrunt.hcl")
		require.NoError(t, os.MkdirAll(filepath.Dir(child), 0755))
		require.NoError(t, os.WriteFile(child, nil, 0600))

		started := make(chan error, 1)
		releaseChild := make(chan struct{})

		childDone := make(chan error, 1)
		go func() {
			childDone <- c.Generate(t.Context(), child, func(context.Context) error {
				err := os.MkdirAll(filepath.Dir(unit), 0755)
				if err == nil {
					err = os.WriteFile(unit, []byte("inputs = { marker = \"generated\" }"), 0600)
				}

				started <- err

				if err != nil {
					return err
				}

				<-releaseChild

				return nil
			})
		}()

		require.NoError(t, <-started)
		require.FileExists(t, unit)

		parentDone := make(chan error, 1)
		go func() {
			parentDone <- c.Generate(t.Context(), parent, func(ctx context.Context) error {
				if err := os.MkdirAll(filepath.Dir(child), 0755); err != nil {
					return err
				}

				if err := os.WriteFile(child, nil, 0600); err != nil {
					return err
				}

				return c.Generate(ctx, child, func(context.Context) error {
					assert.Fail(t, "active child was generated twice")
					return nil
				})
			})
		}()

		synctest.Wait()
		assert.FileExists(t, unit, "parent cleanup must not delete an active child's files")
		close(releaseChild)
		require.NoError(t, <-childDone)
		require.ErrorContains(t, <-parentDone, "generate parent stacks first")

		require.NoError(t, c.Generate(t.Context(), child, func(context.Context) error {
			assert.Fail(t, "completed child was generated twice")
			return nil
		}))
		assert.FileExists(t, unit, "cached success must still refer to generated files after both requests finish")
	})
}

func TestCoordinatorAllowsLateParentWithoutCleanup(t *testing.T) {
	t.Parallel()

	for _, command := range []string{"plan", "destroy"} {
		t.Run(command, func(t *testing.T) {
			t.Parallel()
			opts, parent := coordinatorOptions(t)
			opts.TofuCommand = command
			opts.SourceUpdate = command == "destroy"
			c := generate.NewCoordinator(vfs.NewOSFS(), new(rewrite.Store), opts)
			child := filepath.Join(filepath.Dir(parent), ".terragrunt-stack", "child", "terragrunt.stack.hcl")
			require.NoError(t, os.MkdirAll(filepath.Dir(child), 0755))
			require.NoError(t, os.WriteFile(child, nil, 0600))
			require.NoError(t, c.Generate(t.Context(), child, func(context.Context) error { return nil }))

			parentCalled := false

			require.NoError(t, c.Generate(t.Context(), parent, func(context.Context) error {
				parentCalled = true
				return nil
			}))
			assert.True(t, parentCalled)
			assert.FileExists(t, child)
		})
	}
}
