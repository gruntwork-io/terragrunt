package find_test

import (
	"strings"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/cli/commands/find"
	"github.com/gruntwork-io/terragrunt/internal/clihelper"
	"github.com/gruntwork-io/terragrunt/internal/strict/controls"
	"github.com/gruntwork-io/terragrunt/pkg/options"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/gruntwork-io/terragrunt/test/helpers/venvtest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// findCLIUnits is the CLI fixture: alpha depends on zulu, and zulu is excluded from plan.
var findCLIUnits = map[string]string{
	"alpha/terragrunt.hcl": `
dependency "zulu" {
  config_path = "../zulu"
}
`,
	"zulu/terragrunt.hcl": `
exclude {
  if      = true
  actions = ["plan"]
}
`,
}

// TestNewCommandExposesTheFindFlags pins the command name, alias and flags users invoke.
func TestNewCommandExposesTheFindFlags(t *testing.T) {
	t.Parallel()

	cmd := find.NewCommand(
		logger.CreateLogger(), options.NewTerragruntOptions(), venvtest.New(),
	)

	assert.Equal(t, find.CommandName, cmd.Name)
	assert.Equal(t, []string{find.CommandAlias}, cmd.Aliases)

	flagNames := []string{
		find.FormatFlagName,
		find.JSONFlagName,
		find.DAGFlagName,
		find.HiddenFlagName,
		find.NoHiddenFlagName,
		find.Dependencies,
		find.External,
		find.Exclude,
		find.Include,
		find.Reading,
		find.QueueConstructAsFlagName,
	}

	for _, name := range flagNames {
		assert.NotNil(t, cmd.Flags.Get(name), name)
	}
}

// TestNewCommandBeforeResolvesFormatAndMode pins the aliases Before folds into Format and Mode.
func TestNewCommandBeforeResolvesFormatAndMode(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name       string
		wantOutput string
		args       []string
		wantErr    bool
	}{
		{
			name:       "no flags sort alphabetically as text",
			wantOutput: "alpha\nzulu\n",
		},
		{
			name:       "dag flag sorts in dependency order",
			args:       []string{"--" + find.DAGFlagName},
			wantOutput: "zulu\nalpha\n",
		},
		{
			name: "json flag switches the format",
			args: []string{"--" + find.JSONFlagName},
			wantOutput: `[
  {
    "type": "unit",
    "path": "alpha"
  },
  {
    "type": "unit",
    "path": "zulu"
  }
]
`,
		},
		{
			name: "format flag switches the format",
			args: []string{"--" + find.FormatFlagName, find.FormatJSON},
			wantOutput: `[
  {
    "type": "unit",
    "path": "alpha"
  },
  {
    "type": "unit",
    "path": "zulu"
  }
]
`,
		},
		{
			name:       "queue construct as implies dag mode and drops excluded units",
			args:       []string{"--" + find.QueueConstructAsFlagName, "plan"},
			wantOutput: "alpha\n",
		},
		{
			name:    "unknown format is rejected before discovery",
			args:    []string{"--" + find.FormatFlagName, "yaml"},
			wantErr: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var buf strings.Builder

			root := "/find-cli"
			v := venvtest.New().WithFS(newUnitsFS(t, root, findCLIUnits)).WithWriter(&buf)

			tgOpts := options.NewTerragruntOptions()
			tgOpts.WorkingDir = root
			tgOpts.RootWorkingDir = root

			cmd := find.NewCommand(newTestLogger(t), tgOpts, v)
			require.NoError(t, cmd.Flags.Parse(clihelper.Args(tc.args), map[string]string{}))

			err := cmd.Before(t.Context(), &clihelper.Context{})
			if tc.wantErr {
				require.Error(t, err)

				var exitErr clihelper.ExitCoder

				require.ErrorAs(t, err, &exitErr)
				assert.Equal(t, int(clihelper.ExitCodeGeneralError), exitErr.ExitCode())

				return
			}

			require.NoError(t, err)
			require.NoError(t, cmd.Action(t.Context(), &clihelper.Context{}))
			assert.Equal(t, tc.wantOutput, buf.String())
		})
	}
}

// TestNewFlagsHiddenFlagRunsTheStrictControl pins that --hidden fails once its control is on.
func TestNewFlagsHiddenFlagRunsTheStrictControl(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name       string
		args       []string
		strictMode bool
		wantErr    bool
	}{
		{name: "unset", args: nil},
		{name: "set", args: []string{"--" + find.HiddenFlagName}},
		{
			name:       "set under the strict control",
			args:       []string{"--" + find.HiddenFlagName},
			strictMode: true,
			wantErr:    true,
		},
		{
			name:       "unset under the strict control",
			strictMode: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			tgOpts := options.NewTerragruntOptions()
			if tc.strictMode {
				require.NoError(
					t,
					tgOpts.StrictControls.EnableControl(controls.DeprecatedHiddenFlag),
				)
			}

			flags := find.NewFlags(newTestLogger(t), find.NewOptions(tgOpts), venvtest.New(), nil)
			require.NoError(t, flags.Parse(clihelper.Args(tc.args), map[string]string{}))

			err := flags.RunActions(t.Context(), &clihelper.Context{})
			if tc.wantErr {
				require.Error(t, err)

				return
			}

			require.NoError(t, err)
		})
	}
}

// TestNewFlagsExternalFlagAddsAGraphFilter pins that --external appends a dependency filter.
func TestNewFlagsExternalFlagAddsAGraphFilter(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name        string
		args        []string
		wantFilters int
	}{
		{name: "unset adds no filter"},
		{
			name: "explicitly disabled adds no filter",
			args: []string{"--" + find.External + "=false"},
		},
		{
			name:        "set adds the dependency filter",
			args:        []string{"--" + find.External},
			wantFilters: 1,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			opts := find.NewOptions(options.NewTerragruntOptions())

			flags := find.NewFlags(newTestLogger(t), opts, venvtest.New(), nil)
			require.NoError(t, flags.Parse(clihelper.Args(tc.args), map[string]string{}))
			require.NoError(t, flags.RunActions(t.Context(), &clihelper.Context{}))
			assert.Len(t, opts.Filters, tc.wantFilters)
		})
	}
}
