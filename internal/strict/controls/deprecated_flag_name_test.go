package controls_test

import (
	libflag "flag"
	"io"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/clihelper"
	"github.com/gruntwork-io/terragrunt/internal/strict"
	"github.com/gruntwork-io/terragrunt/internal/strict/controls"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewDeprecatedFlagName(t *testing.T) {
	t.Parallel()

	t.Run("with newValue", func(t *testing.T) {
		t.Parallel()

		deprecated := &clihelper.GenericFlag[string]{Name: "old"}
		newer := &clihelper.GenericFlag[string]{Name: "new"}

		ctrl := controls.NewDeprecatedFlagName(deprecated, newer, "value")

		assert.Equal(t, "old", ctrl.Name)
		assert.Equal(t, "replaced with: new=value", ctrl.Description)
	})

	t.Run("without newValue", func(t *testing.T) {
		t.Parallel()

		deprecated := &clihelper.GenericFlag[string]{Name: "old"}
		newer := &clihelper.GenericFlag[string]{Name: "new"}

		ctrl := controls.NewDeprecatedFlagName(deprecated, newer, "")

		assert.Equal(t, "old", ctrl.Name)
		assert.Equal(t, "replaced with: new", ctrl.Description)
	})
}

func parseFlag[T clihelper.GenericType](t *testing.T, flag *clihelper.GenericFlag[T], args []string) {
	t.Helper()

	flag.LookupEnvFunc = func(key string) []string { return nil }

	set := libflag.NewFlagSet("test", libflag.ContinueOnError)
	set.SetOutput(io.Discard)
	require.NoError(t, flag.Apply(set))
	require.NoError(t, set.Parse(args))
}

func parseBoolFlag(t *testing.T, flag *clihelper.BoolFlag, args []string) {
	t.Helper()

	flag.LookupEnvFunc = func(key string) []string { return nil }

	set := libflag.NewFlagSet("test", libflag.ContinueOnError)
	set.SetOutput(io.Discard)
	require.NoError(t, flag.Apply(set))
	require.NoError(t, set.Parse(args))
}

func TestDeprecatedFlagNameEvaluateReturnsNilWhenNotArgSet(t *testing.T) {
	t.Parallel()

	deprecated := &clihelper.GenericFlag[string]{Name: "old"}
	newer := &clihelper.GenericFlag[string]{Name: "new"}

	parseFlag(t, deprecated, nil)
	parseFlag(t, newer, nil)

	ctrl := controls.NewDeprecatedFlagName(deprecated, newer, "")
	require.NoError(t, ctrl.Evaluate(t.Context()))
}

func TestDeprecatedFlagNameEvaluateEnabledActiveReturnsError(t *testing.T) {
	t.Parallel()

	deprecated := &clihelper.GenericFlag[string]{Name: "old"}
	newer := &clihelper.GenericFlag[string]{Name: "new"}

	parseFlag(t, deprecated, []string{"--old", "deprecatedVal"})
	parseFlag(t, newer, nil)

	ctrl := controls.NewDeprecatedFlagName(deprecated, newer, "")
	ctrl.Control.Enabled = true
	ctrl.Control.Status = strict.ActiveStatus

	err := ctrl.Evaluate(t.Context())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "--old")
}

func TestDeprecatedFlagNameEvaluateEnabledCompletedReturnsNil(t *testing.T) {
	t.Parallel()

	deprecated := &clihelper.GenericFlag[string]{Name: "old"}
	newer := &clihelper.GenericFlag[string]{Name: "new"}

	parseFlag(t, deprecated, []string{"--old", "val"})
	parseFlag(t, newer, nil)

	ctrl := controls.NewDeprecatedFlagName(deprecated, newer, "")
	ctrl.Control.Enabled = true
	ctrl.Control.Status = strict.CompletedStatus

	require.NoError(t, ctrl.Evaluate(t.Context()))
}

func TestDeprecatedFlagNameEvaluateEnabledEmptyErrorFmtReturnsNil(t *testing.T) {
	t.Parallel()

	deprecated := &clihelper.GenericFlag[string]{Name: "old"}
	newer := &clihelper.GenericFlag[string]{Name: "new"}

	parseFlag(t, deprecated, []string{"--old", "val"})
	parseFlag(t, newer, nil)

	ctrl := controls.NewDeprecatedFlagName(deprecated, newer, "")
	ctrl.Control.Enabled = true
	ctrl.Control.Status = strict.ActiveStatus
	ctrl.ErrorFmt = ""

	require.NoError(t, ctrl.Evaluate(t.Context()))
}

func TestDeprecatedFlagNameEvaluateDisabledWithLogger(t *testing.T) {
	t.Parallel()

	l := logger.CreateLogger()
	ctx := log.ContextWithLogger(t.Context(), l)

	deprecated := &clihelper.GenericFlag[string]{Name: "old"}
	newer := &clihelper.GenericFlag[string]{Name: "new"}

	parseFlag(t, deprecated, []string{"--old", "val"})
	parseFlag(t, newer, nil)

	ctrl := controls.NewDeprecatedFlagName(deprecated, newer, "")
	require.NoError(t, ctrl.Evaluate(ctx))
}

func TestDeprecatedFlagNameEvaluateDisabledWithSuppression(t *testing.T) {
	t.Parallel()

	l := logger.CreateLogger()
	ctx := log.ContextWithLogger(t.Context(), l)

	deprecated := &clihelper.GenericFlag[string]{Name: "old"}
	newer := &clihelper.GenericFlag[string]{Name: "new"}

	parseFlag(t, deprecated, []string{"--old", "val"})
	parseFlag(t, newer, nil)

	ctrl := controls.NewDeprecatedFlagName(deprecated, newer, "")
	ctrl.SuppressWarning()
	require.NoError(t, ctrl.Evaluate(ctx))
}

func TestDeprecatedFlagNameEvaluateDisabledWithoutLogger(t *testing.T) {
	t.Parallel()

	deprecated := &clihelper.GenericFlag[string]{Name: "old"}
	newer := &clihelper.GenericFlag[string]{Name: "new"}

	parseFlag(t, deprecated, []string{"--old", "val"})
	parseFlag(t, newer, nil)

	ctrl := controls.NewDeprecatedFlagName(deprecated, newer, "")
	require.NoError(t, ctrl.Evaluate(t.Context()))
}

func TestDeprecatedFlagNameEvaluateNewFlagWithoutNames(t *testing.T) {
	t.Parallel()

	deprecated := &clihelper.GenericFlag[string]{Name: "old"}
	newer := &clihelper.GenericFlag[string]{} // no Name -> Names() empty

	parseFlag(t, deprecated, []string{"--old", "val"})
	parseFlag(t, newer, nil)

	ctrl := controls.NewDeprecatedFlagName(deprecated, newer, "")
	require.NoError(t, ctrl.Evaluate(t.Context()))
}

func TestDeprecatedFlagNameEvaluateFallsBackToDeprecatedValue(t *testing.T) {
	t.Parallel()

	deprecated := &clihelper.GenericFlag[string]{Name: "old"}
	newer := &clihelper.GenericFlag[string]{Name: "new"}

	parseFlag(t, deprecated, []string{"--old", "deprecatedVal"})
	parseFlag(t, newer, nil)

	ctrl := controls.NewDeprecatedFlagName(deprecated, newer, "")
	ctrl.Control.Enabled = true
	ctrl.Control.Status = strict.ActiveStatus

	err := ctrl.Evaluate(t.Context())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "deprecatedVal")
}

func TestDeprecatedFlagNameEvaluateNewFlagDoesNotTakeValue(t *testing.T) {
	t.Parallel()

	deprecated := &clihelper.GenericFlag[string]{Name: "old"}
	newer := &clihelper.BoolFlag{Name: "new"} // BoolFlag.TakesValue() = false

	parseFlag(t, deprecated, []string{"--old", "val"})
	parseBoolFlag(t, newer, nil)

	ctrl := controls.NewDeprecatedFlagName(deprecated, newer, "")
	ctrl.Control.Enabled = true
	ctrl.Control.Status = strict.ActiveStatus

	err := ctrl.Evaluate(t.Context())
	require.Error(t, err)
	// Without TakesValue, no "=value" is appended -> error mentions just `new`.
	assert.Contains(t, err.Error(), "`--new`")
}
