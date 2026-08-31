package cliconfig_test

import (
	"path/filepath"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/tf/cliconfig"
	"github.com/gruntwork-io/terragrunt/internal/tfimpl"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/gruntwork-io/terragrunt/test/helpers/venvtest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadUserConfig(t *testing.T) {
	t.Parallel()

	const home = "/virtual/home"

	v := userConfigVenv(home, nil)
	configPath := filepath.Join(home, ".tofurc")

	require.NoError(t, v.FS.MkdirAll(home, 0o755))
	require.NoError(t, vfs.WriteFile(v.FS, configPath, []byte(`
plugin_cache_dir             = "/virtual/cache"
disable_checkpoint           = true
disable_checkpoint_signature = true

credentials "registry.example.com" {
  token = "configured-token"
}

credentials_helper "example" {
  args = ["one", "two"]
}

host "registry.example.com" {
  services = {
    "providers.v1" = "https://registry.example.com/providers/"
  }
}

provider_installation {
  filesystem_mirror {
    path    = "/virtual/mirror"
    include = ["registry.example.com/*/*"]
  }

  network_mirror {
    url     = "https://mirror.example.com/providers/"
    exclude = ["registry.example.com/private/*"]
  }

  direct {
    exclude = ["registry.example.com/*/*"]
  }
}
`), 0o600))

	cfg, err := cliconfig.LoadUserConfig(v, tfimpl.OpenTofu)
	require.NoError(t, err)

	assert.Equal(t, "/virtual/cache", cfg.PluginCacheDir)
	assert.True(t, cfg.DisableCheckpoint)
	assert.True(t, cfg.DisableCheckpointSignature)
	assert.Equal(t, []cliconfig.ConfigCredentials{{
		Name:  "registry.example.com",
		Token: "configured-token",
	}}, cfg.Credentials)
	require.NotNil(t, cfg.CredentialsHelpers)
	assert.Equal(t, "example", cfg.CredentialsHelpers.Name)
	assert.Equal(t, []string{"one", "two"}, cfg.CredentialsHelpers.Args)
	assert.Equal(t, []cliconfig.ConfigHost{{
		Name: "registry.example.com",
		Services: map[string]string{
			"providers.v1": "https://registry.example.com/providers/",
		},
	}}, cfg.Hosts)
	require.NotNil(t, cfg.ProviderInstallation)
	require.Len(t, cfg.ProviderInstallation.Methods, 3)

	filesystemMirror, ok := cfg.ProviderInstallation.Methods[0].(*cliconfig.ProviderInstallationFilesystemMirror)
	require.True(t, ok)
	assert.Equal(t, "/virtual/mirror", filesystemMirror.Path)
	assert.Equal(t, []string{"registry.example.com/*/*"}, *filesystemMirror.Include)

	networkMirror, ok := cfg.ProviderInstallation.Methods[1].(*cliconfig.ProviderInstallationNetworkMirror)
	require.True(t, ok)
	assert.Equal(t, "https://mirror.example.com/providers/", networkMirror.URL)
	assert.Equal(t, []string{"registry.example.com/private/*"}, *networkMirror.Exclude)

	direct, ok := cfg.ProviderInstallation.Methods[2].(*cliconfig.ProviderInstallationDirect)
	require.True(t, ok)
	assert.Equal(t, []string{"registry.example.com/*/*"}, *direct.Exclude)
}

func TestLoadUserConfig_JSON(t *testing.T) {
	t.Parallel()

	const home = "/virtual/home"

	configPath := filepath.Join(home, "config.tfrc.json")
	v := userConfigVenv(home, map[string]string{cliconfig.EnvNameTFCLIConfigFile: configPath})

	require.NoError(t, v.FS.MkdirAll(home, 0o755))
	require.NoError(t, vfs.WriteFile(v.FS, configPath, []byte(`{
  "plugin_cache_dir": "/virtual/json-cache",
  "credentials": {
    "registry.example.com": {
      "token": "json-token"
    }
  }
}`), 0o600))

	cfg, err := cliconfig.LoadUserConfig(v, tfimpl.OpenTofu)
	require.NoError(t, err)

	assert.Equal(t, "/virtual/json-cache", cfg.PluginCacheDir)
	assert.Equal(t, []cliconfig.ConfigCredentials{{
		Name:  "registry.example.com",
		Token: "json-token",
	}}, cfg.Credentials)
}

func TestLoadUserConfig_Fragments(t *testing.T) {
	t.Parallel()

	const home = "/virtual/home"

	configDir := filepath.Join(home, ".terraform.d")
	v := userConfigVenv(home, nil)

	require.NoError(t, v.FS.MkdirAll(configDir, 0o755))
	require.NoError(t, vfs.WriteFile(v.FS, filepath.Join(configDir, "10-base.tfrc"), []byte(`
plugin_cache_dir   = "/virtual/first-cache"
disable_checkpoint = true

credentials "registry.example.com" {
  token = "first-token"
}
`), 0o600))
	require.NoError(t, vfs.WriteFile(v.FS, filepath.Join(configDir, "20-override.tfrc"), []byte(`
plugin_cache_dir             = "/virtual/second-cache"
disable_checkpoint_signature = true

credentials "registry.example.com" {
  token = "second-token"
}
`), 0o600))

	cfg, err := cliconfig.LoadUserConfig(v, tfimpl.OpenTofu)
	require.NoError(t, err)

	// plugin_cache_dir is first-wins upstream; credentials are last-wins.
	assert.Equal(t, "/virtual/first-cache", cfg.PluginCacheDir)
	assert.True(t, cfg.DisableCheckpoint)
	assert.True(t, cfg.DisableCheckpointSignature)
	assert.Equal(t, []cliconfig.ConfigCredentials{{
		Name:  "registry.example.com",
		Token: "second-token",
	}}, cfg.Credentials)
}

func TestLoadUserConfig_PluginCacheEnvOverride(t *testing.T) {
	t.Parallel()

	const home = "/virtual/home"

	v := userConfigVenv(home, map[string]string{"TF_PLUGIN_CACHE_DIR": "/virtual/env-cache"})
	configPath := filepath.Join(home, ".tofurc")

	require.NoError(t, v.FS.MkdirAll(home, 0o755))
	require.NoError(t, vfs.WriteFile(v.FS, configPath, []byte(`
plugin_cache_dir = "/virtual/file-cache"
`), 0o600))

	cfg, err := cliconfig.LoadUserConfig(v, tfimpl.OpenTofu)
	require.NoError(t, err)

	assert.Equal(t, "/virtual/env-cache", cfg.PluginCacheDir)
}

func TestLoadUserConfig_MainFileBeatsFragments(t *testing.T) {
	t.Parallel()

	const home = "/virtual/home"

	configDir := filepath.Join(home, ".terraform.d")
	v := userConfigVenv(home, nil)

	require.NoError(t, v.FS.MkdirAll(configDir, 0o755))
	require.NoError(t, vfs.WriteFile(v.FS, filepath.Join(home, ".tofurc"), []byte(`
plugin_cache_dir = "/virtual/main-cache"

credentials "registry.example.com" {
  token = "main-token"
}
`), 0o600))
	require.NoError(t, vfs.WriteFile(v.FS, filepath.Join(configDir, "10-base.tfrc"), []byte(`
plugin_cache_dir = "/virtual/fragment-cache"

credentials "registry.example.com" {
  token = "fragment-token"
}
`), 0o600))

	cfg, err := cliconfig.LoadUserConfig(v, tfimpl.OpenTofu)
	require.NoError(t, err)

	assert.Equal(t, "/virtual/main-cache", cfg.PluginCacheDir)
	assert.Equal(t, []cliconfig.ConfigCredentials{{
		Name:  "registry.example.com",
		Token: "fragment-token",
	}}, cfg.Credentials)
}

func TestLoadUserConfig_DevOverrides(t *testing.T) {
	t.Parallel()

	const home = "/virtual/home"

	v := userConfigVenv(home, nil)

	require.NoError(t, v.FS.MkdirAll(home, 0o755))
	require.NoError(t, vfs.WriteFile(v.FS, filepath.Join(home, ".tofurc"), []byte(`
provider_installation {
  dev_overrides {
    "hashicorp/null" = "/virtual/null-provider"
  }

  direct {}
}
`), 0o600))

	cfg, err := cliconfig.LoadUserConfig(v, tfimpl.OpenTofu)
	require.NoError(t, err)

	// dev_overrides has no equivalent in the generated config, so only direct survives.
	require.Len(t, cfg.ProviderInstallation.Methods, 1)
	assert.IsType(t, &cliconfig.ProviderInstallationDirect{}, cfg.ProviderInstallation.Methods[0])
}

func TestLoadUserConfig_EnvExpansion(t *testing.T) {
	t.Parallel()

	const home = "/virtual/home"

	testCases := []struct {
		name     string
		source   string
		expected string
	}{
		{name: "braced", source: `plugin_cache_dir = "${CACHE_ROOT}/plugins"`, expected: "/virtual/root/plugins"},
		{name: "bare", source: `plugin_cache_dir = "$CACHE_ROOT/plugins"`, expected: "/virtual/root/plugins"},
		{name: "unset", source: `plugin_cache_dir = "${NOT_SET}/plugins"`, expected: "/plugins"},
		{name: "literal", source: `plugin_cache_dir = "/virtual/plain"`, expected: "/virtual/plain"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			v := userConfigVenv(home, map[string]string{"CACHE_ROOT": "/virtual/root"})

			require.NoError(t, v.FS.MkdirAll(home, 0o755))
			require.NoError(t, vfs.WriteFile(v.FS, filepath.Join(home, ".tofurc"), []byte(tc.source), 0o600))

			cfg, err := cliconfig.LoadUserConfig(v, tfimpl.OpenTofu)
			require.NoError(t, err)
			assert.Equal(t, tc.expected, cfg.PluginCacheDir)
		})
	}
}

func TestLoadUserConfig_Errors(t *testing.T) {
	t.Parallel()

	const home = "/virtual/home"

	testCases := []struct {
		expected error
		name     string
		source   string
	}{
		{
			name:     "unparsable",
			source:   "credentials \"registry.example.com\" {\n  token = \"x\"\n",
			expected: cliconfig.ErrUserConfig,
		},
		{
			name:     "invalid credentials hostname",
			source:   "credentials \"not a host\" {\n  token = \"x\"\n}\n",
			expected: cliconfig.ErrInvalidUserConfig,
		},
		{
			name:     "unsupported installation method",
			source:   "provider_installation {\n  bogus_mirror {}\n}\n",
			expected: cliconfig.ErrInvalidUserConfig,
		},
		{
			name:     "two credentials helpers in one file",
			source:   "credentials_helper \"vault\" {\n  args = [\"token\"]\n}\n\ncredentials_helper \"oskeychain\" {\n}\n",
			expected: cliconfig.ErrInvalidUserConfig,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			v := userConfigVenv(home, nil)

			require.NoError(t, v.FS.MkdirAll(home, 0o755))
			require.NoError(t, vfs.WriteFile(v.FS, filepath.Join(home, ".tofurc"), []byte(tc.source), 0o600))

			_, err := cliconfig.LoadUserConfig(v, tfimpl.OpenTofu)
			require.ErrorIs(t, err, tc.expected)
		})
	}
}

func TestUserProviderDir(t *testing.T) {
	t.Parallel()

	t.Run("resolved home", func(t *testing.T) {
		t.Parallel()

		dir, err := cliconfig.UserProviderDir(userConfigVenv("/virtual/home", nil), tfimpl.OpenTofu)
		require.NoError(t, err)
		assert.Equal(t, filepath.Join("/virtual/home", ".terraform.d", "plugins"), dir)
		assert.True(t, filepath.IsAbs(dir))
	})

	// An unresolvable home must not yield a relative path, which the provider cache
	// would resolve against the unit's working directory.
	t.Run("unresolvable home", func(t *testing.T) {
		t.Parallel()

		dir, err := cliconfig.UserProviderDir(userConfigVenv("", nil), tfimpl.OpenTofu)
		require.NoError(t, err)
		assert.Empty(t, dir)
	})
}

// userConfigVenv builds an in-memory venv pinned to a non-Windows platform, so the
// candidate list does not depend on the host the tests happen to run on.
func userConfigVenv(home string, env map[string]string) *venv.Venv {
	if env == nil {
		env = map[string]string{}
	}

	return venvtest.New().
		WithGOOS("linux").
		WithUserHomeDir(func() (string, error) { return home, nil }).
		WithEnv(env)
}

// TestLoadUserConfig_CredentialsHelperAcrossFiles pins that a second helper in a fragment is
// rejected too, rather than silently overriding the one in the main file.
func TestLoadUserConfig_CredentialsHelperAcrossFiles(t *testing.T) {
	t.Parallel()

	const home = "/virtual/home"

	configDir := filepath.Join(home, ".terraform.d")
	v := userConfigVenv(home, nil)

	require.NoError(t, v.FS.MkdirAll(configDir, 0o755))
	require.NoError(t, vfs.WriteFile(v.FS, filepath.Join(home, ".tofurc"), []byte(`
credentials_helper "vault" {
  args = ["token"]
}
`), 0o600))
	require.NoError(t, vfs.WriteFile(v.FS, filepath.Join(configDir, "10-extra.tfrc"), []byte(`
credentials_helper "oskeychain" {
}
`), 0o600))

	_, err := cliconfig.LoadUserConfig(v, tfimpl.OpenTofu)
	require.ErrorIs(t, err, cliconfig.ErrInvalidUserConfig)
}

// TestLoadUserConfigTerraformIgnoresOpenTofuFiles is the regression test for
// https://github.com/gruntwork-io/terragrunt/issues/6787: a stray ~/.tofurc must not
// reconfigure provider installation when the binary being run is Terraform.
func TestLoadUserConfigTerraformIgnoresOpenTofuFiles(t *testing.T) {
	t.Parallel()

	const home = "/virtual/home"

	tofurc := `
provider_installation {
  network_mirror {
    url = "https://registry.terraform.io"
  }

  direct {
    exclude = ["registry.terraform.io/*/*"]
  }
}
`

	newVenv := func(t *testing.T, withTerraformrc bool) *venv.Venv {
		t.Helper()

		v := userConfigVenv(home, map[string]string{"XDG_CONFIG_HOME": "/virtual/config"})

		require.NoError(t, v.FS.MkdirAll(home, 0o755))
		require.NoError(t, vfs.WriteFile(v.FS, filepath.Join(home, ".tofurc"), []byte(tofurc), 0o600))

		if withTerraformrc {
			require.NoError(t, vfs.WriteFile(v.FS, filepath.Join(home, ".terraformrc"), []byte(`
plugin_cache_dir = "/virtual/tf-cache"
`), 0o600))
		}

		return v
	}

	t.Run("terraform loads nothing from .tofurc", func(t *testing.T) {
		t.Parallel()

		cfg, err := cliconfig.LoadUserConfig(newVenv(t, false), tfimpl.Terraform)
		require.NoError(t, err)

		assert.Empty(t, cfg.ProviderInstallation.Methods)
	})

	t.Run("terraform reads .terraformrc when both files exist", func(t *testing.T) {
		t.Parallel()

		cfg, err := cliconfig.LoadUserConfig(newVenv(t, true), tfimpl.Terraform)
		require.NoError(t, err)

		assert.Empty(t, cfg.ProviderInstallation.Methods)
		assert.Equal(t, "/virtual/tf-cache", cfg.PluginCacheDir)
	})

	t.Run("tofu keeps reading .tofurc", func(t *testing.T) {
		t.Parallel()

		cfg, err := cliconfig.LoadUserConfig(newVenv(t, true), tfimpl.OpenTofu)
		require.NoError(t, err)

		require.Len(t, cfg.ProviderInstallation.Methods, 2)
		assert.Empty(t, cfg.PluginCacheDir)
	})
}

// TestLoadUserConfigTerraformKeepsLegacyFragments pins v1.1.3 parity: a Terraform run still
// reads ~/.terraform.d/*.tfrc fragments even when XDG_CONFIG_HOME is set.
func TestLoadUserConfigTerraformKeepsLegacyFragments(t *testing.T) {
	t.Parallel()

	const home = "/virtual/home"

	configDir := filepath.Join(home, ".terraform.d")
	v := userConfigVenv(home, map[string]string{"XDG_CONFIG_HOME": "/virtual/config"})

	require.NoError(t, v.FS.MkdirAll(configDir, 0o755))
	require.NoError(t, vfs.WriteFile(v.FS, filepath.Join(configDir, "cache.tfrc"), []byte(`
plugin_cache_dir = "/virtual/fragment-cache"
`), 0o600))

	cfg, err := cliconfig.LoadUserConfig(v, tfimpl.Terraform)
	require.NoError(t, err)

	assert.Equal(t, "/virtual/fragment-cache", cfg.PluginCacheDir)
}

// TestLoadUserConfigOverrideWinsForTerraform: TF_CLI_CONFIG_FILE names the file outright,
// so a Terraform run reads it even when it is not ~/.terraformrc.
func TestLoadUserConfigOverrideWinsForTerraform(t *testing.T) {
	t.Parallel()

	const home = "/virtual/home"

	override := "/virtual/custom/cli.tfrc"
	v := userConfigVenv(home, map[string]string{cliconfig.EnvNameTFCLIConfigFile: override})

	require.NoError(t, v.FS.MkdirAll(home, 0o755))
	require.NoError(t, vfs.WriteFile(v.FS, filepath.Join(home, ".terraformrc"), []byte(`
plugin_cache_dir = "/virtual/home-cache"
`), 0o600))
	require.NoError(t, vfs.WriteFile(v.FS, override, []byte(`
plugin_cache_dir = "/virtual/override-cache"
`), 0o600))

	cfg, err := cliconfig.LoadUserConfig(v, tfimpl.Terraform)
	require.NoError(t, err)

	assert.Equal(t, "/virtual/override-cache", cfg.PluginCacheDir)
}

// TestUserProviderDirTerraform: the user plugins directory stays under ~/.terraform.d for
// Terraform even when XDG_CONFIG_HOME points elsewhere.
func TestUserProviderDirTerraform(t *testing.T) {
	t.Parallel()

	v := userConfigVenv("/virtual/home", map[string]string{"XDG_CONFIG_HOME": "/virtual/config"})

	dir, err := cliconfig.UserProviderDir(v, tfimpl.Terraform)
	require.NoError(t, err)
	assert.Equal(t, filepath.Join("/virtual/home", ".terraform.d", "plugins"), dir)
}
