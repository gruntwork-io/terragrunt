package cliconfig_test

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/gruntwork-io/terragrunt/tf/cliconfig"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConfig(t *testing.T) {
	t.Parallel()

	var (
		include = []string{"registry.terraform.io/*/*"}
		exclude = []string{"registry.opentofu.org/*/*"}
	)

	tempCacheDir := t.TempDir()

	testCases := []struct {
		expectedHCL                 string
		providerInstallationMethods []cliconfig.ProviderInstallationMethod
		hosts                       []cliconfig.ConfigHost
		config                      cliconfig.Config
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
			config: cliconfig.Config{
				DisableCheckpoint: true,
				PluginCacheDir:    "path/to/plugin/cache/dir1",
			},
			expectedHCL: `
provider_installation {

   "filesystem_mirror" {
    include = ["registry.terraform.io/*/*"]
    exclude = ["registry.opentofu.org/*/*"]
    path    = "` + tempCacheDir + `"
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
			config: cliconfig.Config{
				DisableCheckpoint: false,
				PluginCacheDir:    tempCacheDir,
			},
			expectedHCL: `
provider_installation {
}

plugin_cache_dir             = "` + tempCacheDir + `"
disable_checkpoint           = false
disable_checkpoint_signature = false
`,
		},
	}

	for i, testCase := range testCases {
		testCase := testCase

		t.Run(fmt.Sprintf("testCase-%d", i), func(t *testing.T) {
			t.Parallel()

			tempDir := t.TempDir()
			configFile := filepath.Join(tempDir, ".terraformrc")

			for _, host := range testCase.hosts {
				testCase.config.AddHost(host.Name, host.Services)
			}
			testCase.config.AddProviderInstallationMethods(testCase.providerInstallationMethods...)

			err := testCase.config.Save(configFile)
			require.NoError(t, err)

			hclBytes, err := os.ReadFile(configFile)
			require.NoError(t, err)

			assert.Equal(t, testCase.expectedHCL, string(hclBytes))
		})
	}
}
