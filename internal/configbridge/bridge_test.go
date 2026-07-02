package configbridge_test

import (
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/configbridge"
	"github.com/gruntwork-io/terragrunt/internal/experiment"
	"github.com/gruntwork-io/terragrunt/pkg/options"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBackendOptsFromOpts_CopiesFields(t *testing.T) {
	t.Parallel()

	opts, err := options.NewTerragruntOptionsForTest("terragrunt.hcl")
	require.NoError(t, err)

	require.NoError(t, opts.Experiments.EnableExperiment(experiment.AzureBackend))

	opts.Env = map[string]string{"ARM_SUBSCRIPTION_ID": "sub"}
	opts.NonInteractive = true
	opts.FailIfBucketCreationRequired = true

	got := configbridge.BackendOptsFromOpts(opts)

	assert.True(t, got.Experiments.Evaluate(experiment.AzureBackend), "enabled experiments must reach backend options")
	assert.Equal(t, opts.Env, got.Env)
	assert.True(t, got.NonInteractive)
	assert.True(t, got.FailIfBucketCreationRequired)
}

func TestRemoteStateOptsFromOpts_CarriesBackendOptions(t *testing.T) {
	t.Parallel()

	opts, err := options.NewTerragruntOptionsForTest("terragrunt.hcl")
	require.NoError(t, err)

	require.NoError(t, opts.Experiments.EnableExperiment(experiment.AzureBackend))

	opts.DisableBucketUpdate = true
	opts.TerragruntConfigPath = "/work/terragrunt.hcl"
	opts.Env = map[string]string{"ARM_SUBSCRIPTION_ID": "sub"}

	got := configbridge.RemoteStateOptsFromOpts(opts)

	assert.True(t, got.Experiments.Evaluate(experiment.AzureBackend), "experiments must reach remote state options")
	assert.True(t, got.DisableBucketUpdate)

	// TFRunOpts feeds migrate state pull/push, so its threading is load-bearing.
	require.NotNil(t, got.TFRunOpts)
	assert.Equal(t, opts.TerragruntConfigPath, got.TFRunOpts.TerragruntConfigPath)
	require.NotNil(t, got.TFRunOpts.ShellOptions)
	assert.Equal(t, opts.Env, got.TFRunOpts.ShellOptions.Env, "env must reach the tofu process")
}
