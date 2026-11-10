package run_test

import (
	"path/filepath"
	"testing"

	runcommand "github.com/gruntwork-io/terragrunt/internal/cli/commands/run"
	"github.com/gruntwork-io/terragrunt/internal/cli/flags/shared"
	rewrite "github.com/gruntwork-io/terragrunt/internal/config/rewrite"
	"github.com/gruntwork-io/terragrunt/internal/tf"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestNewCommandExposesEveryOpenTofuTerraformCommand pins the subcommands `run` offers and which ones help lists.
func TestNewCommandExposesEveryOpenTofuTerraformCommand(t *testing.T) {
	t.Parallel()

	rec := &recorder{}
	v := rec.venv(t, nil)

	cmd := runcommand.NewCommand(logger.CreateLogger(), newOpts(t, v, "", ""), v, new(rewrite.Store))

	assert.Equal(t, runcommand.CommandName, cmd.Name)
	assert.NotNil(t, cmd.Flags.Get(shared.AllFlagName), shared.AllFlagName)
	assert.NotNil(t, cmd.Flags.Get(shared.GraphFlagName), shared.GraphFlagName)
	require.Len(t, cmd.Subcommands, len(tf.CommandNames))

	for i, name := range tf.CommandNames {
		_, visible := tf.CommandUsages[name]

		assert.Equal(t, name, cmd.Subcommands[i].Name)
		assert.Equal(t, !visible, cmd.Subcommands[i].Hidden, "%s is listed in help only when it has a usage", name)
		assert.NotNil(t, cmd.Subcommands[i].CustomHelp, "%s prints the OpenTofu/Terraform help", name)
	}
}

// TestNewCommandShowsHelpWithoutACommand pins that bare `run` prints its help instead of running anything.
func TestNewCommandShowsHelpWithoutACommand(t *testing.T) {
	t.Parallel()

	rec := &recorder{}
	v := rec.venv(t, map[string]string{"unit/terragrunt.hcl": "", "unit/main.tf": ""})

	cmd := runcommand.NewCommand(logger.CreateLogger(), newOpts(t, v, "unit", tf.CommandNamePlan), v, new(rewrite.Store))
	cmd.CustomHelpTemplate = "run help"

	out, err := runCommand(t, cmd)
	require.NoError(t, err)
	assert.Equal(t, "run help", out)
	assert.Empty(t, rec.invocations(), "help runs nothing")
}

// TestNewCommandRunsTheCommand pins the argument forms that reach a single-unit run.
func TestNewCommandRunsTheCommand(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name string
		args []string
	}{
		{name: "after the separator", args: []string{"--", tf.CommandNamePlan}},
		{name: "as a subcommand", args: []string{tf.CommandNamePlan}},
		{name: "a command run does not list", args: []string{"--", "custom-command"}},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			rec := &recorder{}
			v := rec.venv(t, map[string]string{"unit/terragrunt.hcl": "", "unit/main.tf": ""})

			cmd := runcommand.NewCommand(logger.CreateLogger(), newOpts(t, v, "unit", tf.CommandNamePlan), v, new(rewrite.Store))

			_, err := runCommand(t, cmd, tc.args...)
			require.NoError(t, err)
			assert.Equal(t, [][]string{{tf.CommandNameInit, "-no-color"}, {tf.CommandNamePlan}}, rec.commands())
		})
	}
}

// TestNewCommandRunsEveryUnit pins that --all and --graph run the command across the stack.
func TestNewCommandRunsEveryUnit(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name string
		unit string
		args []string
	}{
		{name: "all", args: []string{"--" + shared.AllFlagName}},
		{name: "graph", unit: "vpc", args: []string{"--" + shared.GraphFlagName}},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			rec := &recorder{}
			v := rec.venv(t, dependentUnits)

			opts := newOpts(t, v, tc.unit, tf.CommandNamePlan)
			opts.GraphRoot = memRoot

			_, err := runCommand(t, runcommand.NewCommand(logger.CreateLogger(), opts, v, new(rewrite.Store)), tc.args...)
			require.NoError(t, err)
			assert.Equal(
				t,
				[]string{filepath.Join(memRoot, "app"), filepath.Join(memRoot, "vpc")},
				rec.unitsRun(tf.CommandNamePlan),
			)
		})
	}
}
