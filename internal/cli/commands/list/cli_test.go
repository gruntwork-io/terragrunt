package list_test

import (
	"strings"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/cli/commands/list"
	"github.com/gruntwork-io/terragrunt/internal/clihelper"
	"github.com/gruntwork-io/terragrunt/internal/strict/controls"
	"github.com/gruntwork-io/terragrunt/pkg/options"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/gruntwork-io/terragrunt/test/helpers/venvtest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// listCLIUnits is the CLI fixture: alpha depends on zulu, and zulu is excluded from plan.
var listCLIUnits = map[string]string{
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

// TestNewCommandExposesTheListFlags pins the command name, alias and flags users invoke.
func TestNewCommandExposesTheListFlags(t *testing.T) {
	t.Parallel()

	cmd := list.NewCommand(
		logger.CreateLogger(), options.NewTerragruntOptions(), venvtest.New(),
	)

	assert.Equal(t, list.CommandName, cmd.Name)
	assert.Equal(t, []string{list.CommandAlias}, cmd.Aliases)

	flagNames := []string{
		list.FormatFlagName,
		list.TreeFlagName,
		list.LongFlagName,
		list.DAGFlagName,
		list.HiddenFlagName,
		list.NoHiddenFlagName,
		list.DependenciesFlagName,
		list.ExternalFlagName,
		list.QueueConstructAsFlagName,
	}

	for _, name := range flagNames {
		assert.NotNil(t, cmd.Flags.Get(name), name)
	}
}

// TestNewCommandBeforeResolvesTheFormat pins the flag aliases that Before folds into Format.
func TestNewCommandBeforeResolvesTheFormat(t *testing.T) {
	t.Parallel()

	longOutput := "Type  Path\nunit  alpha\nunit  zulu\n"

	testCases := []struct {
		name       string
		wantOutput string
		args       []string
		wantErr    bool
	}{
		{
			name:       "long flag",
			args:       []string{"--" + list.LongFlagName},
			wantOutput: longOutput,
		},
		{
			name:       "long alias",
			args:       []string{"-" + list.LongFlagAlias},
			wantOutput: longOutput,
		},
		{
			name:       "format flag",
			args:       []string{"--" + list.FormatFlagName, list.FormatLong},
			wantOutput: longOutput,
		},
		{
			name:       "dot format",
			args:       []string{"--" + list.FormatFlagName, list.FormatDot},
			wantOutput: "digraph {\n\t\"alpha\" ;\n\t\"zulu\" ;\n}\n",
		},
		{
			name:    "unknown format is rejected before discovery",
			args:    []string{"--" + list.FormatFlagName, "yaml"},
			wantErr: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var buf strings.Builder

			cmd := newListCommand(t, &buf)
			require.NoError(t, cmd.Flags.Parse(clihelper.Args(tc.args)))

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

// TestNewCommandBeforeResolvesTheMode pins the flags that Before folds into Mode.
func TestNewCommandBeforeResolvesTheMode(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name      string
		args      []string
		wantPaths []string
	}{
		{
			name:      "no flags sort alphabetically",
			wantPaths: []string{"alpha", "zulu"},
		},
		{
			name:      "dag flag sorts in dependency order",
			args:      []string{"--" + list.DAGFlagName},
			wantPaths: []string{"zulu", "alpha"},
		},
		{
			name:      "queue construct as implies dag mode and drops excluded units",
			args:      []string{"--" + list.QueueConstructAsFlagName, "plan"},
			wantPaths: []string{"alpha"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var buf strings.Builder

			cmd := newListCommand(t, &buf)
			require.NoError(t, cmd.Flags.Parse(clihelper.Args(tc.args)))
			require.NoError(t, cmd.Before(t.Context(), &clihelper.Context{}))
			require.NoError(t, cmd.Action(t.Context(), &clihelper.Context{}))
			assert.Equal(t, tc.wantPaths, strings.Fields(buf.String()))
		})
	}
}

// TestNewCommandTreeFlagRendersATree pins that the --tree alias reaches the tree renderer.
func TestNewCommandTreeFlagRendersATree(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name string
		args []string
	}{
		{name: "tree flag", args: []string{"--" + list.TreeFlagName}},
		{name: "tree alias", args: []string{"-" + list.TreeFlagAlias}},
		{name: "format flag", args: []string{"--" + list.FormatFlagName, list.FormatTree}},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var buf strings.Builder

			cmd := newListCommand(t, &buf)
			require.NoError(t, cmd.Flags.Parse(clihelper.Args(tc.args)))
			require.NoError(t, cmd.Before(t.Context(), &clihelper.Context{}))
			require.NoError(t, cmd.Action(t.Context(), &clihelper.Context{}))
			assert.Equal(t, []string{".", "alpha", "zulu"}, treeLabels(buf.String()))
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
		{name: "set", args: []string{"--" + list.HiddenFlagName}},
		{
			name:       "set under the strict control",
			args:       []string{"--" + list.HiddenFlagName},
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

			flags := list.NewFlags(newTestLogger(t), list.NewOptions(tgOpts), nil)
			require.NoError(t, flags.Parse(clihelper.Args(tc.args)))

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
			args: []string{"--" + list.ExternalFlagName + "=false"},
		},
		{
			name:        "set adds the dependency filter",
			args:        []string{"--" + list.ExternalFlagName},
			wantFilters: 1,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			opts := list.NewOptions(options.NewTerragruntOptions())

			flags := list.NewFlags(newTestLogger(t), opts, nil)
			require.NoError(t, flags.Parse(clihelper.Args(tc.args)))
			require.NoError(t, flags.RunActions(t.Context(), &clihelper.Context{}))
			assert.Len(t, opts.Filters, tc.wantFilters)
		})
	}
}

// newListCommand returns the list command wired to the shared fixture and to w.
func newListCommand(t *testing.T, w *strings.Builder) *clihelper.Command {
	t.Helper()

	root := "/list-cli"
	v := venvtest.New().WithFS(newUnitsFS(t, root, listCLIUnits)).WithWriter(w)

	tgOpts := options.NewTerragruntOptions()
	tgOpts.WorkingDir = root
	tgOpts.RootWorkingDir = root

	return list.NewCommand(newTestLogger(t), tgOpts, v)
}
