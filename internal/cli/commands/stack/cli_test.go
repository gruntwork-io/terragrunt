package stack_test

import (
	"io"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/cli/commands/stack"
	"github.com/gruntwork-io/terragrunt/internal/cli/flags/shared"
	"github.com/gruntwork-io/terragrunt/internal/clihelper"
	rewrite "github.com/gruntwork-io/terragrunt/internal/config/rewrite"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/pkg/config"
	"github.com/gruntwork-io/terragrunt/pkg/options"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// configParsingFlagNames names one flag from each group that every stack command with flags passes to config parsing.
var configParsingFlagNames = []string{
	shared.ConfigFlagName,
	shared.DownloadDirFlagName,
	shared.TFPathFlagName,
	shared.SourceFlagName,
	shared.SourceMapFlagName,
	shared.FeatureFlagName,
	shared.IAMAssumeRoleFlagName,
	shared.AuthProviderCmdFlagName,
	shared.NoDependencyOutputsFlagName,
	shared.InputsDebugFlagName,
	shared.UsePartialParseConfigCacheFlagName,
	shared.ProviderCacheFlagName,
	shared.BackendBootstrapFlagName,
	shared.NoCASFlagName,
}

// selectionFlagNames names the flags that select the stacks and units a stack command acts on.
var selectionFlagNames = []string{
	shared.FilterFlagName,
	shared.FilterAffectedFlagName,
	shared.DiscoveryBoundaryFlagName,
	shared.QueueExcludeDirFlagName,
	shared.QueueIncludeDirFlagName,
	shared.QueueExcludesFileFlagName,
	shared.UnitsThatIncludeFlagName,
}

// TestNewCommandExposesTheStackFlags pins the flags users pass to each stack subcommand.
func TestNewCommandExposesTheStackFlags(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		subcommand string
		flagNames  []string
	}{
		{
			subcommand: "generate",
			flagNames: []string{
				stack.NoStackValidate,
				shared.NoStackGenerateFlagName,
				shared.SourceUpdateFlagName,
				shared.ParallelismFlagName,
			},
		},
		{
			subcommand: "run",
			flagNames: []string{
				stack.NoStackValidate,
				shared.NoStackGenerateFlagName,
				shared.SourceUpdateFlagName,
				shared.ParallelismFlagName,
				shared.FailFastFlagName,
				shared.NoAutoApproveFlagName,
				shared.NoAutoRetryFlagName,
				shared.NoRunHooksFlagName,
				shared.OutDirFlagName,
				shared.JSONOutDirFlagName,
				shared.ReportFileFlagName,
				shared.SummaryDisableFlagName,
				shared.QueueIgnoreErrorsFlagName,
				shared.QueueIgnoreDAGOrderFlagName,
				shared.DestroyDependenciesCheckFlagName,
				shared.FilterAllowDestroyFlagName,
			},
		},
		{
			subcommand: "output",
			flagNames: []string{
				stack.NoStackValidate,
				stack.OutputFormatFlagName,
				stack.JSONFormatFlagName,
				stack.RawFormatFlagName,
				shared.ParallelismFlagName,
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.subcommand, func(t *testing.T) {
			t.Parallel()

			subcommand := newCommand().Subcommand(tc.subcommand)
			require.NotNil(t, subcommand)

			for _, names := range [][]string{tc.flagNames, configParsingFlagNames, selectionFlagNames} {
				for _, name := range names {
					assert.NotNil(t, subcommand.Flags.Get(name), name)
				}
			}
		})
	}
}

// TestNewCommandCleanTakesNoFlags pins that `stack clean` defines no flags of its own.
func TestNewCommandCleanTakesNoFlags(t *testing.T) {
	t.Parallel()

	subcommand := newCommand().Subcommand("clean")
	require.NotNil(t, subcommand)

	assert.Empty(t, subcommand.Flags)
}

// TestNewCommandRejectsFlagsTheSubcommandDoesNotRead pins that a `run` flag a stack subcommand ignores is an error.
func TestNewCommandRejectsFlagsTheSubcommandDoesNotRead(t *testing.T) {
	t.Parallel()

	runOnlyArgs := []string{
		"--" + shared.OutDirFlagName + "=plans",
		"--" + shared.ReportFileFlagName + "=report.json",
		"--" + shared.SummaryDisableFlagName,
		"--" + shared.QueueIgnoreErrorsFlagName,
		"--" + shared.QueueIgnoreDAGOrderFlagName,
		"--" + shared.NoAutoApproveFlagName,
		"--" + shared.NoAutoRetryFlagName,
		"--" + shared.NoRunHooksFlagName,
		"--" + shared.FailFastFlagName,
		"--" + shared.DestroyDependenciesCheckFlagName,
		"--graph-root=.",
	}

	testCases := []struct {
		subcommand string
		args       []string
	}{
		{subcommand: "generate", args: runOnlyArgs},
		{
			subcommand: "output",
			args: append(
				[]string{"--" + shared.NoStackGenerateFlagName, "--" + shared.SourceUpdateFlagName},
				runOnlyArgs...,
			),
		},
		{subcommand: "run", args: []string{"--graph-root=."}},
	}

	for _, tc := range testCases {
		for _, arg := range tc.args {
			t.Run(tc.subcommand+" "+arg, func(t *testing.T) {
				t.Parallel()

				err := runCommand(t, newCommand(), tc.subcommand, arg)

				require.ErrorAs(t, err, new(clihelper.UndefinedFlagError))
			})
		}
	}
}

func newCommand() *clihelper.Command {
	v := venv.MemVenv()

	return stack.NewCommand(logger.CreateLogger(), options.NewTerragruntOptions(v.Exec), v, new(rewrite.Store))
}

// runCommand runs cmd with args the way the app does and returns its error.
func runCommand(t *testing.T, cmd *clihelper.Command, args ...string) error {
	t.Helper()

	app := clihelper.NewApp(map[string]string{})
	app.Writer = io.Discard
	app.ExitErrHandler = func(_ *clihelper.Context, err error) error { return err }

	return cmd.Run(config.WithCaches(t.Context()), clihelper.NewAppContext(app, args), args)
}
