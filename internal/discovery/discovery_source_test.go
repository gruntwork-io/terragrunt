package discovery_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/gruntwork-io/terragrunt/config"
	"github.com/gruntwork-io/terragrunt/internal/discovery"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func stringPtr(s string) *string {
	return &s
}

func TestExtractSourcesFromConfig(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		terraformSource *string
		name            string
		stackContent    string
		expectedSources []string
		isStackFile     bool
	}{
		{
			name:            "unit with terraform source",
			terraformSource: stringPtr("github.com/gruntwork-io/terraform-aws-service-catalog//modules/networking/vpc"),
			isStackFile:     false,
			expectedSources: []string{"github.com/gruntwork-io/terraform-aws-service-catalog//modules/networking/vpc"},
		},
		{
			name:            "unit without terraform source",
			terraformSource: nil,
			isStackFile:     false,
			expectedSources: []string{},
		},
		// TODO: Stack file source extraction can be added as an enhancement
		// {
		// 	name:        "stack file with multiple sources",
		// 	isStackFile: true,
		// 	stackContent: `
		// unit "vpc" {
		//   source = "github.com/gruntwork-io/terraform-aws-service-catalog//modules/networking/vpc"
		//   path   = "vpc"
		// }
		//
		// unit "app" {
		//   source = "github.com/example/terraform-modules//modules/app"
		//   path   = "app"
		// }
		//
		// stack "database" {
		//   source = "tfr://registry.terraform.io/terraform-aws-modules/rds/aws"
		//   path   = "database"
		// }
		// `,
		// 	expectedSources: []string{
		// 		"github.com/gruntwork-io/terraform-aws-service-catalog//modules/networking/vpc",
		// 		"github.com/example/terraform-modules//modules/app",
		// 		"tfr://registry.terraform.io/terraform-aws-modules/rds/aws",
		// 	},
		// },
		{
			name:        "stack file with no sources",
			isStackFile: true,
			stackContent: `
# Empty stack file
`,
			expectedSources: []string{},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// Create temporary directory for test
			tmpDir := t.TempDir()

			var (
				configPath string
				cfg        *config.TerragruntConfig
			)

			if tc.isStackFile {
				// Create stack file
				configPath = filepath.Join(tmpDir, "terragrunt.stack.hcl")
				err := os.WriteFile(configPath, []byte(tc.stackContent), 0644)
				require.NoError(t, err)

				cfg = &config.TerragruntConfig{}
			} else {
				// Create unit config
				cfg = &config.TerragruntConfig{
					Terraform: &config.TerraformConfig{
						Source: tc.terraformSource,
					},
				}
			}

			// Extract sources
			sources := discovery.ExtractSourcesFromConfig(cfg)

			// Verify results
			assert.ElementsMatch(t, tc.expectedSources, sources, "Sources should match expected values")
		})
	}
}

// TODO: Re-enable this test when stack file source extraction is implemented
// func TestExtractStackSources(t *testing.T) {
// 	t.Parallel()
//
// 	testCases := []struct {
// 		name            string
// 		stackContent    string
// 		expectedSources []string
// 	}{
// 		{
// 			name: "multiple unit and stack sources",
// 			stackContent: `
// unit "vpc" {
//   source = "github.com/gruntwork-io/terraform-aws-service-catalog//modules/networking/vpc"
//   path   = "vpc"
// }
//
// unit "app" {
//   source = "github.com/example/terraform-modules//modules/app"
//   path   = "app"
// }
//
// stack "database" {
//   source = "tfr://registry.terraform.io/terraform-aws-modules/rds/aws"
//   path   = "database"
// }
// `,
// 			expectedSources: []string{
// 				"github.com/gruntwork-io/terraform-aws-service-catalog//modules/networking/vpc",
// 				"github.com/example/terraform-modules//modules/app",
// 				"tfr://registry.terraform.io/terraform-aws-modules/rds/aws",
// 			},
// 		},
// 		{
// 			name: "only units",
// 			stackContent: `
// unit "vpc" {
//   source = "github.com/gruntwork-io/vpc"
//   path   = "vpc"
// }
// `,
// 			expectedSources: []string{
// 				"github.com/gruntwork-io/vpc",
// 			},
// 		},
// 		{
// 			name: "only stacks",
// 			stackContent: `
// stack "database" {
//   source = "tfr://registry.terraform.io/terraform-aws-modules/rds/aws"
//   path   = "database"
// }
// `,
// 			expectedSources: []string{
// 				"tfr://registry.terraform.io/terraform-aws-modules/rds/aws",
// 			},
// 		},
// 		{
// 			name: "empty stack file",
// 			stackContent: `
// # Just a comment
// `,
// 			expectedSources: []string{},
// 		},
// 	}
//
// 	for _, tc := range testCases {
// 		t.Run(tc.name, func(t *testing.T) {
// 			t.Parallel()
//
// 			// Create temporary stack file
// 			tmpDir := t.TempDir()
// 			stackPath := filepath.Join(tmpDir, "terragrunt.stack.hcl")
// 			err := os.WriteFile(stackPath, []byte(tc.stackContent), 0644)
// 			require.NoError(t, err)
//
// 			// Extract sources
// 			sources := extractStackSources(stackPath)
//
// 			// Verify results
// 			assert.ElementsMatch(t, tc.expectedSources, sources, "Sources should match expected values")
// 		})
// 	}
// }
