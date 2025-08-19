package hclparse_test

import (
	"testing"

	"github.com/gruntwork-io/terragrunt/config"
	"github.com/gruntwork-io/terragrunt/config/hclparse"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/hashicorp/hcl/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateNoDuplicateBlocks(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		hclCode     string
		expectedErr string
		expectError bool
	}{
		{
			name: "unique dependency blocks",
			hclCode: `
dependency "vpc" {
  config_path = "../vpc"
}

dependency "database" {
  config_path = "../database"
}
`,
			expectError: false,
		},
		{
			name: "single dependency block",
			hclCode: `
dependency "vpc" {
  config_path = "../vpc"
}
`,
			expectError: false,
		},
		{
			name:        "empty file",
			hclCode:     ``,
			expectError: false,
		},
		{
			name: "duplicate dependency blocks",
			hclCode: `
dependency "vpc" {
  config_path = "../vpc"
}

dependency "vpc" {
  config_path = "../vpc2"
}
`,
			expectError: true,
			expectedErr: "Duplicate dependency block with label 'vpc' found",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			file, err := hclparse.NewParser().ParseFromString(test.hclCode, "test.hcl")
			require.NoError(t, err)

			// Determine which struct to decode into based on content
			if test.name == "empty file" {
				var decoded struct{}
				err = file.Decode(&decoded, &hcl.EvalContext{})
			} else {
				var decoded config.TerragruntDependency
				err = file.Decode(&decoded, &hcl.EvalContext{})
			}

			if test.expectError {
				require.Error(t, err)
				assert.Contains(t, err.Error(), test.expectedErr)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidateNoDuplicateBlocks_NonSyntaxBody(t *testing.T) {
	t.Parallel()

	// Test that non-syntax bodies are handled gracefully
	parser := hclparse.NewParser(hclparse.WithLogger(logger.CreateLogger()))
	file, err := parser.ParseFromString("", "empty.hcl")
	require.NoError(t, err)

	var decoded struct{}
	err = file.Decode(&decoded, &hcl.EvalContext{})
	assert.NoError(t, err)
}
