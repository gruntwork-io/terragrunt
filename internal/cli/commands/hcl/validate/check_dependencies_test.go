package validate_test

import (
	"errors"
	"path/filepath"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/cli/commands/hcl/validate"
	"github.com/gruntwork-io/terragrunt/pkg/config"
	"github.com/gruntwork-io/terragrunt/pkg/options"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/gruntwork-io/terragrunt/test/helpers/venvtest"
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
		{name: "stack unit not generated", configPath: "../net/.terragrunt-stack/vpc", wantErr: true},
	}

	for _, tc := range tcs {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			root := venvtest.Root("/repo")
			v := venvtest.New().WithFS(venvtest.NewFS(t, root, map[string]string{
				"app/terragrunt.hcl":       "dependency \"dep\" {\n  config_path = \"" + tc.configPath + "\"\n}\n",
				"db/terragrunt.hcl":        "",
				"net/terragrunt.stack.hcl": "unit \"vpc\" {\n  source = \"../vpc\"\n  path   = \"vpc\"\n}\n",
			}))

			opts, err := options.NewTerragruntOptionsForTest(filepath.Join(root, config.DefaultTerragruntConfigPath))
			require.NoError(t, err)

			err = validate.RunValidate(t.Context(), logger.CreateLogger(), v, opts)
			if !tc.wantErr {
				require.NoError(t, err)

				return
			}

			_, ok := errors.AsType[config.DependencyConfigNotFound](err)
			require.True(t, ok, "unexpected error %v", err)
		})
	}
}
