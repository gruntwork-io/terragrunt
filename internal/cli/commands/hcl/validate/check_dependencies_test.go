package validate_test

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRunValidateDependencyConfigPaths(t *testing.T) {
	t.Parallel()

	tcs := []struct {
		name       string
		configPath string
		wantErr    bool
	}{
		{name: "existing dependency", configPath: "../db"},
		{name: "stack dependency", configPath: "../net"},
		{name: "deleted dependency", configPath: "../deleted", wantErr: true},
		{
			name:       "stack unit not generated",
			configPath: "../net/.terragrunt-stack/vpc",
			wantErr:    true,
		},
	}

	for _, tc := range tcs {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			result := runValidate(t, validateRun{files: map[string]string{
				"app/terragrunt.hcl":       "dependency \"dep\" {\n  config_path = \"" + tc.configPath + "\"\n}\n",
				"db/terragrunt.hcl":        "",
				"net/terragrunt.stack.hcl": "unit \"vpc\" {\n  source = \"../vpc\"\n  path   = \"vpc\"\n}\n",
			}})
			if !tc.wantErr {
				require.NoError(t, result.err)

				return
			}

			requireMissingDependencyAt(
				t,
				result,
				"app/terragrunt.hcl",
				2,
				`"`+tc.configPath+`"`,
				"This directory does not exist.",
			)
		})
	}
}
