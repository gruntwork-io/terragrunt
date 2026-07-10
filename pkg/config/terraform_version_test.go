package config_test

import (
	"testing"

	"github.com/gruntwork-io/terragrunt/pkg/config"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseTerraformVersionAttribute(t *testing.T) {
	t.Parallel()

	cfg := `
terraform {
	source  = "tfr://registry.opentofu.org/terraform-aws-modules/vpc/aws"
	version = "~> 3.3"
}
`

	l := logger.CreateLogger()

	ctx, pctx := newTestParsingContext(t, "test-time-mock")

	terragruntConfig, err := config.ParseConfigString(ctx, pctx, l, config.DefaultTerragruntConfigPath, cfg, nil)
	require.NoError(t, err)

	require.NotNil(t, terragruntConfig.Terraform)
	require.NotNil(t, terragruntConfig.Terraform.Version)
	assert.Equal(t, "~> 3.3", *terragruntConfig.Terraform.Version)

	runConfig := terragruntConfig.ToRunConfig(l)
	require.NotNil(t, runConfig)
	assert.Equal(t, "~> 3.3", runConfig.Terraform.Version)
}

func TestTerraformConfigValidateVersion(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		cfg       *config.TerraformConfig
		assertErr func(t *testing.T, err error)
		name      string
	}{
		{
			name: "no version attribute",
			cfg:  &config.TerraformConfig{Source: new("tfr://registry.opentofu.org/terraform-aws-modules/vpc/aws")},
			assertErr: func(t *testing.T, err error) {
				t.Helper()
				require.NoError(t, err)
			},
		},
		{
			name: "tfr source with version constraint",
			cfg:  &config.TerraformConfig{Source: new("tfr://registry.opentofu.org/terraform-aws-modules/vpc/aws"), Version: new("~> 3.3")},
			assertErr: func(t *testing.T, err error) {
				t.Helper()
				require.NoError(t, err)
			},
		},
		{
			name: "non-registry source with version",
			cfg:  &config.TerraformConfig{Source: new("github.com/terraform-aws-modules/terraform-aws-vpc"), Version: new("~> 3.3")},
			assertErr: func(t *testing.T, err error) {
				t.Helper()

				var typed config.VersionAttributeNonRegistrySourceError

				require.ErrorAs(t, err, &typed)
			},
		},
		{
			name: "version without any source",
			cfg:  &config.TerraformConfig{Version: new("~> 3.3")},
			assertErr: func(t *testing.T, err error) {
				t.Helper()

				var typed config.VersionAttributeNonRegistrySourceError

				require.ErrorAs(t, err, &typed)
			},
		},
		{
			name: "version attribute conflicts with inline version query",
			cfg:  &config.TerraformConfig{Source: new("tfr://registry.opentofu.org/terraform-aws-modules/vpc/aws?version=3.3.0"), Version: new("~> 3.3")},
			assertErr: func(t *testing.T, err error) {
				t.Helper()

				var typed config.VersionAttributeSourceConstraintConflictError

				require.ErrorAs(t, err, &typed)
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			tc.assertErr(t, tc.cfg.ValidateVersion())
		})
	}
}
