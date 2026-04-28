package cliconfig_test

import (
	"fmt"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/tf/cliconfig"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAddHost(t *testing.T) {
	t.Parallel()

	t.Run("new host is appended", func(t *testing.T) {
		t.Parallel()

		cfg := cliconfig.NewConfig()
		cfg.AddHost("registry.example.com", map[string]string{
			"providers.v1": "https://registry.example.com/v1/providers/",
		})

		require.Len(t, cfg.Hosts, 1)
		assert.Equal(t, "registry.example.com", cfg.Hosts[0].Name)
		assert.Equal(t, "https://registry.example.com/v1/providers/", cfg.Hosts[0].Services["providers.v1"])
	})

	t.Run("existing host services are merged", func(t *testing.T) {
		t.Parallel()

		cfg := cliconfig.NewConfig()
		cfg.AddHost("registry.example.com", map[string]string{
			"modules.v1": "https://registry.example.com/v1/modules/",
		})
		cfg.AddHost("registry.example.com", map[string]string{
			"providers.v1": "https://registry.example.com/v1/providers/",
		})

		require.Len(t, cfg.Hosts, 1, "should not create a duplicate host entry")
		assert.Equal(t, "https://registry.example.com/v1/modules/", cfg.Hosts[0].Services["modules.v1"], "original service preserved")
		assert.Equal(t, "https://registry.example.com/v1/providers/", cfg.Hosts[0].Services["providers.v1"], "new service added")
	})

	t.Run("overlapping service key is overwritten by new value", func(t *testing.T) {
		t.Parallel()

		cfg := cliconfig.NewConfig()
		cfg.AddHost("registry.example.com", map[string]string{
			"providers.v1": "https://old-url.example.com/v1/providers/",
			"modules.v1":   "https://registry.example.com/v1/modules/",
		})
		cfg.AddHost("registry.example.com", map[string]string{
			"providers.v1": "https://new-url.example.com/v1/providers/",
		})

		require.Len(t, cfg.Hosts, 1)
		assert.Equal(t, "https://new-url.example.com/v1/providers/", cfg.Hosts[0].Services["providers.v1"], "overlapping key should be overwritten")
		assert.Equal(t, "https://registry.example.com/v1/modules/", cfg.Hosts[0].Services["modules.v1"], "non-overlapping key should be preserved")
	})

	t.Run("multiple different hosts are all appended", func(t *testing.T) {
		t.Parallel()

		cfg := cliconfig.NewConfig()
		cfg.AddHost("registry.terraform.io", map[string]string{"providers.v1": "http://localhost/tf/"})
		cfg.AddHost("registry.opentofu.org", map[string]string{"providers.v1": "http://localhost/opentofu/"})

		require.Len(t, cfg.Hosts, 2)
	})
}

func TestConfig(t *testing.T) {
	t.Parallel()

	var (
		include = []string{"registry.terraform.io/*/*"}
		exclude = []string{"registry.opentofu.org/*/*"}
	)

	// Use a fixed path for the cache dir since we're using an in-memory filesystem
	tempCacheDir := "/tmp/provider-cache"
	testCases := []struct {
		config                      *cliconfig.Config
		expectedHCL                 string
		providerInstallationMethods []cliconfig.ProviderInstallationMethod
		hosts                       []cliconfig.ConfigHost
	}{
		{
			providerInstallationMethods: []cliconfig.ProviderInstallationMethod{
				cliconfig.NewProviderInstallationFilesystemMirror(tempCacheDir, include, exclude),
				cliconfig.NewProviderInstallationNetworkMirror("https://network-mirror.io/providers/", include, exclude),
				cliconfig.NewProviderInstallationDirect(include, exclude),
			},
			hosts: []cliconfig.ConfigHost{
				{Name: "registry.terraform.io", Services: map[string]string{"providers.v1": "http://localhost:5758/v1/providers/registry.terraform.io/"}},
			},
			config: cliconfig.NewConfig().
				WithDisableCheckpoint().
				WithPluginCacheDir("path/to/plugin/cache/dir1"),
			expectedHCL: `
provider_installation {

   "filesystem_mirror" {
    include = ["registry.terraform.io/*/*"]
    exclude = ["registry.opentofu.org/*/*"]
    path    = "/tmp/provider-cache"
  }
   "network_mirror" {
    include = ["registry.terraform.io/*/*"]
    exclude = ["registry.opentofu.org/*/*"]
    url     = "https://network-mirror.io/providers/"
  }
   "direct" {
    include = ["registry.terraform.io/*/*"]
    exclude = ["registry.opentofu.org/*/*"]
  }
}

plugin_cache_dir = "path/to/plugin/cache/dir1"

host "registry.terraform.io" {
  services = {
    "providers.v1" = "http://localhost:5758/v1/providers/registry.terraform.io/"
  }
}

disable_checkpoint           = true
disable_checkpoint_signature = false
`,
		},
		{
			config: cliconfig.NewConfig().
				WithPluginCacheDir(tempCacheDir),
			expectedHCL: `
provider_installation {
}

plugin_cache_dir             = "/tmp/provider-cache"
disable_checkpoint           = false
disable_checkpoint_signature = false
`,
		},
	}

	for i, tc := range testCases {
		t.Run(fmt.Sprintf("testCase-%d", i), func(t *testing.T) {
			t.Parallel()

			// Use an in-memory filesystem for faster, isolated tests
			memFs := vfs.NewMemMapFS()
			configFile := "/config/.terraformrc"

			for _, host := range tc.hosts {
				tc.config.AddHost(host.Name, host.Services)
			}

			tc.config.AddProviderInstallationMethods(tc.providerInstallationMethods...)

			// Inject filesystem via options - same Save() method as production
			tc.config.WithOptions(cliconfig.WithFS(memFs))

			err := tc.config.Save(configFile)
			require.NoError(t, err)

			hclBytes, err := vfs.ReadFile(memFs, configFile)
			require.NoError(t, err)

			actualHCL := string(hclBytes)
			assert.Equal(t, tc.expectedHCL, actualHCL)
		})
	}
}
