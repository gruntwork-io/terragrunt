package mcp_test

import (
	"path/filepath"
	"testing"

	tgmcp "github.com/gruntwork-io/terragrunt/internal/cli/commands/mcp"
	"github.com/gruntwork-io/terragrunt/internal/experiment"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/gruntwork-io/terragrunt/pkg/config"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const unitCommandsVariableConfig = `
inputs = {
  vars = commands.accept_vars
}
`

func TestRenderConfigToolResolvesCommandsVariableWithExperiment(t *testing.T) {
	t.Parallel()

	session := newCommandsVariableSession(t, experiment.CommandsVariable)

	var out struct {
		Config struct {
			Inputs map[string]any `json:"inputs"`
		} `json:"config"`
	}

	callTool(t, session, "render_config", map[string]any{"working_dir": "commands", "format": "full"}, &out)

	want := make([]any, 0, len(config.TerraformCommandsNeedVars))
	for _, command := range config.TerraformCommandsNeedVars {
		want = append(want, command)
	}

	assert.Equal(t, want, out.Config.Inputs["vars"])
}

func TestRenderConfigToolNamesTheCommandsVariableExperimentWhenDisabled(t *testing.T) {
	t.Parallel()

	session := newCommandsVariableSession(t)

	res, err := session.CallTool(t.Context(), &mcp.CallToolParams{
		Name:      "render_config",
		Arguments: map[string]any{"working_dir": "commands", "format": "full"},
	})
	require.NoError(t, err)
	require.True(t, res.IsError)
	require.Len(t, res.Content, 1)

	text, ok := res.Content[0].(*mcp.TextContent)
	require.True(t, ok)
	assert.Contains(t, text.Text, "--experiment "+experiment.CommandsVariable)
}

func newCommandsVariableSession(t *testing.T, experiments ...string) *mcp.ClientSession {
	t.Helper()

	dir := newTestTree(t)
	unit := filepath.Join(dir, "commands")

	fsys := vfs.NewOSFS()
	require.NoError(t, fsys.MkdirAll(unit, 0o755))
	require.NoError(
		t,
		vfs.WriteFile(fsys, filepath.Join(unit, "terragrunt.hcl"), []byte(unitCommandsVariableConfig), 0o644),
	)

	return newTestSessionWithOptions(t, dir, func(o *tgmcp.Options) {
		for _, name := range experiments {
			require.NoError(t, o.Experiments.EnableExperiment(name))
		}
	})
}
