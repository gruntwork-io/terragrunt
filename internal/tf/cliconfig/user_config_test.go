package cliconfig_test

import (
	"path/filepath"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/tf/cliconfig"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/gruntwork-io/terragrunt/test/helpers/venvtest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadUserConfig(t *testing.T) {
	t.Parallel()

	const home = "/virtual/home"

	v := venvtest.New().WithUserHomeDir(func() (string, error) { return home, nil })
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

	cfg, err := cliconfig.LoadUserConfig(v)
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
