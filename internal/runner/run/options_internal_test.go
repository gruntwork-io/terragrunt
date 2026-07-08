package run

import (
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/experiment"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRemoteStateOptsPropagatesExperiments(t *testing.T) {
	t.Parallel()

	exps := experiment.NewExperiments()
	require.NoError(t, exps.EnableExperiment(experiment.AzureBackend))

	o := &Options{
		Experiments:                  exps,
		TerragruntConfigPath:         "/work/terragrunt.hcl",
		NonInteractive:               true,
		FailIfBucketCreationRequired: true,
		DisableBucketUpdate:          true,
	}

	got := o.remoteStateOpts()

	assert.True(t, got.Experiments.Evaluate(experiment.AzureBackend), "enabled experiments must reach backend options")
	assert.True(t, got.NonInteractive)
	assert.True(t, got.FailIfBucketCreationRequired)
	assert.True(t, got.DisableBucketUpdate)

	// TFRunOpts feeds migrate state pull/push, so its threading is load-bearing.
	require.NotNil(t, got.TFRunOpts)
	assert.Equal(t, o.TerragruntConfigPath, got.TFRunOpts.TerragruntConfigPath)
}
