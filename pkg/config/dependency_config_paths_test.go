package config_test

import (
	"errors"
	"path/filepath"
	"testing"

	"github.com/gruntwork-io/terragrunt/pkg/config"
	"github.com/gruntwork-io/terragrunt/test/helpers/venvtest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zclconf/go-cty/cty"
)

func TestValidateDependencyConfigPaths(t *testing.T) {
	t.Parallel()

	repo := venvtest.Root("/repo")
	disabled := false

	tcs := []struct {
		enabled     *bool
		name        string
		configPath  string
		wantMissing string
	}{
		{name: "unit", configPath: "../unit"},
		{name: "JSON unit", configPath: "../json-unit"},
		{name: "stack", configPath: "../stack"},
		{name: "unit config file", configPath: "../unit/terragrunt.hcl"},
		{name: "deleted unit", configPath: "../deleted", wantMissing: filepath.Join(repo, "deleted")},
		{
			name:        "directory without a config",
			configPath:  "../empty",
			wantMissing: filepath.Join(repo, "empty", config.DefaultTerragruntConfigPath),
		},
		{name: "disabled dependency on a deleted unit", configPath: "../deleted", enabled: &disabled},
		{name: "stack-generated unit not generated yet", configPath: "../net/.terragrunt-stack/vpc"},
	}

	for _, tc := range tcs {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			fsys := venvtest.NewFS(t, repo, map[string]string{
				"app/terragrunt.hcl":            "",
				"unit/terragrunt.hcl":           "",
				"json-unit/terragrunt.hcl.json": "{}",
				"stack/terragrunt.stack.hcl":    "",
				"empty/main.tf":                 "",
			})

			cfg := &config.TerragruntConfig{
				TerragruntDependencies: config.Dependencies{
					{Name: "dep", ConfigPath: cty.StringVal(tc.configPath), Enabled: tc.enabled},
				},
			}

			err := config.ValidateDependencyConfigPaths(fsys, cfg, filepath.Join(repo, "app", "terragrunt.hcl"))
			if tc.wantMissing == "" {
				require.NoError(t, err)

				return
			}

			notFound, ok := errors.AsType[config.DependencyConfigNotFound](err)
			require.True(t, ok, "unexpected error %v", err)
			assert.Equal(t, tc.wantMissing, notFound.Path)
			assert.Contains(t, err.Error(), `dependency "dep"`)
		})
	}
}

func TestValidateDependencyConfigPathsNilConfig(t *testing.T) {
	t.Parallel()

	require.NoError(t, config.ValidateDependencyConfigPaths(nil, nil, "/repo/app/terragrunt.hcl"))
}
