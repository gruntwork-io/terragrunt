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
		{name: "generated stack unit", configPath: "../stack/.terragrunt-stack/vpc"},
		{name: "deleted unit", configPath: "../deleted", wantMissing: filepath.Join(repo, "deleted")},
		{
			name:        "directory without a config",
			configPath:  "../empty",
			wantMissing: filepath.Join(repo, "empty", config.DefaultTerragruntConfigPath),
		},
		{name: "disabled dependency on a deleted unit", configPath: "../deleted", enabled: &disabled},
		{
			name:        "stack unit not generated yet",
			configPath:  "../net/.terragrunt-stack/vpc",
			wantMissing: filepath.Join(repo, "net", ".terragrunt-stack", "vpc"),
		},
	}

	for _, tc := range tcs {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			fsys := venvtest.NewFS(t, repo, map[string]string{
				"app/terragrunt.hcl":                         "",
				"unit/terragrunt.hcl":                        "",
				"json-unit/terragrunt.hcl.json":              "{}",
				"stack/terragrunt.stack.hcl":                 "",
				"stack/.terragrunt-stack/vpc/terragrunt.hcl": "",
				"net/terragrunt.stack.hcl":                   "",
				"empty/main.tf":                              "",
			})

			err := config.ValidateDependencyConfigPaths(fsys, dependencyConfig(tc.configPath, tc.enabled), appConfigPath(repo))
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

// TestValidateDependencyConfigPathsStatError pins that a filesystem fault on the
// dependency's config is reported as is, not as a missing configuration.
func TestValidateDependencyConfigPathsStatError(t *testing.T) {
	t.Parallel()

	repo := venvtest.Root("/repo")
	fsys := statErrorFS{
		FS:       venvtest.NewFS(t, repo, map[string]string{"app/terragrunt.hcl": ""}),
		failPath: filepath.Join(repo, "dep"),
	}

	err := config.ValidateDependencyConfigPaths(fsys, dependencyConfig("../dep", nil), appConfigPath(repo))
	require.ErrorIs(t, err, errStatFailed)

	_, notFound := errors.AsType[config.DependencyConfigNotFound](err)
	assert.False(t, notFound, "a stat fault must not read as a missing configuration")
	assert.Contains(t, err.Error(), `dependency "dep"`)
}

func dependencyConfig(configPath string, enabled *bool) *config.TerragruntConfig {
	return &config.TerragruntConfig{
		TerragruntDependencies: config.Dependencies{
			{Name: "dep", ConfigPath: cty.StringVal(configPath), Enabled: enabled},
		},
	}
}

func appConfigPath(repo string) string {
	return filepath.Join(repo, "app", config.DefaultTerragruntConfigPath)
}
