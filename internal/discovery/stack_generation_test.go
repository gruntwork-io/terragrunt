package discovery_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	rewrite "github.com/gruntwork-io/terragrunt/internal/config/rewrite"
	"github.com/gruntwork-io/terragrunt/internal/discovery"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/internal/vexec"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/gruntwork-io/terragrunt/pkg/config"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/pkg/options"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/gruntwork-io/terragrunt/test/helpers/venvtest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStackReferenceDiscoveryHonorsAuthProviderOptOut(t *testing.T) {
	t.Parallel()

	for _, enabled := range []bool{false, true} {
		name := "disabled"
		if enabled {
			name = "enabled"
		}

		t.Run(name, func(t *testing.T) {
			t.Parallel()

			root := venvtest.Root("/repo")
			fsys := venvtest.NewFS(t, root, map[string]string{
				"terragrunt.hcl": `dependencies { paths = [get_env("STACK_PATH", "fallback")] }`,
			})
			calls := 0
			v := venvtest.NewOSWithEmptyEnv().WithFS(fsys)
			v.Exec = vexec.NewMemExec(func(_ context.Context, _ vexec.Invocation) vexec.Result {
				calls++
				return vexec.Result{Stdout: []byte(`{"envs":{"STACK_PATH":"producer"}}`)}
			})
			opts := options.NewTerragruntOptions(v.Exec)
			opts.WorkingDir = root
			opts.DiscoveryAuthProviderCmd = enabled
			opts.AuthProviderCmd = "fake-auth"

			paths, err := discovery.UnitDependencyPaths(
				config.WithCaches(t.Context()),
				logger.CreateLogger(),
				v,
				opts,
				filepath.Join(root, "terragrunt.hcl"),
				config.DiagnosticsLogged,
			)

			require.NoError(t, err)

			expected := "fallback"
			if enabled {
				expected = "producer"

				assert.Equal(t, 1, calls)
			} else {
				assert.Zero(t, calls)
			}

			assert.Equal(t, []string{filepath.Join(root, expected)}, paths)
			assert.NotContains(t, v.Env, "STACK_PATH")
		})
	}
}

type unreadableDiscoveryDirFS struct {
	vfs.FS
	target     string
	unreadable bool
}

func (f *unreadableDiscoveryDirFS) Open(name string) (vfs.File, error) {
	if f.unreadable && name == f.target {
		return nil, &os.PathError{Op: "open", Path: name, Err: os.ErrPermission}
	}

	return f.FS.Open(name)
}

func TestStackDiscoveryRescanPreservesFilesystemErrorPolicy(t *testing.T) {
	t.Parallel()

	for _, afterGeneration := range []bool{false, true} {
		for _, suppress := range []bool{false, true} {
			t.Run(fmt.Sprintf("after_generation=%t/suppress=%t", afterGeneration, suppress), func(t *testing.T) {
				t.Parallel()

				root := venvtest.Root("/repo")
				fsys := &unreadableDiscoveryDirFS{
					FS: venvtest.NewFS(t, root, map[string]string{
						"unreadable/terragrunt.hcl": "",
						"valid/terragrunt.hcl":      "",
					}),
					target:     filepath.Join(root, "unreadable"),
					unreadable: !afterGeneration,
				}
				v := venvtest.NewOSWithEmptyEnv().WithFS(fsys)
				opts := options.NewTerragruntOptions(v.Exec)
				opts.WorkingDir = root
				ctx := discovery.WithStackDiscovery(
					config.WithCaches(t.Context()),
					func(context.Context, log.Logger, *venv.Venv, []string, bool) (bool, error) {
						if fsys.unreadable {
							return false, nil
						}

						fsys.unreadable = true

						return true, nil
					},
				)

				d := discovery.NewDiscovery(root).WithRelationships()
				if suppress {
					d = d.WithSuppressParseErrors()
				}

				_, err := d.Discover(ctx, logger.CreateLogger(), v, new(rewrite.Store), opts)
				if !suppress {
					require.ErrorIs(t, err, os.ErrPermission)
					return
				}

				require.NoError(t, err)
			})
		}
	}
}
