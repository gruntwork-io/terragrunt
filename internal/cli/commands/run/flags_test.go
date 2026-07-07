package run_test

import (
	"context"
	"testing"

	runcommand "github.com/gruntwork-io/terragrunt/internal/cli/commands/run"
	"github.com/gruntwork-io/terragrunt/internal/clihelper"
	"github.com/gruntwork-io/terragrunt/internal/experiment"
	"github.com/gruntwork-io/terragrunt/pkg/options"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNoHooksFlagRequiresExperiment(t *testing.T) {
	t.Parallel()

	opts := options.NewTerragruntOptions()
	flags := runcommand.NewFlags(logger.CreateLogger(), opts, nil)

	require.NoError(t, flags.Parse(clihelper.Args{"--no-hooks"}))

	err := flags.RunActions(context.Background(), &clihelper.Context{})

	require.ErrorIs(t, err, runcommand.ErrNoHooksRequiresExperiment)
	assert.True(t, opts.NoRunHooks)
}

func TestNoHooksFlagAllowedWithExperiment(t *testing.T) {
	t.Parallel()

	opts := options.NewTerragruntOptions()
	require.NoError(t, opts.Experiments.EnableExperiment(experiment.OptionalHooks))
	flags := runcommand.NewFlags(logger.CreateLogger(), opts, nil)

	require.NoError(t, flags.Parse(clihelper.Args{"--no-hooks"}))
	require.NoError(t, flags.RunActions(context.Background(), &clihelper.Context{}))
	assert.True(t, opts.NoRunHooks)
}

// TestDiscoveryBoundaryFlagParses asserts the shared --discovery-boundary flag is wired into
// `run` and populates opts.DiscoveryBoundary.
func TestDiscoveryBoundaryFlagParses(t *testing.T) {
	t.Parallel()

	opts := options.NewTerragruntOptions()
	flags := runcommand.NewFlags(logger.CreateLogger(), opts, nil)

	require.NoError(t, flags.Parse(clihelper.Args{"--discovery-boundary", "/some/boundary"}))
	assert.Equal(t, "/some/boundary", opts.DiscoveryBoundary)
}
