package generate_test

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"testing/synctest"

	rewrite "github.com/gruntwork-io/terragrunt/internal/config/rewrite"
	"github.com/gruntwork-io/terragrunt/internal/experiment"
	"github.com/gruntwork-io/terragrunt/internal/stacks/generate"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/gruntwork-io/terragrunt/pkg/config"
	"github.com/gruntwork-io/terragrunt/pkg/options"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/gruntwork-io/terragrunt/test/helpers/venvtest"
	"github.com/stretchr/testify/require"
)

type blockedStackCopyFS struct {
	vfs.FS
	release     <-chan struct{}
	target      string
	panicOnCopy bool
}

func (f *blockedStackCopyFS) OpenFile(name string, flag int, perm os.FileMode) (vfs.File, error) {
	if name == f.target {
		<-f.release

		if f.panicOnCopy {
			panic("copy panicked")
		}

		return nil, errors.New("copy failed")
	}

	return f.FS.OpenFile(name, flag, perm)
}

func TestGenerateStacksWaitsForCopiesAndKeepsFailuresWithTheirStack(t *testing.T) {
	t.Parallel()

	for _, panicOnCopy := range []bool{false, true} {
		name := "copy error"
		if panicOnCopy {
			name = "copy panic"
		}

		t.Run(name, func(t *testing.T) {
			t.Parallel()
			synctest.Test(t, func(t *testing.T) {
				root := venvtest.Root("/repo")
				fsys := venvtest.NewFS(t, root, map[string]string{
					"first/terragrunt.stack.hcl":  "unit \"app\" {\n source = \"../source\"\n path = \"app\"\n}\n",
					"second/terragrunt.stack.hcl": "unit \"app\" {\n source = \"../source\"\n path = \"app\"\n}\n",
					"source/terragrunt.hcl":       "# generated unit",
				})
				release := make(chan struct{})
				v := venvtest.NewOSWithEmptyEnv().WithFS(&blockedStackCopyFS{
					FS:          fsys,
					release:     release,
					target:      filepath.Join(root, "first/.terragrunt-stack/app/terragrunt.hcl"),
					panicOnCopy: panicOnCopy,
				})
				opts := options.NewTerragruntOptions(v.Exec)
				opts.WorkingDir, opts.RootWorkingDir = root, root
				opts.NoCAS, opts.Parallelism = true, 2
				require.NoError(t, opts.Experiments.EnableExperiment(experiment.DependencyStackGeneration))

				s := new(rewrite.Store)
				ctx := generate.WithCoordinator(config.WithCaches(t.Context()), v.FS, s, opts)

				done := make(chan error, 1)
				go func() { done <- generate.NewGenerator().GenerateStacks(ctx, logger.CreateLogger(), v, s, opts, nil) }()

				synctest.Wait()

				select {
				case <-done:
					require.FailNow(t, "published a result before the copy finished")
				default:
				}

				close(release)
				require.Error(t, <-done)

				failedOpts := opts.Clone()
				failedOpts.WorkingDir = filepath.Join(root, "first")
				err := generate.NewGenerator().GenerateStacks(ctx, logger.CreateLogger(), v, s, failedOpts, nil)

				expected := "copy failed"
				if panicOnCopy {
					expected = "copy panicked"
				}

				require.ErrorContains(t, err, expected)

				successfulOpts := opts.Clone()
				successfulOpts.WorkingDir = filepath.Join(root, "second")
				require.NoError(t, generate.NewGenerator().GenerateStacks(ctx, logger.CreateLogger(), v, s, successfulOpts, nil))
				data, err := vfs.ReadFile(v.FS, filepath.Join(root, "second/.terragrunt-stack/app/terragrunt.hcl"))
				require.NoError(t, err)
				require.Equal(t, "# generated unit", string(data))
			})
		})
	}
}
