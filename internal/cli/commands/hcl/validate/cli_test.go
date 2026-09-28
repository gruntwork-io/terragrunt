package validate_test

import (
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/cli/commands/hcl/validate"
	"github.com/gruntwork-io/terragrunt/internal/clihelper"
	"github.com/gruntwork-io/terragrunt/internal/vexec"
	"github.com/gruntwork-io/terragrunt/pkg/options"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/gruntwork-io/terragrunt/test/helpers/venvtest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestNewFlagsMapsEachEnvVarToOneFlag pins every env var of the command's own
// flags, current and deprecated, to the single flag it sets. Each case checks
// all four destinations, so an env var wired to a second flag fails here.
func TestNewFlagsMapsEachEnvVarToOneFlag(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		envVar             string
		wantStrict         bool
		wantInputs         bool
		wantShowConfigPath bool
		wantJSONOutput     bool
	}{
		{
			envVar:     "TG_STRICT",
			wantStrict: true,
		},
		{
			envVar:     "TG_STRICT_VALIDATE",
			wantStrict: true,
		},
		{
			envVar:     "TG_HCLVALIDATE_STRICT_VALIDATE",
			wantStrict: true,
		},
		{
			envVar:     "TERRAGRUNT_STRICT_VALIDATE",
			wantStrict: true,
		},
		{
			envVar:     "TG_INPUTS",
			wantInputs: true,
		},
		{
			envVar:             "TG_SHOW_CONFIG_PATH",
			wantShowConfigPath: true,
		},
		{
			envVar:             "TG_HCLVALIDATE_SHOW_CONFIG_PATH",
			wantShowConfigPath: true,
		},
		{
			envVar:             "TERRAGRUNT_HCLVALIDATE_SHOW_CONFIG_PATH",
			wantShowConfigPath: true,
		},
		{
			envVar:         "TG_JSON",
			wantJSONOutput: true,
		},
		{
			envVar:         "TG_HCLVALIDATE_JSON",
			wantJSONOutput: true,
		},
		{
			envVar:         "TERRAGRUNT_HCLVALIDATE_JSON",
			wantJSONOutput: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.envVar, func(t *testing.T) {
			t.Parallel()

			opts := options.NewTerragruntOptions(vexec.NewOSExec())
			flagSet := validate.NewFlags(logger.CreateLogger(), opts, venvtest.New())

			require.NoError(t, flagSet.Parse(clihelper.Args{}, map[string]string{tc.envVar: "true"}))

			assert.Equal(t, tc.wantStrict, opts.HCLValidateStrict, validate.StrictFlagName)
			assert.Equal(t, tc.wantInputs, opts.HCLValidateInputs, validate.InputsFlagName)
			assert.Equal(t, tc.wantShowConfigPath, opts.HCLValidateShowConfigPath, validate.ShowConfigPathFlagName)
			assert.Equal(t, tc.wantJSONOutput, opts.HCLValidateJSONOutput, validate.JSONFlagName)
		})
	}
}
