package shared_test

import (
	"context"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/cli/flags/shared"
	"github.com/gruntwork-io/terragrunt/internal/clihelper"
	"github.com/gruntwork-io/terragrunt/internal/experiment"
	"github.com/gruntwork-io/terragrunt/pkg/options"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFilterBoundaryFlagRequiresExperiment(t *testing.T) {
	t.Parallel()

	opts := options.NewTerragruntOptions()
	flags := shared.NewFilterFlags(logger.CreateLogger(), opts)

	require.NoError(t, flags.Parse(clihelper.Args{"--filter-boundary", "."}))

	err := flags.RunActions(context.Background(), &clihelper.Context{})

	require.ErrorIs(t, err, shared.ErrFilterBoundaryRequiresExperiment)
}

func TestFilterBoundaryFlagAllowedWithExperiment(t *testing.T) {
	t.Parallel()

	opts := options.NewTerragruntOptions()
	require.NoError(t, opts.Experiments.EnableExperiment(experiment.BoundedFilter))
	flags := shared.NewFilterFlags(logger.CreateLogger(), opts)

	require.NoError(t, flags.Parse(clihelper.Args{"--filter-boundary", "."}))
	require.NoError(t, flags.RunActions(context.Background(), &clihelper.Context{}))
	assert.Equal(t, ".", opts.FilterBoundary)
}
