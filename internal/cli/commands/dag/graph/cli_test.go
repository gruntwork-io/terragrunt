package graph_test

import (
	"io"
	"path/filepath"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/cli/commands/dag/graph"
	"github.com/gruntwork-io/terragrunt/internal/cli/flags/shared"
	"github.com/gruntwork-io/terragrunt/internal/vexec"
	"github.com/gruntwork-io/terragrunt/pkg/options"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/gruntwork-io/terragrunt/test/helpers/venvtest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Run a benchmark on runGraphDependencies for all fixtures possible.
// This should reveal regression on execution time due to new, changed or removed features.
func BenchmarkRunGraphDependencies(b *testing.B) {
	// Setup
	b.StopTimer()

	testDir := "../../../../../test/fixtures"

	fixtureDirs := []struct {
		description          string
		workingDir           string
		usePartialParseCache bool
	}{
		{
			"PartialParseBenchmarkRegressionCaching",
			"regressions/benchmark-parsing/production/deployment-group-1/webserver/terragrunt.hcl",
			true,
		},
		{
			"PartialParseBenchmarkRegressionNoCache",
			"regressions/benchmark-parsing/production/deployment-group-1/webserver/terragrunt.hcl",
			false,
		},
		{
			"PartialParseBenchmarkRegressionIncludesCaching",
			"regressions/benchmark-parsing-includes/production/deployment-group-1/webserver/terragrunt.hcl",
			true,
		},
		{
			"PartialParseBenchmarkRegressionIncludesNoCache",
			"regressions/benchmark-parsing-includes/production/deployment-group-1/webserver/terragrunt.hcl",
			false,
		},
	}

	// Run benchmarks
	for _, fixture := range fixtureDirs {
		b.Run(fixture.description, func(b *testing.B) {
			workingDir, err := filepath.Abs(filepath.Join(testDir, fixture.workingDir))
			require.NoError(b, err)

			terragruntOptions, err := options.NewTerragruntOptionsForTest(workingDir)
			if fixture.usePartialParseCache {
				terragruntOptions.UsePartialParseConfigCache = true
			} else {
				terragruntOptions.UsePartialParseConfigCache = false
			}

			require.NoError(b, err)

			b.ResetTimer()
			b.StartTimer()

			err = graph.Run(
				b.Context(),
				logger.CreateLogger(),
				venvtest.NewOSWithEmptyEnv().WithWriter(io.Discard),
				terragruntOptions,
			)

			b.StopTimer()
			require.NoError(b, err)
		})
	}
}

// TestNewCommandExposesTheQueueFlags pins the queue flags users pass to
// `dag graph`. The command builds its flag set from the shared sets rather
// than declaring one, so a set dropped from that list disappears from the
// command without any signature changing.
func TestNewCommandExposesTheQueueFlags(t *testing.T) {
	t.Parallel()

	cmd := graph.NewCommand(
		logger.CreateLogger(), options.NewTerragruntOptions(vexec.NewOSExec()), venvtest.New(),
	)

	assert.Equal(t, graph.CommandName, cmd.Name)

	for _, name := range []string{
		shared.QueueIgnoreDAGOrderFlagName,
		shared.QueueIgnoreErrorsFlagName,
	} {
		assert.NotNil(t, cmd.Flags.Get(name), name)
	}
}
