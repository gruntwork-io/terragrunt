package config_test

import (
	"testing"

	"github.com/gruntwork-io/terragrunt/config"
	"github.com/gruntwork-io/terragrunt/config/hclparse"
	"github.com/gruntwork-io/terragrunt/internal/strict/controls"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDependencyInputsBlockedByDefault(t *testing.T) {
	t.Parallel()

	// Test that dependency.foo.inputs syntax is now blocked by default
	configWithDependencyInputs := `
dependency "dep" {
  config_path = "../dep"
}

inputs = {
  value = dependency.dep.inputs.some_value
}
`

	parser := hclparse.NewParser()
	file, err := parser.ParseFromString(configWithDependencyInputs, "terragrunt.hcl")
	require.NoError(t, err)

	// Create a parsing context with strict controls
	ctx := &config.ParsingContext{
		TerragruntOptions: &options.TerragruntOptions{
			StrictControls: controls.New(),
		},
	}

	logger := log.New()

	// Test that the deprecated configuration is detected and blocked
	err = config.DetectDeprecatedConfigurations(ctx, logger, file)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "Reading inputs from dependencies is no longer supported")
	assert.Contains(t, err.Error(), "use outputs")
}

func TestDependencyOutputsStillAllowed(t *testing.T) {
	t.Parallel()

	// Test that dependency.foo.outputs syntax still works fine
	configWithDependencyOutputs := `
dependency "dep" {
  config_path = "../dep"
}

inputs = {
  value = dependency.dep.outputs.some_value
}
`

	parser := hclparse.NewParser()
	file, err := parser.ParseFromString(configWithDependencyOutputs, "terragrunt.hcl")
	require.NoError(t, err)

	// Create a parsing context with strict controls
	ctx := &config.ParsingContext{
		TerragruntOptions: &options.TerragruntOptions{
			StrictControls: controls.New(),
		},
	}

	logger := log.New()

	// Test that the dependency outputs are allowed (no error)
	err = config.DetectDeprecatedConfigurations(ctx, logger, file)
	require.NoError(t, err)
}

func TestDetectInputsCtyUsageFunction(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string
		config   string
		expected bool
	}{
		{
			name: "dependency inputs detected",
			config: `
inputs = {
  value = dependency.dep.inputs.some_value
}
`,
			expected: true,
		},
		{
			name: "dependency outputs not detected",
			config: `
inputs = {
  value = dependency.dep.outputs.some_value
}
`,
			expected: false,
		},
		{
			name: "no dependency references",
			config: `
inputs = {
  value = "static_value"
}
`,
			expected: false,
		},
		{
			name: "multiple dependency inputs detected",
			config: `
inputs = {
  value1 = dependency.dep1.inputs.val1
  value2 = dependency.dep2.inputs.val2
}
`,
			expected: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			parser := hclparse.NewParser()
			file, err := parser.ParseFromString(tc.config, "terragrunt.hcl")
			require.NoError(t, err)

			result := config.DetectInputsCtyUsage(file)
			assert.Equal(t, tc.expected, result)
		})
	}
}
