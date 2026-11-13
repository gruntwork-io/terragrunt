package generate_test

import (
	"errors"
	"fmt"
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
	"github.com/stretchr/testify/assert"
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

type blockedStackValuesFS struct {
	vfs.FS
	release <-chan struct{}
	target  string
}

func (f *blockedStackValuesFS) OpenFile(name string, flag int, perm os.FileMode) (vfs.File, error) {
	if name == f.target {
		<-f.release
	}

	return f.FS.OpenFile(name, flag, perm)
}

func TestGenerateDependencyStackWaitsForParentValues(t *testing.T) {
	t.Parallel()

	for _, noStack := range []bool{false, true} {
		for _, blockedFile := range []string{config.DefaultStackFile, "terragrunt.values.hcl"} {
			t.Run(fmt.Sprintf("no_stack=%t/blocked=%s", noStack, blockedFile), func(t *testing.T) {
				t.Parallel()
				synctest.Test(t, func(t *testing.T) {
					root := venvtest.Root("/repo")
					fsys := venvtest.NewFS(t, root, map[string]string{
						"parent/terragrunt.stack.hcl": fmt.Sprintf(`stack "nested" {
 source = "../child-source"
 path = "nested"
 no_dot_terragrunt_stack = %t
 values = { source = %q }
}
`, noStack, filepath.Join(root, "unit-source")),
						"child-source/terragrunt.stack.hcl": `unit "app" {
 source = values.source
 path = "app"
}
`,
						"unit-source/terragrunt.hcl": "# generated with parent values",
					})

					nested := filepath.Join(root, "parent/.terragrunt-stack/nested")
					if noStack {
						nested = filepath.Join(root, "parent/nested")
					}

					release := make(chan struct{})
					v := venvtest.NewOSWithEmptyEnv().WithFS(&blockedStackValuesFS{
						FS: fsys, target: filepath.Join(nested, blockedFile), release: release,
					})
					opts := options.NewTerragruntOptions(v.Exec)
					opts.WorkingDir, opts.RootWorkingDir = root, root
					opts.NoCAS, opts.Parallelism = true, 2
					require.NoError(t, opts.Experiments.EnableExperiment(experiment.DependencyStackGeneration))

					ctx := generate.WithCoordinator(config.WithCaches(t.Context()), v.FS, new(rewrite.Store), opts)

					parentDone := make(chan error, 1)
					go func() {
						parentDone <- config.GenerateStackDependency(ctx, logger.CreateLogger(), v, filepath.Join(root, "parent"))
					}()

					synctest.Wait()
					assert.Equal(
						t,
						blockedFile != config.DefaultStackFile,
						vfs.Exists(fsys, filepath.Join(nested, config.DefaultStackFile)),
					)
					assert.False(t, vfs.Exists(fsys, filepath.Join(nested, "terragrunt.values.hcl")))

					childDone := make(chan error, 1)
					go func() {
						childDone <- config.GenerateStackDependency(ctx, logger.CreateLogger(), v, nested)
					}()

					synctest.Wait()

					var childErr error

					childFinished := false

					select {
					case childErr = <-childDone:
						childFinished = true

						assert.Fail(t, "nested generation finished before its parent wrote its values", "%v", childErr)
					default:
					}

					close(release)
					assert.NoError(t, <-parentDone)

					if !childFinished {
						childErr = <-childDone
					}

					assert.NoError(t, childErr)
					assert.NoError(t, config.GenerateStackDependency(ctx, logger.CreateLogger(), v, nested))
					contents, err := vfs.ReadFile(fsys, filepath.Join(nested, ".terragrunt-stack/app/terragrunt.hcl"))
					require.NoError(t, err)
					assert.Equal(t, "# generated with parent values", string(contents))
				})
			})
		}
	}
}

func TestGenerateStackReadsIndependentDescendantWhileParsing(t *testing.T) {
	t.Parallel()

	root := venvtest.Root("/repo")
	fsys := venvtest.NewFS(t, root, map[string]string{
		"parent/terragrunt.stack.hcl": `locals {
 settings = read_terragrunt_config("./settings.hcl")
}
unit "app" {
 source = local.settings.locals.source
 path = "app"
}
`,
		"parent/settings.hcl": fmt.Sprintf(`locals { source = %q }
dependency "independent" {
 config_path = "./independent"
 skip_outputs = true
}
`, filepath.Join(root, "unit-source")),
		"parent/independent/terragrunt.stack.hcl": `unit "app" {
 source = "../../unit-source"
 path = "app"
}
`,
		"unit-source/terragrunt.hcl": "# app",
	})
	v := venvtest.NewOSWithEmptyEnv().WithFS(fsys)
	opts := options.NewTerragruntOptions(v.Exec)
	opts.WorkingDir, opts.RootWorkingDir = root, root
	opts.NoCAS, opts.Parallelism = true, 2
	require.NoError(t, opts.Experiments.EnableExperiment(experiment.DependencyStackGeneration))
	ctx := generate.WithCoordinator(config.WithCaches(t.Context()), fsys, new(rewrite.Store), opts)
	require.NoError(t, config.GenerateStackDependency(ctx, logger.CreateLogger(), v, filepath.Join(root, "parent")))
	require.True(t, vfs.Exists(fsys, filepath.Join(root, "parent/.terragrunt-stack/app/terragrunt.hcl")))
	require.True(t, vfs.Exists(fsys, filepath.Join(root, "parent/independent/.terragrunt-stack/app/terragrunt.hcl")))
}

func TestGenerateDependencyStackWaitsForRecursiveCompletion(t *testing.T) {
	t.Parallel()

	for _, failCopy := range []bool{false, true} {
		t.Run(fmt.Sprintf("dependency_fails=%t", failCopy), func(t *testing.T) {
			t.Parallel()
			synctest.Test(t, func(t *testing.T) {
				root := venvtest.Root("/repo")
				fsys := venvtest.NewFS(t, root, map[string]string{
					"parent/terragrunt.stack.hcl": fmt.Sprintf(`stack "nested" {
 source = "../nested-source"
 path = "nested"
 values = { source = %q }
}
`, filepath.Join(root, "relay-source")),
					"nested-source/terragrunt.stack.hcl": `unit "app" {
 source = values.source
 path = "app"
}`,
					"relay-source/terragrunt.hcl": fmt.Sprintf(`dependencies { paths = [%q] }`, filepath.Join(root, "independent")),
					"independent/terragrunt.stack.hcl": `unit "app" {
 source = "../unit-source"
 path = "app"
}`,
					"unrelated/terragrunt.stack.hcl": `unit "app" {
 source = "../unit-source"
 path = "app"
}`,
					"unit-source/terragrunt.hcl": "# independent",
				})
				nested := filepath.Join(root, "parent/.terragrunt-stack/nested")
				filesReady := make(chan struct{})
				dependencyReady := make(chan struct{})
				target := filepath.Join(root, "independent/.terragrunt-stack/app/terragrunt.hcl")

				var dependencyFS vfs.FS = &blockedStackValuesFS{FS: fsys, target: target, release: dependencyReady}
				if failCopy {
					dependencyFS = &blockedStackCopyFS{FS: fsys, target: target, release: dependencyReady}
				}

				v := venvtest.NewOSWithEmptyEnv().WithFS(&blockedStackValuesFS{
					FS: dependencyFS, target: filepath.Join(nested, "terragrunt.values.hcl"), release: filesReady,
				})
				opts := options.NewTerragruntOptions(v.Exec)
				opts.WorkingDir, opts.RootWorkingDir = root, root
				opts.NoCAS, opts.Parallelism = true, 4
				require.NoError(t, opts.Experiments.EnableExperiment(experiment.DependencyStackGeneration))

				ctx := generate.WithCoordinator(config.WithCaches(t.Context()), v.FS, new(rewrite.Store), opts)

				parentDone := make(chan error, 1)
				go func() {
					parentDone <- config.GenerateStackDependency(ctx, logger.CreateLogger(), v, filepath.Join(root, "parent"))
				}()

				synctest.Wait()

				consumers := []chan error{make(chan error, 1), make(chan error, 1)}
				for _, done := range consumers {
					go func() { done <- config.GenerateStackDependency(ctx, logger.CreateLogger(), v, nested) }()
				}

				synctest.Wait()
				close(filesReady)
				synctest.Wait()

				assert.True(t, vfs.Exists(fsys, filepath.Join(nested, ".terragrunt-stack/app/terragrunt.hcl")))
				assert.Empty(t, parentDone, "parent returned while its nested dependency was unfinished")

				for _, done := range consumers {
					assert.Empty(t, done, "consumer treated copied files as completed generation")
				}

				require.NoError(t, config.GenerateStackDependency(ctx, logger.CreateLogger(), v, filepath.Join(root, "unrelated")))
				assert.True(t, vfs.Exists(fsys, filepath.Join(root, "unrelated/.terragrunt-stack/app/terragrunt.hcl")))
				close(dependencyReady)

				for _, done := range append(consumers, parentDone) {
					err := <-done
					if failCopy {
						assert.ErrorContains(t, err, "copy failed")
					} else {
						assert.NoError(t, err)
					}
				}
			})
		})
	}
}
