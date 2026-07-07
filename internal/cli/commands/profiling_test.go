package commands_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/cli/commands"
	"github.com/gruntwork-io/terragrunt/internal/clihelper"
	"github.com/gruntwork-io/terragrunt/internal/experiment"
	"github.com/gruntwork-io/terragrunt/pkg/options"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWrapWithProfilingRequiresExperiment(t *testing.T) {
	t.Parallel()

	opts := options.NewTerragruntOptions()
	opts.ProfileCPU = filepath.Join(t.TempDir(), "cpu.prof")

	called := false
	err := commands.WrapWithProfiling(logger.CreateLogger(), opts)(context.Background(), nil, func(_ context.Context, _ *clihelper.Context) error {
		called = true

		return nil
	})

	require.ErrorIs(t, err, commands.ErrProfilingRequiresExperiment)
	assert.False(t, called, "the wrapped action must not run when the experiment gate fails")
	assert.NoFileExists(t, opts.ProfileCPU)
}

func TestWrapWithProfilingNoFlagsRunsAction(t *testing.T) {
	t.Parallel()

	opts := options.NewTerragruntOptions()

	called := false
	err := commands.WrapWithProfiling(logger.CreateLogger(), opts)(context.Background(), nil, func(_ context.Context, _ *clihelper.Context) error {
		called = true

		return nil
	})

	require.NoError(t, err)
	assert.True(t, called)
}

func TestWrapWithProfilingWritesProfile(t *testing.T) {
	t.Parallel()

	opts := options.NewTerragruntOptions()
	opts.ProfileGoroutine = filepath.Join(t.TempDir(), "goroutine.prof")
	require.NoError(t, opts.Experiments.EnableExperiment(experiment.Profiling))

	err := commands.WrapWithProfiling(logger.CreateLogger(), opts)(context.Background(), nil, func(_ context.Context, _ *clihelper.Context) error {
		return nil
	})
	require.NoError(t, err)

	info, err := os.Stat(opts.ProfileGoroutine)
	require.NoError(t, err)
	assert.Positive(t, info.Size(), "goroutine profile should be non-empty")
}
