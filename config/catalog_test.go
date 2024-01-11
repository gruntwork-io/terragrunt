package config

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/gruntwork-io/terragrunt/options"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCatalogParseConfigFile(t *testing.T) {
	t.Parallel()

	curDir, err := os.Getwd()
	require.NoError(t, err)

	basePath := filepath.Join(curDir, "../test/fixture-catalog")

	testCases := []struct {
		configPath     string
		expectedConfig *CatalogConfig
		expectedErr    error
	}{
		{
			filepath.Join(basePath, "config1.hcl"),
			&CatalogConfig{
				URLs: []string{
					filepath.Join(basePath, "terraform-aws-eks"), // this path exists in the fixture directory and must be converted to the absolute path.
					"/repo-copier",
					"./terraform-aws-service-catalog",
					"/project/terragrunt/test/terraform-aws-vpc",
					"github.com/gruntwork-io/terraform-aws-lambda",
				},
			},
			nil,
		},
		{
			filepath.Join(basePath, "config2.hcl"),
			nil,
			nil,
		},
		{
			filepath.Join(basePath, "config3.hcl"),
			&CatalogConfig{},
			nil,
		},
		{
			filepath.Join(basePath, "complex/terragrunt.hcl"),
			&CatalogConfig{
				URLs: []string{
					filepath.Join(basePath, "complex/dev/us-west-1/modules/terraform-aws-eks"),
					"./terraform-aws-service-catalog",
					"https://github.com/gruntwork-io/terraform-aws-utilities",
				},
			},
			nil,
		},
		{
			filepath.Join(basePath, "complex/dev/terragrunt.hcl"),
			&CatalogConfig{
				URLs: []string{
					filepath.Join(basePath, "complex/dev/us-west-1/modules/terraform-aws-eks"),
					"./terraform-aws-service-catalog",
					"https://github.com/gruntwork-io/terraform-aws-utilities",
				},
			},
			nil,
		},
		{
			filepath.Join(basePath, "complex/dev/us-west-1/terragrunt.hcl"),
			&CatalogConfig{
				URLs: []string{
					filepath.Join(basePath, "complex/dev/us-west-1/modules/terraform-aws-eks"),
					"./terraform-aws-service-catalog",
					"https://github.com/gruntwork-io/terraform-aws-utilities",
				},
			},
			nil,
		},
		{
			filepath.Join(basePath, "complex/dev/us-west-1/modules/terragrunt.hcl"),
			&CatalogConfig{
				URLs: []string{
					filepath.Join(basePath, "complex/dev/us-west-1/modules/terraform-aws-eks"),
					"./terraform-aws-service-catalog",
					"https://github.com/gruntwork-io/terraform-aws-utilities",
				},
			},
			nil,
		},
		{
			filepath.Join(basePath, "complex/prod/terragrunt.hcl"),
			&CatalogConfig{
				URLs: []string{
					filepath.Join(basePath, "complex/dev/us-west-1/modules/terraform-aws-eks"),
					"./terraform-aws-service-catalog",
					"https://github.com/gruntwork-io/terraform-aws-utilities",
				},
			},
			nil,
		},
	}

	for i, testCase := range testCases {
		testCase := testCase

		t.Run(fmt.Sprintf("testCase-%d", i), func(t *testing.T) {
			t.Parallel()

			opts, err := options.NewTerragruntOptionsWithConfigPath(testCase.configPath)
			require.NoError(t, err)

			config, err := ReadCatalogConfig(context.Background(), opts)

			if testCase.expectedErr == nil {
				require.NoError(t, err)
				assert.Equal(t, testCase.expectedConfig, config)
			} else {
				assert.EqualError(t, err, testCase.expectedErr.Error())
			}
		})

	}
}
