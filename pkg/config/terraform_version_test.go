package config_test

import (
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/experiment"
	"github.com/gruntwork-io/terragrunt/pkg/config"
	"github.com/gruntwork-io/terragrunt/test/helpers"
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
	require.NoError(t, pctx.Experiments.EnableExperiment(experiment.VersionAttribute))

	terragruntConfig, err := config.ParseConfigString(ctx, pctx, l, config.DefaultTerragruntConfigPath, cfg, nil)
	require.NoError(t, err)

	require.NotNil(t, terragruntConfig.Terraform)
	require.NotNil(t, terragruntConfig.Terraform.Version)
	assert.Equal(t, "~> 3.3", *terragruntConfig.Terraform.Version)

	runConfig := terragruntConfig.ToRunConfig(l)
	require.NotNil(t, runConfig)
	assert.Equal(t, "~> 3.3", runConfig.Terraform.Version)
}

// TestTerraformConfigValidateVersionRequiresExperiment pins that the version
// attribute is rejected unless the version-attribute experiment is enabled.
// TG_EXPERIMENT_MODE forces every experiment on, which defeats the
// disabled-state assertion, so skip it there.
func TestTerraformConfigValidateVersionRequiresExperiment(t *testing.T) {
	t.Parallel()

	if helpers.IsExperimentMode(t) {
		t.Skip("Skipping: TG_EXPERIMENT_MODE forces the version-attribute experiment on, so its disabled-state error can't be verified")
	}

	cfg := &config.TerraformConfig{
		Source:  new("tfr://registry.opentofu.org/terraform-aws-modules/vpc/aws"),
		Version: new("~> 3.3"),
	}

	err := cfg.ValidateVersion(experiment.NewExperiments(), "terragrunt.hcl")

	var typed config.VersionAttributeRequiresExperimentError

	require.ErrorAs(t, err, &typed)
}

func TestTerraformConfigValidateVersion(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		cfg         *config.TerraformConfig
		expectedErr any
		name        string
	}{
		{
			name: "no version attribute",
			cfg:  &config.TerraformConfig{Source: new("tfr://registry.opentofu.org/terraform-aws-modules/vpc/aws")},
		},
		{
			name: "tfr source with version constraint",
			cfg:  &config.TerraformConfig{Source: new("tfr://registry.opentofu.org/terraform-aws-modules/vpc/aws"), Version: new("~> 3.3")},
		},
		{
			name:        "non-registry source with version",
			cfg:         &config.TerraformConfig{Source: new("github.com/terraform-aws-modules/terraform-aws-vpc"), Version: new("~> 3.3")},
			expectedErr: &config.VersionAttributeNonRegistrySourceError{},
		},
		{
			name:        "version without any source",
			cfg:         &config.TerraformConfig{Version: new("~> 3.3")},
			expectedErr: &config.VersionAttributeNonRegistrySourceError{},
		},
		{
			name:        "version attribute conflicts with inline version query",
			cfg:         &config.TerraformConfig{Source: new("tfr://registry.opentofu.org/terraform-aws-modules/vpc/aws?version=3.3.0"), Version: new("~> 3.3")},
			expectedErr: &config.VersionAttributeSourceConstraintConflictError{},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			experiments := experiment.NewExperiments()
			require.NoError(t, experiments.EnableExperiment(experiment.VersionAttribute))

			err := tc.cfg.ValidateVersion(experiments, "terragrunt.hcl")

			if tc.expectedErr == nil {
				require.NoError(t, err)
				return
			}

			require.ErrorAs(t, err, tc.expectedErr)
		})
	}
}
