package engine_test

import (
	"context"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/engine"
	"github.com/gruntwork-io/terragrunt/internal/experiment"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/stretchr/testify/require"
)

func iacEngineEnabled(t *testing.T) experiment.Experiments {
	t.Helper()

	exps := experiment.NewExperiments()
	require.NoError(t, exps.EnableExperiment(experiment.IacEngine))

	return exps
}

// TestShutdownUnit_NoOpWhenGated asserts ShutdownUnit returns nil without touching the
// registry when the iac-engine experiment is disabled or NoEngine is set.
func TestShutdownUnit_NoOpWhenGated(t *testing.T) {
	t.Parallel()

	ctx := engine.WithEngineValues(context.Background())

	const unitDir = "/repo/unit"

	require.NoError(
		t,
		engine.ShutdownUnit(ctx, log.New(), experiment.NewExperiments(), false, unitDir),
		"a disabled iac-engine experiment must be a no-op",
	)
	require.NoError(t, engine.ShutdownUnit(ctx, log.New(), iacEngineEnabled(t), true, unitDir),
		"NoEngine must be a no-op")
}

// TestShutdownUnit_NoMatchingEngineIsNoOp asserts that, with the experiment enabled and
// no engine registered for the unit, ShutdownUnit is a safe no-op rather than erroring.
func TestShutdownUnit_NoMatchingEngineIsNoOp(t *testing.T) {
	t.Parallel()

	ctx := engine.WithEngineValues(context.Background())

	require.NoError(
		t,
		engine.ShutdownUnit(
			ctx,
			log.New(),
			iacEngineEnabled(t),
			false,
			"/repo/unit-without-engine",
		),
	)
}
