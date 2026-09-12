package mcp_test

import (
	"testing"

	tgmcp "github.com/gruntwork-io/terragrunt/internal/cli/commands/mcp"
	"github.com/gruntwork-io/terragrunt/internal/clihelper"
	"github.com/gruntwork-io/terragrunt/internal/experiment"
	"github.com/gruntwork-io/terragrunt/internal/vexec"
	"github.com/gruntwork-io/terragrunt/pkg/options"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/gruntwork-io/terragrunt/test/helpers/venvtest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestMCPExperimentGate pins the decision the mcp command's Before hook makes:
// the command is rejected unless the mcp-command experiment is enabled.
func TestMCPExperimentGate(t *testing.T) {
	t.Parallel()

	newCommand := func(t *testing.T) (*clihelper.Command, *options.TerragruntOptions) {
		t.Helper()

		opts := options.NewTerragruntOptions(vexec.NewOSExec())

		return tgmcp.NewCommand(logger.CreateLogger(), opts, venvtest.NewWithOSFS()), opts
	}

	t.Run("refused without the experiment", func(t *testing.T) {
		t.Parallel()

		cmd, _ := newCommand(t)

		err := cmd.Before(t.Context(), nil)
		require.ErrorIs(t, err, tgmcp.ErrExperimentRequired)
	})

	t.Run("accepted with the experiment", func(t *testing.T) {
		t.Parallel()

		cmd, opts := newCommand(t)
		require.NoError(t, opts.Experiments.EnableExperiment(experiment.MCPCommand))

		require.NoError(t, cmd.Before(t.Context(), nil))
	})
}

func TestNewCommandIsNamedMCP(t *testing.T) {
	t.Parallel()

	cmd := tgmcp.NewCommand(
		logger.CreateLogger(),
		options.NewTerragruntOptions(vexec.NewOSExec()),
		venvtest.NewWithOSFS(),
	)

	require.NotNil(t, cmd)
	assert.Equal(t, tgmcp.CommandName, cmd.Name)
	assert.NotEmpty(t, cmd.Flags)
}
