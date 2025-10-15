package config_test

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/gruntwork-io/terragrunt/config"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCatalogParseConfigFile(t *testing.T) {
	t.Parallel()

	curDir, err := os.Getwd()
	require.NoError(t, err)

	basePath := filepath.Join(curDir, "../test/fixtures/catalog")

	testCases := []struct {
		expectedErr    error
		expectedConfig *config.CatalogConfig
		configPath     string
	}{
		{
			configPath: filepath.Join(basePath, "config1.hcl"),
			expectedConfig: &config.CatalogConfig{
				URLs: []string{
					filepath.Join(basePath, "terraform-aws-eks"), // this path exists in the fixture directory and must be converted to the absolute path.
					"/repo-copier",
					"./terraform-aws-service-catalog",
					"/project/terragrunt/test/terraform-aws-vpc",
					"github.com/gruntwork-io/terraform-aws-lambda",
				},
			},
		},
		{
			configPath: filepath.Join(basePath, "config2.hcl"),
		},
		{
			configPath:     filepath.Join(basePath, "config3.hcl"),
			expectedConfig: &config.CatalogConfig{},
		},
		{
			configPath: filepath.Join(basePath, "complex-legacy-root/terragrunt.hcl"),
			expectedConfig: &config.CatalogConfig{
				URLs: []string{
					filepath.Join(basePath, "complex-legacy-root/dev/us-west-1/modules/terraform-aws-eks"),
					"./terraform-aws-service-catalog",
					"https://github.com/gruntwork-io/terraform-aws-utilities",
				},
			},
		},
		{
			configPath: filepath.Join(basePath, "complex/root.hcl"),
			expectedConfig: &config.CatalogConfig{
				URLs: []string{
					filepath.Join(basePath, "complex/dev/us-west-1/modules/terraform-aws-eks"),
					"./terraform-aws-service-catalog",
					"https://github.com/gruntwork-io/terraform-aws-utilities",
				},
			},
		},
		{
			configPath: filepath.Join(basePath, "complex-legacy-root/dev/terragrunt.hcl"),
			expectedConfig: &config.CatalogConfig{
				URLs: []string{
					filepath.Join(basePath, "complex-legacy-root/dev/us-west-1/modules/terraform-aws-eks"),
					"./terraform-aws-service-catalog",
					"https://github.com/gruntwork-io/terraform-aws-utilities",
				},
			},
		},
		{
			configPath: filepath.Join(basePath, "complex/dev/root.hcl"),
			expectedConfig: &config.CatalogConfig{
				URLs: []string{
					filepath.Join(basePath, "complex/dev/us-west-1/modules/terraform-aws-eks"),
					"./terraform-aws-service-catalog",
					"https://github.com/gruntwork-io/terraform-aws-utilities",
				},
			},
		},
		{
			configPath: filepath.Join(basePath, "complex/dev/us-west-1/terragrunt.hcl"),
			expectedConfig: &config.CatalogConfig{
				URLs: []string{
					filepath.Join(basePath, "complex/dev/us-west-1/modules/terraform-aws-eks"),
					"./terraform-aws-service-catalog",
					"https://github.com/gruntwork-io/terraform-aws-utilities",
				},
			},
		},
		{
			configPath: filepath.Join(basePath, "complex/dev/us-west-1/modules/terragrunt.hcl"),
			expectedConfig: &config.CatalogConfig{
				URLs: []string{
					filepath.Join(basePath, "complex/dev/us-west-1/modules/terraform-aws-eks"),
					"./terraform-aws-service-catalog",
					"https://github.com/gruntwork-io/terraform-aws-utilities",
				},
			},
		},
		{
			configPath: filepath.Join(basePath, "complex/prod/terragrunt.hcl"),
			expectedConfig: &config.CatalogConfig{
				URLs: []string{
					filepath.Join(basePath, "complex/dev/us-west-1/modules/terraform-aws-eks"),
					"./terraform-aws-service-catalog",
					"https://github.com/gruntwork-io/terraform-aws-utilities",
				},
			},
		},
		{
			configPath: filepath.Join(basePath, "config4.hcl"),
			expectedConfig: &config.CatalogConfig{
				DefaultTemplate: "/test/fixtures/scaffold/external-template",
			},
		},
	}

	for i, tt := range testCases {
		t.Run(fmt.Sprintf("testCase-%d", i), func(t *testing.T) {
			t.Parallel()

			opts, err := options.NewTerragruntOptionsWithConfigPath(tt.configPath)
			require.NoError(t, err)

			opts.ScaffoldRootFileName = filepath.Base(tt.configPath)

			l := logger.CreateLogger()
			config, err := config.ReadCatalogConfig(t.Context(), l, opts)

			if tt.expectedErr == nil {
				require.NoError(t, err)
				assert.Equal(t, tt.expectedConfig, config)
			} else {
				assert.EqualError(t, err, tt.expectedErr.Error())
			}
		})
	}
}
