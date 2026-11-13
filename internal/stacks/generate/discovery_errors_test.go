package generate //nolint:testpackage // Exercises private coordinator discovery and generation state.

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	rewrite "github.com/gruntwork-io/terragrunt/internal/config/rewrite"
	"github.com/gruntwork-io/terragrunt/internal/experiment"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/internal/vexec"
	"github.com/gruntwork-io/terragrunt/pkg/config"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/pkg/options"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/gruntwork-io/terragrunt/test/helpers/venvtest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStackDiscoverySurfacesUnitErrorsWhenGenerationStops(t *testing.T) {
	t.Parallel()

	for _, hasStack := range []bool{false, true} {
		name := "without generation"
		if hasStack {
			name = "after generation"
		}

		t.Run(name, func(t *testing.T) {
			t.Parallel()

			root := venvtest.Root("/repo")
			fsys := venvtest.NewFS(t, root, map[string]string{
				"broken/terragrunt.hcl":         `dependencies { paths = [values.next] }`,
				"consumer/terragrunt.hcl":       `dependencies { paths = ["../producer"] }`,
				"producer/terragrunt.stack.hcl": "",
			})
			v := venvtest.NewOSWithEmptyEnv().WithFS(fsys)
			opts := options.NewTerragruntOptions(v.Exec)
			opts.WorkingDir = root
			c := NewCoordinator(fsys, new(rewrite.Store), opts)
			calls := 0
			ctx := config.WithStackGenerator(
				config.WithCaches(t.Context()),
				func(ctx context.Context, _ log.Logger, _ *venv.Venv, dir string) error {
					calls++

					return c.generateSelected(
						ctx,
						filepath.Join(dir, config.DefaultStackFile),
						func(context.Context) error { return nil },
					)
				},
			)

			roots := []string{filepath.Join(root, "broken/terragrunt.hcl")}
			if hasStack {
				roots = append(roots, filepath.Join(root, "consumer/terragrunt.hcl"))
			}

			progressed, err := c.discoverDependencyStacks(ctx, logger.CreateLogger(), v, roots, false)
			if hasStack {
				require.NoError(t, err)
				require.True(t, progressed)
				progressed, err = c.discoverDependencyStacks(ctx, logger.CreateLogger(), v, roots, false)

				assert.Equal(t, 1, calls)
			} else {
				assert.Zero(t, calls)
			}

			require.Error(t, err)
			assert.Contains(t, err.Error(), "values")
			assert.False(t, progressed)
		})
	}
}

func TestStackDiscoveryAbortsOnGenerationFailureOrCancellation(t *testing.T) {
	t.Parallel()

	for _, canceled := range []bool{false, true} {
		name := "generation failure"
		if canceled {
			name = "cancellation"
		}

		t.Run(name, func(t *testing.T) {
			t.Parallel()

			root := venvtest.Root("/repo")
			fsys := venvtest.NewFS(t, root, map[string]string{
				"broken/terragrunt.hcl":       `dependencies { paths = [values.next] }`,
				"consumer/terragrunt.hcl":     `dependencies { paths = ["../first", "../second"] }`,
				"first/terragrunt.stack.hcl":  "",
				"second/terragrunt.stack.hcl": "",
			})
			v := venvtest.NewOSWithEmptyEnv().WithFS(fsys)
			opts := options.NewTerragruntOptions(v.Exec)
			opts.WorkingDir = root
			c := NewCoordinator(fsys, new(rewrite.Store), opts)

			ctx, cancel := context.WithCancel(config.WithCaches(t.Context()))
			defer cancel()

			failure := errors.New("stack copy failed")
			calls := 0
			ctx = config.WithStackGenerator(ctx, func(context.Context, log.Logger, *venv.Venv, string) error {
				calls++

				if canceled {
					cancel()
					return context.Canceled
				}

				return failure
			})

			progressed, err := c.discoverDependencyStacks(ctx, logger.CreateLogger(), v, []string{
				filepath.Join(root, "broken/terragrunt.hcl"), filepath.Join(root, "consumer/terragrunt.hcl"),
			}, true)

			assert.False(t, progressed)
			assert.Equal(t, 1, calls)

			if canceled {
				require.ErrorIs(t, err, context.Canceled)
			} else {
				require.ErrorIs(t, err, failure)
			}
		})
	}
}

func TestStackDiscoveryAbortsOnAuthFailureOrCancellationDuringParsing(t *testing.T) {
	t.Parallel()

	for _, canceled := range []bool{false, true} {
		name := "auth failure"
		if canceled {
			name = "cancellation"
		}

		t.Run(name, func(t *testing.T) {
			t.Parallel()

			root := venvtest.Root("/repo")
			fsys := venvtest.NewFS(t, root, map[string]string{
				"consumer/terragrunt.hcl":       `dependencies { paths = ["../producer"] }`,
				"producer/terragrunt.stack.hcl": "",
			})

			ctx, cancel := context.WithCancel(config.WithCaches(t.Context()))
			defer cancel()

			v := venvtest.NewOSWithEmptyEnv().WithFS(fsys)
			v.Exec = vexec.NewMemExec(func(context.Context, vexec.Invocation) vexec.Result {
				if canceled {
					cancel()
					return vexec.Result{Stdout: []byte(`{"envs":{}}`)}
				}

				return vexec.Result{Err: errors.New("auth provider failed")}
			})
			opts := options.NewTerragruntOptions(v.Exec)
			opts.WorkingDir = root
			opts.DiscoveryAuthProviderCmd = true
			opts.AuthProviderCmd = "fake-auth"
			require.NoError(t, opts.Experiments.EnableExperiment(experiment.DependencyStackGeneration))
			c := NewCoordinator(fsys, new(rewrite.Store), opts)
			ctx = config.WithStackGenerator(ctx, func(context.Context, log.Logger, *venv.Venv, string) error {
				assert.Fail(t, "generation must not start after auth failure or cancellation")
				return nil
			})

			progressed, err := c.discoverDependencyStacks(
				ctx,
				logger.CreateLogger(),
				v,
				[]string{filepath.Join(root, "consumer/terragrunt.hcl")},
				true,
			)

			assert.False(t, progressed)
			require.Error(t, err)

			if canceled {
				assert.ErrorIs(t, err, context.Canceled)
			} else {
				assert.Contains(t, err.Error(), "auth provider failed")
			}
		})
	}
}
