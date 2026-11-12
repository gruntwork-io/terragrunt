package discovery_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/discovery"
	"github.com/gruntwork-io/terragrunt/internal/vexec"
	"github.com/gruntwork-io/terragrunt/pkg/config"
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

			paths, err := discovery.UnitDependencyPaths(config.WithCaches(t.Context()), logger.CreateLogger(), v, opts, filepath.Join(root, "terragrunt.hcl"), config.DiagnosticsLogged)

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
