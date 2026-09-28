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

func TestRunValidateCheckDependencies(t *testing.T) {
	t.Parallel()

	tcs := []struct {
		name    string
		check   bool
		wantErr bool
	}{
		{name: "without the flag"},
		{name: "with the flag", check: true, wantErr: true},
	}

	for _, tc := range tcs {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			root := venvtest.Root("/repo")
			v := venvtest.New().WithFS(venvtest.NewFS(t, root, map[string]string{
				"app/terragrunt.hcl": "dependency \"deleted\" {\n  config_path = \"../deleted\"\n}\n",
			}))

			opts, err := options.NewTerragruntOptionsForTest(filepath.Join(root, config.DefaultTerragruntConfigPath))
			require.NoError(t, err)

			opts.HCLValidateCheckDependencies = tc.check

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

func TestRunCheckDependenciesFlagCombinations(t *testing.T) {
	t.Parallel()

	tcs := []struct {
		name    string
		wantErr string
		json    bool
		show    bool
	}{
		{name: "json output", json: true, wantErr: "specifying both -json and -check-dependencies is invalid"},
		{
			name:    "show config path",
			show:    true,
			wantErr: "specifying both -show-config-path and -check-dependencies is invalid",
		},
	}

	for _, tc := range tcs {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			opts, err := options.NewTerragruntOptionsForTest(filepath.Join(venvtest.Root("/repo"), "terragrunt.hcl"))
			require.NoError(t, err)

			opts.HCLValidateCheckDependencies = true
			opts.HCLValidateJSONOutput = tc.json
			opts.HCLValidateShowConfigPath = tc.show

			require.EqualError(t, validate.Run(t.Context(), logger.CreateLogger(), venvtest.New(), opts), tc.wantErr)
		})
	}
}
