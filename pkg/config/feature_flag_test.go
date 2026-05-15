package config_test

import (
	"testing"

	"github.com/gruntwork-io/terragrunt/pkg/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zclconf/go-cty/cty"
)

// TestFeatureFlagDeepMergeHandlesNilDefaults verifies DeepMerge does not panic on nil defaults.
func TestFeatureFlagDeepMergeHandlesNilDefaults(t *testing.T) {
	t.Parallel()

	sourceDefault := cty.MapVal(map[string]cty.Value{
		"enabled": cty.BoolVal(true),
	})
	target := &config.FeatureFlag{Name: "target"}
	source := &config.FeatureFlag{Name: "source", Default: &sourceDefault}

	require.NoError(t, target.DeepMerge(source))

	require.NotNil(t, target.Default)
	assert.Equal(t, sourceDefault, *target.Default)

	require.NoError(t, target.DeepMerge(&config.FeatureFlag{}))
	assert.Equal(t, sourceDefault, *target.Default)
}

// TestFeatureFlagDeepMergeMapOnlyHandlesNilDefaults verifies DeepMergeMapOnly does not panic on nil defaults.
func TestFeatureFlagDeepMergeMapOnlyHandlesNilDefaults(t *testing.T) {
	t.Parallel()

	sourceDefault := cty.MapVal(map[string]cty.Value{
		"enabled": cty.BoolVal(true),
	})
	target := &config.FeatureFlag{Name: "target"}
	source := &config.FeatureFlag{Name: "source", Default: &sourceDefault}

	require.NoError(t, target.DeepMergeMapOnly(source))

	require.NotNil(t, target.Default)
	assert.Equal(t, sourceDefault, *target.Default)

	require.NoError(t, target.DeepMergeMapOnly(&config.FeatureFlag{}))
	assert.Equal(t, sourceDefault, *target.Default)
}
