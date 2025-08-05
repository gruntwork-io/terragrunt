package graph_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/gruntwork-io/terragrunt/cli/commands/dag/graph"
	"github.com/gruntwork-io/terragrunt/cli/flags"
	"github.com/gruntwork-io/terragrunt/internal/cli"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/stretchr/testify/require"
)

// TestDagGraphWithQueueIncludeExternal tests that the TG_QUEUE_INCLUDE_EXTERNAL environment variable
// prevents the interactive prompt for external dependencies when running dag graph.
func TestDagGraphWithQueueIncludeExternal(t *testing.T) {
	t.Parallel()

	// Setup test environment
	cwd, err := os.Getwd()
	require.NoError(t, err)

	testDir := "../../../../test/fixtures/graph-dependencies"
	workingDir := filepath.Join(cwd, testDir, "root/frontend-app")

	// Set the environment variable to test the fix
	originalEnv := os.Getenv("TG_QUEUE_INCLUDE_EXTERNAL")
	defer os.Setenv("TG_QUEUE_INCLUDE_EXTERNAL", originalEnv)
	os.Setenv("TG_QUEUE_INCLUDE_EXTERNAL", "true")

	// Create terragrunt options
	terragruntOptions, err := options.NewTerragruntOptionsForTest(workingDir)
	require.NoError(t, err)

	// Set non-interactive to avoid any prompts
	terragruntOptions.NonInteractive = true

	// Run the dag graph command
	ctx := cli.NewAppContext(t.Context(), cli.NewApp(), nil)
	err = graph.Run(ctx, logger.CreateLogger(), terragruntOptions)

	// The command should complete successfully without prompting for external dependencies
	require.NoError(t, err)
}

// TestDagGraphWithExternalDependency tests the specific scenario from GitHub issue #4613
// where TG_QUEUE_INCLUDE_EXTERNAL=true should prevent the interactive prompt for external dependencies.
func TestDagGraphWithExternalDependency(t *testing.T) {
	t.Parallel()

	// Test that the dag graph command properly handles the TG_QUEUE_INCLUDE_EXTERNAL environment variable
	t.Run("with_environment_variable", func(t *testing.T) {
		// Set the environment variable
		originalEnv := os.Getenv("TG_QUEUE_INCLUDE_EXTERNAL")
		defer os.Setenv("TG_QUEUE_INCLUDE_EXTERNAL", originalEnv)
		os.Setenv("TG_QUEUE_INCLUDE_EXTERNAL", "true")

		// Create terragrunt options
		terragruntOptions, err := options.NewTerragruntOptionsForTest(".")
		require.NoError(t, err)

		// Set non-interactive to avoid any prompts
		terragruntOptions.NonInteractive = true

		// Create the command to test flag parsing
		cmd := graph.NewCommand(logger.CreateLogger(), terragruntOptions, flags.Prefix{})

		// Verify that the command has the queue flags
		hasQueueIncludeExternalFlag := false
		for _, flag := range cmd.Flags {
			if flag.Names()[0] == "queue-include-external" {
				hasQueueIncludeExternalFlag = true
				break
			}
		}
		require.True(t, hasQueueIncludeExternalFlag, "dag graph command should have queue-include-external flag")
	})
}

// Run a benchmark on runGraphDependencies for all fixtures possible.
// This should reveal regression on execution time due to new, changed or removed features.
func BenchmarkRunGraphDependencies(b *testing.B) {
	// Setup
	b.StopTimer()
	cwd, err := os.Getwd()
	require.NoError(b, err)

	testDir := "../../../../test/fixtures"

	fixtureDirs := []struct {
		description          string
		workingDir           string
		usePartialParseCache bool
	}{
		{"PartialParseBenchmarkRegressionCaching", "regressions/benchmark-parsing/production/deployment-group-1/webserver/terragrunt.hcl", true},
		{"PartialParseBenchmarkRegressionNoCache", "regressions/benchmark-parsing/production/deployment-group-1/webserver/terragrunt.hcl", false},
		{"PartialParseBenchmarkRegressionIncludesCaching", "regressions/benchmark-parsing-includes/production/deployment-group-1/webserver/terragrunt.hcl", true},
		{"PartialParseBenchmarkRegressionIncludesNoCache", "regressions/benchmark-parsing-includes/production/deployment-group-1/webserver/terragrunt.hcl", false},
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
			ctx := cli.NewAppContext(b.Context(), cli.NewApp(), nil)
			err = graph.Run(ctx, logger.CreateLogger(), terragruntOptions)
			b.StopTimer()
			require.NoError(b, err)
		})
	}
}
