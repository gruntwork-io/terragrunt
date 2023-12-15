package config

import (
	"errors"
	"fmt"
	"path/filepath"
	"testing"

	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/util"
	"github.com/stretchr/testify/assert"
)

func TestCatalogParseConfigFile(t *testing.T) {
	t.Parallel()

	basePath := "testdata/fixture-catalog"

	testCases := []struct {
		configPath      string
		expectedCatalog *CatalogConfig
		expectedErr     error
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
			nil,
			errors.New(filepath.Join(basePath, "config3.hcl") + `:1,9-9: Missing required argument; The argument "urls" is required, but no definition was found.`),
		},
	}

	for i, testCase := range testCases {
		testCase := testCase

		t.Run(fmt.Sprintf("testCase-%d", i), func(t *testing.T) {
			t.Parallel()

			opts := &options.TerragruntOptions{
				Logger:               util.GlobalFallbackLogEntry,
				TerragruntConfigPath: testCase.configPath,
			}

			config, err := ReadTerragruntConfig(opts)

			if testCase.expectedErr == nil {
				assert.NoError(t, err)
				assert.Equal(t, testCase.expectedCatalog, config.Catalog)
			} else {
				assert.EqualError(t, err, testCase.expectedErr.Error())
			}
		})

	}
}
