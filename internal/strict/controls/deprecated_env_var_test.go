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

func TestNewDeprecatedEnvVar(t *testing.T) {
	t.Parallel()

	t.Run("with newValue", func(t *testing.T) {
		t.Parallel()

		deprecated := &clihelper.GenericFlag[string]{Name: "old", EnvVars: []string{"OLD_ENV"}}
		newer := &clihelper.GenericFlag[string]{Name: "new", EnvVars: []string{"NEW_ENV"}}

		ctrl := controls.NewDeprecatedEnvVar(deprecated, newer, "value")

		assert.Equal(t, "OLD_ENV", ctrl.Name)
		assert.Equal(t, "replaced with: NEW_ENV=value", ctrl.Description)
	})

	t.Run("without newValue", func(t *testing.T) {
		t.Parallel()

		deprecated := &clihelper.GenericFlag[string]{Name: "old", EnvVars: []string{"OLD_ENV"}}
		newer := &clihelper.GenericFlag[string]{Name: "new", EnvVars: []string{"NEW_ENV"}}

		ctrl := controls.NewDeprecatedEnvVar(deprecated, newer, "")

		assert.Equal(t, "OLD_ENV", ctrl.Name)
		assert.Equal(t, "replaced with: NEW_ENV", ctrl.Description)
	})
}

// applyEnvVarFlag wires a fake env lookup into the provided flag and applies
// it to a fresh flag set, so flag.Value().IsEnvSet() / GetName() reflect the
// provided env value.
func applyEnvVarFlag[T clihelper.GenericType](t *testing.T, flag *clihelper.GenericFlag[T], envs map[string]string) {
	t.Helper()

	flag.LookupEnvFunc = func(key string) []string {
		if val, ok := envs[key]; ok {
			return []string{val}
		}

		return nil
	}

	set := libflag.NewFlagSet("test", libflag.ContinueOnError)
	set.SetOutput(io.Discard)
	require.NoError(t, flag.Apply(set))
}

func applyEnvVarBoolFlag(t *testing.T, flag *clihelper.BoolFlag, envs map[string]string) {
	t.Helper()

	flag.LookupEnvFunc = func(key string) []string {
		if val, ok := envs[key]; ok {
			return []string{val}
		}

		return nil
	}

	set := libflag.NewFlagSet("test", libflag.ContinueOnError)
	set.SetOutput(io.Discard)
	require.NoError(t, flag.Apply(set))
}

func TestDeprecatedEnvVarEvaluateReturnsNilWhenNotEnvSet(t *testing.T) {
	t.Parallel()

	deprecated := &clihelper.GenericFlag[string]{Name: "old", EnvVars: []string{"OLD_ENV"}}
	newer := &clihelper.GenericFlag[string]{Name: "new", EnvVars: []string{"NEW_ENV"}}

	applyEnvVarFlag(t, deprecated, nil)
	applyEnvVarFlag(t, newer, nil)

	ctrl := controls.NewDeprecatedEnvVar(deprecated, newer, "")
	require.NoError(t, ctrl.Evaluate(t.Context()))
}

func TestDeprecatedEnvVarEvaluateReturnsNilWhenNameNotInDeprecatedEnvVars(t *testing.T) {
	t.Parallel()

	deprecated := &clihelper.GenericFlag[string]{
		Name:    "old",
		EnvVars: []string{"OLD_ENV"},
	}
	newer := &clihelper.GenericFlag[string]{Name: "new", EnvVars: []string{"NEW_ENV"}}

	// Apply with the env value present so envHasBeenSet=true, then parse a CLI
	// arg so flag.name is overwritten to "old" (the flag name, not env var).
	// This makes IsEnvSet=true while GetName() returns "old", which is not in
	// GetEnvVars(), exercising the slices.Contains branch.
	deprecated.LookupEnvFunc = func(key string) []string {
		if key == "OLD_ENV" {
			return []string{"envValue"}
		}

		return nil
	}
	set := libflag.NewFlagSet("test", libflag.ContinueOnError)
	set.SetOutput(io.Discard)
	require.NoError(t, deprecated.Apply(set))
	require.NoError(t, set.Parse([]string{"--old", "argValue"}))

	applyEnvVarFlag(t, newer, nil)

	ctrl := controls.NewDeprecatedEnvVar(deprecated, newer, "")
	require.NoError(t, ctrl.Evaluate(t.Context()))
}

func TestDeprecatedEnvVarEvaluateEnabledActiveReturnsError(t *testing.T) {
	t.Parallel()

	deprecated := &clihelper.GenericFlag[string]{Name: "old", EnvVars: []string{"OLD_ENV"}}
	newer := &clihelper.GenericFlag[string]{Name: "new", EnvVars: []string{"NEW_ENV"}}

	applyEnvVarFlag(t, deprecated, map[string]string{"OLD_ENV": "value"})
	applyEnvVarFlag(t, newer, nil)

	ctrl := controls.NewDeprecatedEnvVar(deprecated, newer, "")
	ctrl.Control.Enabled = true
	ctrl.Control.Status = strict.ActiveStatus

	err := ctrl.Evaluate(t.Context())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "OLD_ENV")
}

func TestDeprecatedEnvVarEvaluateEnabledCompletedReturnsNil(t *testing.T) {
	t.Parallel()

	deprecated := &clihelper.GenericFlag[string]{Name: "old", EnvVars: []string{"OLD_ENV"}}
	newer := &clihelper.GenericFlag[string]{Name: "new", EnvVars: []string{"NEW_ENV"}}

	applyEnvVarFlag(t, deprecated, map[string]string{"OLD_ENV": "value"})
	applyEnvVarFlag(t, newer, nil)

	ctrl := controls.NewDeprecatedEnvVar(deprecated, newer, "")
	ctrl.Control.Enabled = true
	ctrl.Control.Status = strict.CompletedStatus

	require.NoError(t, ctrl.Evaluate(t.Context()))
}

func TestDeprecatedEnvVarEvaluateEnabledEmptyErrorFmtReturnsNil(t *testing.T) {
	t.Parallel()

	deprecated := &clihelper.GenericFlag[string]{Name: "old", EnvVars: []string{"OLD_ENV"}}
	newer := &clihelper.GenericFlag[string]{Name: "new", EnvVars: []string{"NEW_ENV"}}

	applyEnvVarFlag(t, deprecated, map[string]string{"OLD_ENV": "value"})
	applyEnvVarFlag(t, newer, nil)

	ctrl := controls.NewDeprecatedEnvVar(deprecated, newer, "")
	ctrl.Control.Enabled = true
	ctrl.Control.Status = strict.ActiveStatus
	ctrl.ErrorFmt = ""

	require.NoError(t, ctrl.Evaluate(t.Context()))
}

func TestDeprecatedEnvVarEvaluateDisabledWithLogger(t *testing.T) {
	t.Parallel()

	l := logger.CreateLogger()
	ctx := log.ContextWithLogger(t.Context(), l)

	deprecated := &clihelper.GenericFlag[string]{Name: "old", EnvVars: []string{"OLD_ENV"}}
	newer := &clihelper.GenericFlag[string]{Name: "new", EnvVars: []string{"NEW_ENV"}}

	applyEnvVarFlag(t, deprecated, map[string]string{"OLD_ENV": "value"})
	applyEnvVarFlag(t, newer, nil)

	ctrl := controls.NewDeprecatedEnvVar(deprecated, newer, "")
	require.NoError(t, ctrl.Evaluate(ctx))
}

func TestDeprecatedEnvVarEvaluateDisabledWithSuppression(t *testing.T) {
	t.Parallel()

	l := logger.CreateLogger()
	ctx := log.ContextWithLogger(t.Context(), l)

	deprecated := &clihelper.GenericFlag[string]{Name: "old", EnvVars: []string{"OLD_ENV"}}
	newer := &clihelper.GenericFlag[string]{Name: "new", EnvVars: []string{"NEW_ENV"}}

	applyEnvVarFlag(t, deprecated, map[string]string{"OLD_ENV": "value"})
	applyEnvVarFlag(t, newer, nil)

	ctrl := controls.NewDeprecatedEnvVar(deprecated, newer, "")
	ctrl.SuppressWarning()
	require.NoError(t, ctrl.Evaluate(ctx))
}

func TestDeprecatedEnvVarEvaluateDisabledWithoutLogger(t *testing.T) {
	t.Parallel()

	deprecated := &clihelper.GenericFlag[string]{Name: "old", EnvVars: []string{"OLD_ENV"}}
	newer := &clihelper.GenericFlag[string]{Name: "new", EnvVars: []string{"NEW_ENV"}}

	applyEnvVarFlag(t, deprecated, map[string]string{"OLD_ENV": "value"})
	applyEnvVarFlag(t, newer, nil)

	ctrl := controls.NewDeprecatedEnvVar(deprecated, newer, "")
	require.NoError(t, ctrl.Evaluate(t.Context()))
}

func TestDeprecatedEnvVarEvaluateNewFlagWithoutEnvVars(t *testing.T) {
	t.Parallel()

	deprecated := &clihelper.GenericFlag[string]{Name: "old", EnvVars: []string{"OLD_ENV"}}
	newer := &clihelper.GenericFlag[string]{Name: "new"} // no env vars

	applyEnvVarFlag(t, deprecated, map[string]string{"OLD_ENV": "value"})
	applyEnvVarFlag(t, newer, nil)

	ctrl := controls.NewDeprecatedEnvVar(deprecated, newer, "")
	require.NoError(t, ctrl.Evaluate(t.Context()))
}

func TestDeprecatedEnvVarEvaluateFallsBackToDeprecatedValue(t *testing.T) {
	t.Parallel()

	deprecated := &clihelper.GenericFlag[string]{Name: "old", EnvVars: []string{"OLD_ENV"}}
	newer := &clihelper.GenericFlag[string]{Name: "new", EnvVars: []string{"NEW_ENV"}}

	applyEnvVarFlag(t, deprecated, map[string]string{"OLD_ENV": "deprecatedVal"})
	applyEnvVarFlag(t, newer, nil) // newer has no value -> falls back

	ctrl := controls.NewDeprecatedEnvVar(deprecated, newer, "")
	ctrl.Control.Enabled = true
	ctrl.Control.Status = strict.ActiveStatus

	err := ctrl.Evaluate(t.Context())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "deprecatedVal")
}

func TestDeprecatedEnvVarEvaluateNegativeBoolFlag(t *testing.T) {
	t.Parallel()

	deprecated := &clihelper.GenericFlag[string]{Name: "old", EnvVars: []string{"OLD_ENV"}}
	newer := &clihelper.BoolFlag{Name: "new", EnvVars: []string{"NEW_ENV"}, Negative: true}

	applyEnvVarFlag(t, deprecated, map[string]string{"OLD_ENV": "deprecatedVal"})
	applyEnvVarBoolFlag(t, newer, map[string]string{"NEW_ENV": "true"})

	ctrl := controls.NewDeprecatedEnvVar(deprecated, newer, "")
	ctrl.Control.Enabled = true
	ctrl.Control.Status = strict.ActiveStatus

	err := ctrl.Evaluate(t.Context())
	require.Error(t, err)
	// Negative bool with raw value=false (Negative inverts on Set) → strconv.FormatBool(!false) = "true"
	assert.Contains(t, err.Error(), "NEW_ENV=true")
}
