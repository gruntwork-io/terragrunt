package cliconfig_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
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
	// Normalize paths to forward slashes for consistent comparison across platforms
	normalizedTempCacheDir := filepath.Clean(tempCacheDir)
	// replace backslashes with double forward slashes to match windows HCL representation
	normalizedTempCacheDir = strings.ReplaceAll(normalizedTempCacheDir, "\\", "//")
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
    path    = "` + normalizedTempCacheDir + `"
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

plugin_cache_dir             = "` + normalizedTempCacheDir + `"
disable_checkpoint           = false
disable_checkpoint_signature = false
`,
		},
	}

	for i, tc := range testCases {
		t.Run(fmt.Sprintf("testCase-%d", i), func(t *testing.T) {
			t.Parallel()

			tempDir := t.TempDir()
			configFile := filepath.Join(tempDir, ".terraformrc")

			for _, host := range tc.hosts {
				tc.config.AddHost(host.Name, host.Services)
			}

			tc.config.AddProviderInstallationMethods(tc.providerInstallationMethods...)

			err := tc.config.Save(configFile)
			require.NoError(t, err)

			hclBytes, err := os.ReadFile(configFile)
			require.NoError(t, err)

			// Normalize the actual output paths to forward slashes for comparison
			actualHCL := filepath.ToSlash(string(hclBytes))
			assert.Equal(t, tc.expectedHCL, actualHCL)
		})
	}
}
