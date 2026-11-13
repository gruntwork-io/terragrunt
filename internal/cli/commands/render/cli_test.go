package render_test

import (
	"io"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/cli/commands/render"
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

// TestNewCommandExposesTheRenderFlags pins the flags users pass to `render`.
func TestNewCommandExposesTheRenderFlags(t *testing.T) {
	t.Parallel()

	cmd := newCommand()

	assert.Equal(t, render.CommandName, cmd.Name)

	for _, name := range []string{
		render.FormatFlagName,
		render.JSONFlagName,
		render.WriteFlagName,
		render.OutFlagName,
		render.WithMetadataFlagName,
		shared.AllFlagName,
		shared.FailFastFlagName,
		shared.FilterFlagName,
		shared.FilterAffectedFlagName,
		shared.DiscoveryBoundaryFlagName,
		shared.QueueExcludeDirFlagName,
		shared.QueueIncludeDirFlagName,
		shared.QueueExcludesFileFlagName,
		shared.UnitsThatIncludeFlagName,
		shared.ConfigFlagName,
		shared.DownloadDirFlagName,
		shared.TFPathFlagName,
		shared.SourceFlagName,
		shared.SourceMapFlagName,
		shared.FeatureFlagName,
		shared.IAMAssumeRoleFlagName,
		shared.AuthProviderCmdFlagName,
		shared.NoDependencyOutputsFlagName,
		shared.NoDependencyFetchOutputFromStateFlagName,
		shared.UsePartialParseConfigCacheFlagName,
		shared.NoAutoInitFlagName,
		shared.ProviderCacheFlagName,
		shared.BackendBootstrapFlagName,
	} {
		assert.NotNil(t, cmd.Flags.Get(name), name)
	}
}

// TestNewCommandRejectsFlagsRenderDoesNotRead pins that a `run` flag with no effect on a render is an error.
func TestNewCommandRejectsFlagsRenderDoesNotRead(t *testing.T) {
	t.Parallel()

	for _, arg := range []string{
		"--" + shared.OutDirFlagName + "=plans",
		"--" + shared.ReportFileFlagName + "=report.json",
		"--" + shared.SummaryDisableFlagName,
		"--" + shared.QueueIgnoreErrorsFlagName,
		"--" + shared.NoAutoApproveFlagName,
		"--" + shared.NoAutoRetryFlagName,
		"--" + shared.NoRunHooksFlagName,
		"--" + shared.NoStackGenerateFlagName,
		"--" + shared.SourceUpdateFlagName,
		"--" + shared.InputsDebugFlagName,
		"--" + shared.ParallelismFlagName + "=1",
		"--" + shared.DestroyDependenciesCheckFlagName,
		"--graph-root=.",
	} {
		t.Run(arg, func(t *testing.T) {
			t.Parallel()

			err := runCommand(t, newCommand(), arg)

			require.ErrorAs(t, err, new(clihelper.UndefinedFlagError))
		})
	}
}

func newCommand() *clihelper.Command {
	v := venv.MemVenv()

	return render.NewCommand(logger.CreateLogger(), options.NewTerragruntOptions(v.Exec), v, new(rewrite.Store))
}

// runCommand runs cmd with args the way the app does and returns its error.
func runCommand(t *testing.T, cmd *clihelper.Command, args ...string) error {
	t.Helper()

	app := clihelper.NewApp(map[string]string{})
	app.Writer = io.Discard
	app.ExitErrHandler = func(_ *clihelper.Context, err error) error { return err }

	return cmd.Run(config.WithCaches(t.Context()), clihelper.NewAppContext(app, args), args)
}
