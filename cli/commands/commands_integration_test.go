package commands

import (
	"context"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/cli"
	"github.com/gruntwork-io/terragrunt/internal/experiment"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/gruntwork-io/terragrunt/tf"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNativeProviderCacheExperimentIntegration(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()

	// Create minimal terragrunt options with the experiment enabled
	opts := &options.TerragruntOptions{
		TerraformImplementation: options.OpenTofuImpl,
		TerraformVersion:        mustParseVersion("1.11.0"),
		ProviderCacheDir:        tmpDir,
		Env:                     make(map[string]string),
		Experiments:             experiment.NewExperiments(),
	}

	// Enable the native provider cache experiment
	err := opts.Experiments.EnableExperiment(experiment.NativeProviderCache)
	require.NoError(t, err)

	// Verify the experiment is enabled
	assert.True(t, opts.Experiments.Evaluate(experiment.NativeProviderCache))

	l := logger.CreateLogger()

	// Create a mock CLI context
	ctx := context.Background()
	cliCtx := &cli.Context{
		Context: ctx,
	}

	// Mock action that doesn't actually run anything
	mockAction := func(*cli.Context) error {
		return nil
	}

	// Run the action which should set up the native provider cache
	err = runAction(cliCtx, l, opts, mockAction)
	require.NoError(t, err)

	// Verify that TF_PLUGIN_CACHE_DIR was set correctly
	assert.Equal(t, tmpDir, opts.Env[tf.EnvNameTFPluginCacheDir])
}

func TestNativeProviderCacheWithTerraformFallback(t *testing.T) {
	t.Parallel()

	// Create options with Terraform instead of OpenTofu
	opts := &options.TerragruntOptions{
		TerraformImplementation: options.TerraformImpl,
		TerraformVersion:        mustParseVersion("1.5.0"),
		Env:                     make(map[string]string),
		Experiments:             experiment.NewExperiments(),
	}

	// Enable the native provider cache experiment
	err := opts.Experiments.EnableExperiment(experiment.NativeProviderCache)
	require.NoError(t, err)

	l := logger.CreateLogger()

	// Create a mock CLI context
	ctx := context.Background()
	cliCtx := &cli.Context{
		Context: ctx,
	}

	// Mock action that doesn't actually run anything
	mockAction := func(*cli.Context) error {
		return nil
	}

	// Run the action - should silently fail to set up native provider cache
	err = runAction(cliCtx, l, opts, mockAction)
	require.NoError(t, err)

	// Verify that TF_PLUGIN_CACHE_DIR was NOT set since we're using Terraform
	_, exists := opts.Env[tf.EnvNameTFPluginCacheDir]
	assert.False(t, exists)
}
