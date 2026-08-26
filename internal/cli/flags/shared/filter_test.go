package shared_test

import (
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/cli/flags/shared"
	"github.com/gruntwork-io/terragrunt/internal/clihelper"
	"github.com/gruntwork-io/terragrunt/internal/experiment"
	"github.com/gruntwork-io/terragrunt/internal/vexec"
	"github.com/gruntwork-io/terragrunt/pkg/options"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/gruntwork-io/terragrunt/test/helpers/venvtest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDiscoveryBoundaryFlagRequiresExperiment(t *testing.T) {
	t.Parallel()

	opts := options.NewTerragruntOptions(vexec.NewOSExec())
	flags := shared.NewFilterFlags(logger.CreateLogger(), opts, venvtest.New())

	require.NoError(t, flags.Parse(clihelper.Args{"--discovery-boundary", "."}, map[string]string{}))

	err := flags.RunActions(t.Context(), &clihelper.Context{})

	require.ErrorIs(t, err, shared.ErrDiscoveryBoundaryRequiresExperiment)
}

func TestDiscoveryBoundaryFlagAllowedWithExperiment(t *testing.T) {
	t.Parallel()

	opts := options.NewTerragruntOptions(vexec.NewOSExec())
	require.NoError(t, opts.Experiments.EnableExperiment(experiment.BoundedDiscovery))
	flags := shared.NewFilterFlags(logger.CreateLogger(), opts, venvtest.New())

	require.NoError(t, flags.Parse(clihelper.Args{"--discovery-boundary", "."}, map[string]string{}))
	require.NoError(t, flags.RunActions(t.Context(), &clihelper.Context{}))
	assert.Equal(t, ".", opts.DiscoveryBoundary)
}
