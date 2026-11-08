package run_test

import (
	"maps"
	"path/filepath"
	"testing"

	runcommand "github.com/gruntwork-io/terragrunt/internal/cli/commands/run"
	"github.com/gruntwork-io/terragrunt/internal/cli/flags/shared"
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

	cmd := runcommand.NewCommand(logger.CreateLogger(), newOpts(t, v, "", ""), v)

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

	cmd := runcommand.NewCommand(logger.CreateLogger(), newOpts(t, v, "unit", tf.CommandNamePlan), v)
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
		name    string
		command string
		args    []string
	}{
		{name: "after the separator", command: tf.CommandNamePlan, args: []string{"--", tf.CommandNamePlan}},
		{name: "as a subcommand", command: tf.CommandNamePlan, args: []string{tf.CommandNamePlan}},
		{name: "a command run does not list", command: "custom-command", args: []string{"--", "custom-command"}},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			rec := &recorder{}
			v := rec.venv(t, map[string]string{"unit/terragrunt.hcl": "", "unit/main.tf": ""})

			cmd := runcommand.NewCommand(logger.CreateLogger(), newOpts(t, v, "unit", tc.command), v)

			_, err := runCommand(t, cmd, tc.args...)
			require.NoError(t, err)
			assert.Equal(t, [][]string{{tf.CommandNameInit, "-no-color"}, {tc.command}}, rec.commands())
		})
	}
}

// TestNewCommandRunsTheSelectedUnits pins that --all runs every unit and --graph runs only the graph root and its dependents.
func TestNewCommandRunsTheSelectedUnits(t *testing.T) {
	t.Parallel()

	units := maps.Clone(dependentUnits)
	units["db/terragrunt.hcl"] = ""
	units["db/main.tf"] = ""

	testCases := []struct {
		name      string
		unit      string
		args      []string
		wantUnits []string
	}{
		{
			name:      "all",
			args:      []string{"--" + shared.AllFlagName},
			wantUnits: []string{filepath.Join(memRoot, "app"), filepath.Join(memRoot, "db"), filepath.Join(memRoot, "vpc")},
		},
		{
			name:      "graph",
			unit:      "vpc",
			args:      []string{"--" + shared.GraphFlagName},
			wantUnits: []string{filepath.Join(memRoot, "app"), filepath.Join(memRoot, "vpc")},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			rec := &recorder{}
			v := rec.venv(t, units)

			opts := newOpts(t, v, tc.unit, tf.CommandNamePlan)
			opts.GraphRoot = memRoot

			_, err := runCommand(t, runcommand.NewCommand(logger.CreateLogger(), opts, v), tc.args...)
			require.NoError(t, err)
			assert.Equal(t, tc.wantUnits, rec.unitsRun(tf.CommandNamePlan))
			assert.Equal(t, tc.wantUnits, rec.unitsRun(tf.CommandNameInit))

			for _, unit := range tc.wantUnits {
				assert.Equal(
					t,
					[]string{tf.CommandNameInit, tf.CommandNamePlan},
					rec.commandsIn(unit),
					"%s should run init before plan",
					unit,
				)
			}
		})
	}
}
