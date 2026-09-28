package config_test

import (
	"errors"
	"io/fs"
	"path/filepath"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/gruntwork-io/terragrunt/pkg/config"
	"github.com/gruntwork-io/terragrunt/test/helpers/venvtest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zclconf/go-cty/cty"
)

func TestMissingDependencyConfigs(t *testing.T) {
	t.Parallel()

	repo := venvtest.Root("/repo")
	appConfig := filepath.Join(repo, "app", config.DefaultTerragruntConfigPath)
	disabled := false

	tcs := []struct {
		cfg     *config.TerragruntConfig
		name    string
		missing []string
	}{
		{
			name: "nil config",
		},
		{
			name: "dependency on a unit",
			cfg:  dependencyConfig("../unit"),
		},
		{
			name: "dependency on a JSON unit",
			cfg:  dependencyConfig("../json-unit"),
		},
		{
			name: "dependency on a stack",
			cfg:  dependencyConfig("../stack"),
		},
		{
			name: "dependency on a stack file",
			cfg:  dependencyConfig("../stack/" + config.DefaultStackFile),
		},
		{
			name: "dependency on a unit config file",
			cfg:  dependencyConfig("../unit/" + config.DefaultTerragruntConfigPath),
		},
		{
			name:    "dependency on a deleted unit",
			cfg:     dependencyConfig("../deleted"),
			missing: []string{filepath.Join(repo, "deleted")},
		},
		{
			name:    "dependency on a directory without a config",
			cfg:     dependencyConfig("../empty"),
			missing: []string{filepath.Join(repo, "empty")},
		},
		{
			name:    "dependency on a deleted unit config file",
			cfg:     dependencyConfig("../deleted/" + config.DefaultTerragruntConfigPath),
			missing: []string{filepath.Join(repo, "deleted", config.DefaultTerragruntConfigPath)},
		},
		{
			name:    "absolute dependency on a deleted unit",
			cfg:     dependencyConfig(filepath.Join(repo, "deleted")),
			missing: []string{filepath.Join(repo, "deleted")},
		},
		{
			name: "disabled dependency on a deleted unit",
			cfg: &config.TerragruntConfig{
				TerragruntDependencies: config.Dependencies{
					{Name: "deleted", ConfigPath: cty.StringVal("../deleted"), Enabled: &disabled},
				},
			},
		},
		{
			name: "dependency without a usable config path",
			cfg: &config.TerragruntConfig{
				TerragruntDependencies: config.Dependencies{
					{Name: "null", ConfigPath: cty.NullVal(cty.String)},
					{Name: "unknown", ConfigPath: cty.UnknownVal(cty.String)},
					{Name: "empty", ConfigPath: cty.StringVal("")},
				},
			},
		},
		{
			name: "dependencies block path without a config",
			cfg: &config.TerragruntConfig{
				Dependencies: &config.ModuleDependencies{Paths: []string{"../unit", "../empty"}},
			},
			missing: []string{filepath.Join(repo, "empty")},
		},
		{
			name: "path in both dependency blocks and the dependencies block is reported once",
			cfg: &config.TerragruntConfig{
				TerragruntDependencies: config.Dependencies{
					{Name: "empty", ConfigPath: cty.StringVal("../empty")},
					{Name: "again", ConfigPath: cty.StringVal("../empty/")},
				},
				Dependencies: &config.ModuleDependencies{Paths: []string{"../empty"}},
			},
			missing: []string{filepath.Join(repo, "empty")},
		},
		{
			name: "every missing dependency is reported in order",
			cfg: &config.TerragruntConfig{
				TerragruntDependencies: config.Dependencies{
					{Name: "deleted", ConfigPath: cty.StringVal("../deleted")},
					{Name: "unit", ConfigPath: cty.StringVal("../unit")},
					{Name: "empty", ConfigPath: cty.StringVal("../empty")},
				},
				Dependencies: &config.ModuleDependencies{Paths: []string{"../empty/.."}},
			},
			missing: []string{
				filepath.Join(repo, "deleted"),
				filepath.Join(repo, "empty"),
				repo,
			},
		},
		{
			name: "dependencies path that is not a directory is left to parsing",
			cfg: &config.TerragruntConfig{
				Dependencies: &config.ModuleDependencies{Paths: []string{"../gone", "../app/terragrunt.hcl"}},
			},
		},
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

			missing, err := config.MissingDependencyConfigs(fsys, tc.cfg, appConfig)
			require.NoError(t, err)

			var got []string

			for _, depErr := range missing {
				assert.Equal(t, filepath.Join(repo, "app"), depErr.UnitPath)
				got = append(got, depErr.DependencyPath)
			}

			assert.Equal(t, tc.missing, got)
		})
	}
}

// TestMissingDependencyConfigsStatFailure pins that a Stat fault on the target is reported with its cause, not as missing.
func TestMissingDependencyConfigsStatFailure(t *testing.T) {
	t.Parallel()

	repo := venvtest.Root("/repo")
	fsys := &statFailFS{
		FS: venvtest.NewFS(t, repo, map[string]string{
			"app/terragrunt.hcl":  "",
			"unit/terragrunt.hcl": "",
		}),
		failPath: filepath.Join(repo, "unit"),
	}

	missing, err := config.MissingDependencyConfigs(
		fsys,
		dependencyConfig("../unit"),
		filepath.Join(repo, "app", config.DefaultTerragruntConfigPath),
	)
	require.ErrorIs(t, err, fs.ErrPermission)
	assert.Empty(t, missing)

	checkErr, ok := errors.AsType[config.DependencyConfigCheckError](err)
	require.True(t, ok, "unexpected error %v", err)
	assert.Equal(t, filepath.Join(repo, "app"), checkErr.UnitPath)
	assert.Equal(t, filepath.Join(repo, "unit"), checkErr.DependencyPath)
	assert.Contains(t, checkErr.Error(), "could not be checked")
}

// TestMissingDependencyConfigsOSFiles pins custom config files and paths below a regular file on a real filesystem.
func TestMissingDependencyConfigsOSFiles(t *testing.T) {
	t.Parallel()

	repo := t.TempDir()

	for path, contents := range map[string]string{
		"app/terragrunt.hcl": "",
		"custom/unit.hcl":    "",
		"file.txt":           "",
	} {
		require.NoError(t, vfs.WriteFile(vfs.NewOSFS(), filepath.Join(repo, path), []byte(contents), 0o644))
	}

	cfg := &config.TerragruntConfig{
		TerragruntDependencies: config.Dependencies{
			{Name: "custom", ConfigPath: cty.StringVal("../custom/unit.hcl")},
			{Name: "below-file", ConfigPath: cty.StringVal("../file.txt/unit")},
		},
	}

	missing, err := config.MissingDependencyConfigs(
		vfs.NewOSFS(),
		cfg,
		filepath.Join(repo, "app", config.DefaultTerragruntConfigPath),
	)
	require.NoError(t, err)
	require.Len(t, missing, 1)
	assert.Equal(t, filepath.Join(repo, "file.txt", "unit"), missing[0].DependencyPath)
}

func TestMissingDependencyConfigError(t *testing.T) {
	t.Parallel()

	notFound := config.TerragruntConfigNotFoundError{Path: "/repo/cache/terragrunt.hcl"}
	err := config.MissingDependencyConfigError{
		Err:            notFound,
		UnitPath:       "/repo/app",
		DependencyPath: "/repo/cache",
	}

	assert.Equal(
		t,
		`unit "/repo/app" depends on "/repo/cache", where no Terragrunt configuration was found`,
		err.Error(),
	)
	require.ErrorIs(t, err, notFound)

	_, ok := errors.AsType[config.TerragruntConfigNotFoundError](err)
	assert.True(t, ok)
	require.NoError(t, config.MissingDependencyConfigError{}.Unwrap())
}

func TestDependencyConfigCheckError(t *testing.T) {
	t.Parallel()

	err := config.DependencyConfigCheckError{
		Err:            fs.ErrPermission,
		UnitPath:       "/repo/app",
		DependencyPath: "/repo/db",
	}

	assert.Equal(
		t,
		`unit "/repo/app" depends on "/repo/db", whose Terragrunt configuration could not be checked: permission denied`,
		err.Error(),
	)
	require.ErrorIs(t, err, fs.ErrPermission)
}

// statFailFS fails every Stat under failPath with a non-ENOENT error.
type statFailFS struct {
	vfs.FS
	failPath string
}

func (f *statFailFS) Stat(name string) (fs.FileInfo, error) {
	if name == f.failPath || filepath.Dir(name) == f.failPath {
		return nil, fs.ErrPermission
	}

	return f.FS.Stat(name)
}

func dependencyConfig(configPath string) *config.TerragruntConfig {
	return &config.TerragruntConfig{
		TerragruntDependencies: config.Dependencies{
			{Name: "dep", ConfigPath: cty.StringVal(configPath)},
		},
	}
}
