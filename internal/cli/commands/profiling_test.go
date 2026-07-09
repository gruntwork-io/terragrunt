package commands_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/cli/commands"
	"github.com/gruntwork-io/terragrunt/internal/clihelper"
	"github.com/gruntwork-io/terragrunt/internal/experiment"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/gruntwork-io/terragrunt/pkg/options"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/gruntwork-io/terragrunt/test/helpers/venvtest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWrapWithProfilingRequiresExperiment(t *testing.T) {
	t.Parallel()

	v := venvtest.New()

	opts := options.NewTerragruntOptions()
	opts.ProfileCPU = filepath.Join(t.TempDir(), "cpu.prof")

	called := false
	wrapped := commands.WrapWithProfiling(logger.CreateLogger(), opts, v)
	err := wrapped(t.Context(), &clihelper.Context{}, func(_ context.Context, _ *clihelper.Context) error {
		called = true

		return nil
	})

	require.ErrorIs(t, err, commands.ErrProfilingRequiresExperiment)
	assert.False(t, called, "the wrapped action must not run when the experiment gate fails")

	_, statErr := v.FS.Stat(opts.ProfileCPU)
	assert.True(t, os.IsNotExist(statErr), "profile file must not be created without the experiment")
}

func TestWrapWithProfilingNoFlagsRunsAction(t *testing.T) {
	t.Parallel()

	v := venvtest.New()

	opts := options.NewTerragruntOptions()

	called := false
	wrapped := commands.WrapWithProfiling(logger.CreateLogger(), opts, v)
	err := wrapped(t.Context(), &clihelper.Context{}, func(_ context.Context, _ *clihelper.Context) error {
		called = true

		return nil
	})

	require.NoError(t, err)
	assert.True(t, called)
}

func TestWrapWithProfilingWritesProfile(t *testing.T) {
	t.Parallel()

	v := venvtest.New()

	opts := options.NewTerragruntOptions()
	opts.ProfileGoroutine = filepath.Join(t.TempDir(), "goroutine.prof")
	require.NoError(t, opts.Experiments.EnableExperiment(experiment.Profiling))

	wrapped := commands.WrapWithProfiling(logger.CreateLogger(), opts, v)
	err := wrapped(t.Context(), &clihelper.Context{}, func(_ context.Context, _ *clihelper.Context) error {
		return nil
	})
	require.NoError(t, err)

	info, err := v.FS.Stat(opts.ProfileGoroutine)
	require.NoError(t, err)
	assert.Positive(t, info.Size(), "goroutine profile should be non-empty")
	assert.Equal(t, os.FileMode(0o600), info.Mode().Perm(), "profile files must be owner-only")
}

func TestWrapWithProfilingTightensExistingFilePermissions(t *testing.T) {
	t.Parallel()

	v := venvtest.New()

	opts := options.NewTerragruntOptions()
	opts.ProfileGoroutine = filepath.Join(t.TempDir(), "goroutine.prof")
	require.NoError(t, opts.Experiments.EnableExperiment(experiment.Profiling))

	require.NoError(t, vfs.WriteFile(v.FS, opts.ProfileGoroutine, []byte("stale"), 0o644))

	wrapped := commands.WrapWithProfiling(logger.CreateLogger(), opts, v)
	err := wrapped(t.Context(), &clihelper.Context{}, func(_ context.Context, _ *clihelper.Context) error {
		return nil
	})
	require.NoError(t, err)

	info, err := v.FS.Stat(opts.ProfileGoroutine)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), info.Mode().Perm(), "a pre-existing profile file must be tightened to owner-only")
}
