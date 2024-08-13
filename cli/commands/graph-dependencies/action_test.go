package graphdependencies_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	graphdependencies "github.com/gruntwork-io/terragrunt/cli/commands/graph-dependencies"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/stretchr/testify/require"
)

// Run a benchmark on runGraphDependencies for all fixtures possible.
// This should reveal regression on execution time due to new, changed or removed features.
func BenchmarkRunGraphDependencies(b *testing.B) {
	// Setup
	b.StopTimer()
	cwd, err := os.Getwd()
	require.NoError(b, err)

	testDir := "../../../test"

	fixtureDirs := []struct {
		description          string
		workingDir           string
		usePartialParseCache bool
	}{
		{"PartialParseBenchmarkRegressionCaching", "fixture-regressions/benchmark-parsing/production/deployment-group-1/webserver/terragrunt.hcl", true},
		{"PartialParseBenchmarkRegressionNoCache", "fixture-regressions/benchmark-parsing/production/deployment-group-1/webserver/terragrunt.hcl", false},
		{"PartialParseBenchmarkRegressionIncludesCaching", "fixture-regressions/benchmark-parsing-includes/production/deployment-group-1/webserver/terragrunt.hcl", true},
		{"PartialParseBenchmarkRegressionIncludesNoCache", "fixture-regressions/benchmark-parsing-includes/production/deployment-group-1/webserver/terragrunt.hcl", false},
	}

	// Run benchmarks
	for _, fixture := range fixtureDirs {
		b.Run(fixture.description, func(b *testing.B) {
			workingDir := filepath.Join(cwd, testDir, fixture.workingDir)
			terragruntOptions, err := options.NewTerragruntOptionsForTest(workingDir)
			if fixture.usePartialParseCache {
				terragruntOptions.UsePartialParseConfigCache = true
			} else {
				terragruntOptions.UsePartialParseConfigCache = false
			}
			require.NoError(b, err)

			b.ResetTimer()
			b.StartTimer()
			err = graphdependencies.Run(context.Background(), terragruntOptions)
			b.StopTimer()
			require.NoError(b, err)
		})
	}
}
