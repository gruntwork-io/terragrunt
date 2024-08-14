package cliconfig_test

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/gruntwork-io/terragrunt/terraform/cliconfig"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConfig(t *testing.T) {
	t.Parallel()

	var (
		include = []string{"registry.terraform.io/*/*"}
		exclude = []string{"registry.opentofu.org/*/*"}
	)

	tempCacheDir, err := os.MkdirTemp("", "*")
	require.NoError(t, err)

	testCases := []struct {
		providerInstallationMethods []cliconfig.ProviderInstallationMethod
		hosts                       []cliconfig.ConfigHost
		config                      cliconfig.Config
		expectedHCL                 string
	}{
		{
			providerInstallationMethods: []cliconfig.ProviderInstallationMethod{
				cliconfig.NewProviderInstallationFilesystemMirror(tempCacheDir, include, exclude),
				cliconfig.NewProviderInstallationNetworkMirror("https://network-mirror.io/providers/", include, exclude),
				cliconfig.NewProviderInstallationDirect(include, exclude),
			},
			hosts: []cliconfig.ConfigHost{
				{"registry.terraform.io", map[string]string{"providers.v1": "http://localhost:5758/v1/providers/registry.terraform.io/"}},
			},
			config: cliconfig.Config{
				DisableCheckpoint: true,
				PluginCacheDir:    "path/to/plugin/cache/dir1",
			},
			expectedHCL: `disable_checkpoint           = true
disable_checkpoint_signature = false
plugin_cache_dir             = "path/to/plugin/cache/dir1"

host "registry.terraform.io" {
  services = {
    "providers.v1" = "http://localhost:5758/v1/providers/registry.terraform.io/"
  }
}

provider_installation {

   "filesystem_mirror" {
    path    = "` + tempCacheDir + `"
    include = ["registry.terraform.io/*/*"]
    exclude = ["registry.opentofu.org/*/*"]
  }
   "network_mirror" {
    url     = "https://network-mirror.io/providers/"
    include = ["registry.terraform.io/*/*"]
    exclude = ["registry.opentofu.org/*/*"]
  }
   "direct" {
    include = ["registry.terraform.io/*/*"]
    exclude = ["registry.opentofu.org/*/*"]
  }
}
`,
		},
		{
			config: cliconfig.Config{
				DisableCheckpoint: false,
				PluginCacheDir:    tempCacheDir,
			},
			expectedHCL: `disable_checkpoint           = false
disable_checkpoint_signature = false
plugin_cache_dir             = "` + tempCacheDir + `"

provider_installation {
}
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

			err = testCase.config.Save(configFile)
			require.NoError(t, err)

			hclBytes, err := os.ReadFile(configFile)
			require.NoError(t, err)

			assert.Equal(t, testCase.expectedHCL, string(hclBytes))
		})
	}
}
