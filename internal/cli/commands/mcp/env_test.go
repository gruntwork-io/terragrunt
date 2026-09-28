package mcp_test

import (
	"path/filepath"
	"testing"

	tgmcp "github.com/gruntwork-io/terragrunt/internal/cli/commands/mcp"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/gruntwork-io/terragrunt/test/helpers/venvtest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// unitGetEnvConfig reads a variable the test server's environment sets, with a
// default that says when it was withheld.
const unitGetEnvConfig = `
inputs = {
  profile = get_env("AWS_PROFILE", "withheld")
}
`

// TestConfigurationsSeeTheEnvironmentOnlyUnderAllowEnv pins the env
// capability end to end: a configuration's get_env() finds a variable the
// server has only when --allow=env was granted.
func TestConfigurationsSeeTheEnvironmentOnlyUnderAllowEnv(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name  string
		want  string
		allow []string
	}{
		{name: "withheld by default", want: "withheld"},
		{name: "granted", allow: []string{"env"}, want: "prod"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			dir := newTestTree(t)
			unit := filepath.Join(dir, "env")

			fsys := vfs.NewOSFS()
			require.NoError(t, fsys.MkdirAll(unit, 0o755))
			require.NoError(
				t,
				vfs.WriteFile(fsys, filepath.Join(unit, "terragrunt.hcl"), []byte(unitGetEnvConfig), 0o644),
			)

			v := venvtest.NewWithOSFS().WithEnv(map[string]string{"AWS_PROFILE": "prod"})
			session := newTestSessionWith(t, dir, v, func(o *tgmcp.Options) { o.Allow = tc.allow }, nil)

			var out struct {
				Config struct {
					Inputs map[string]any `json:"inputs"`
				} `json:"config"`
			}

			callTool(t, session, "render_config", map[string]any{"working_dir": "env", "format": "full"}, &out)

			assert.Equal(t, tc.want, out.Config.Inputs["profile"])
		})
	}
}
