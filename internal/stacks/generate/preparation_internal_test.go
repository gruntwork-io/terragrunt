package generate

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"testing"
	"testing/synctest"

	rewrite "github.com/gruntwork-io/terragrunt/internal/config/rewrite"
	"github.com/gruntwork-io/terragrunt/internal/experiment"
	"github.com/gruntwork-io/terragrunt/pkg/options"
	"github.com/gruntwork-io/terragrunt/test/helpers/venvtest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPreparedStackReleasesWaitersOnFailure(t *testing.T) {
	t.Parallel()

	for _, panicOnCopy := range []bool{false, true} {
		t.Run(fmt.Sprintf("panic=%t", panicOnCopy), func(t *testing.T) {
			t.Parallel()
			synctest.Test(t, func(t *testing.T) {
				root := venvtest.Root("/repo")
				v := venvtest.NewOSWithEmptyEnv().WithFS(venvtest.NewFS(t, root, nil))
				opts := options.NewTerragruntOptions(v.Exec)
				opts.WorkingDir = root
				require.NoError(t, opts.Experiments.EnableExperiment(experiment.DependencyStackGeneration))
				c := NewCoordinator(v.FS, new(rewrite.Store), opts)
				child := filepath.Join(root, "child")
				parent := filepath.Join(root, "terragrunt.stack.hcl")
				release := make(chan struct{})
				parentDone := make(chan error, 1)
				failure := errors.New("copy failed")

				go func() {
					parentDone <- c.Generate(t.Context(), parent, func(ctx context.Context) error {
						if err := c.registerOutputs(ctx, []string{child}); err != nil {
							return err
						}

						<-release

						if panicOnCopy {
							panic(failure)
						}

						return failure
					})
				}()

				synctest.Wait()

				canceled, cancel := context.WithCancel(t.Context())

				canceledDone := make(chan error, 1)
				go func() { canceledDone <- c.waitForFiles(canceled, child) }()

				waiterDone := make(chan error, 1)
				go func() { waiterDone <- c.waitForFiles(t.Context(), child) }()

				synctest.Wait()
				cancel()
				require.ErrorIs(t, <-canceledDone, context.Canceled)
				assert.Empty(t, parentDone)
				assert.Empty(t, waiterDone)
				close(release)
				require.ErrorContains(t, <-parentDone, "copy failed")
				require.ErrorContains(t, <-waiterDone, "copy failed")
				require.ErrorContains(t, c.waitForFiles(t.Context(), child), "copy failed")
			})
		})
	}
}

func TestPreparedStackRejectsLateOrConflictingProducer(t *testing.T) {
	t.Parallel()

	for _, alreadyGenerated := range []bool{false, true} {
		t.Run(fmt.Sprintf("already_generated=%t", alreadyGenerated), func(t *testing.T) {
			t.Parallel()

			root := venvtest.Root("/repo")
			v := venvtest.NewOSWithEmptyEnv().WithFS(venvtest.NewFS(t, root, nil))
			opts := options.NewTerragruntOptions(v.Exec)
			opts.WorkingDir = root
			require.NoError(t, opts.Experiments.EnableExperiment(experiment.DependencyStackGeneration))
			c := NewCoordinator(v.FS, new(rewrite.Store), opts)

			child := filepath.Join(root, "child")
			if alreadyGenerated {
				require.NoError(
					t,
					c.Generate(t.Context(), filepath.Join(child, "terragrunt.stack.hcl"), func(context.Context) error { return nil }),
				)
			} else {
				require.NoError(
					t,
					c.Generate(t.Context(), filepath.Join(root, "first/terragrunt.stack.hcl"), func(ctx context.Context) error {
						return c.registerOutputs(ctx, []string{child})
					}),
				)
			}

			err := c.Generate(t.Context(), filepath.Join(root, "second/terragrunt.stack.hcl"), func(ctx context.Context) error {
				return c.registerOutputs(ctx, []string{child})
			})
			require.ErrorContains(t, err, "cannot generate into")
			assert.Contains(t, err.Error(), child)
		})
	}
}

func TestPreparedStackDetectsReadinessCycle(t *testing.T) {
	t.Parallel()

	root := venvtest.Root("/repo")
	v := venvtest.NewOSWithEmptyEnv().WithFS(venvtest.NewFS(t, root, nil))
	opts := options.NewTerragruntOptions(v.Exec)
	opts.WorkingDir = root
	require.NoError(t, opts.Experiments.EnableExperiment(experiment.DependencyStackGeneration))
	c := NewCoordinator(v.FS, new(rewrite.Store), opts)
	child := filepath.Join(root, "child")
	err := c.Generate(t.Context(), filepath.Join(root, "parent/terragrunt.stack.hcl"), func(ctx context.Context) error {
		if err := c.registerOutputs(ctx, []string{child}); err != nil {
			return err
		}

		return c.Generate(ctx, filepath.Join(root, "independent/terragrunt.stack.hcl"), func(ctx context.Context) error {
			return c.waitForFiles(ctx, child)
		})
	})
	require.ErrorContains(t, err, "stack dependency cycle")
	require.NoError(t, t.Context().Err())
}
