package cliconfig

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestConfig(t *testing.T) {
	t.Parallel()

	tempCacheDir, err := os.MkdirTemp("", "*")
	assert.NoError(t, err)

	testCases := []struct {
		filesystemMethod *ProviderInstallationFilesystemMirror
		directMethod     *ProviderInstallationDirect
		hosts            map[string]map[string]string
		config           *Config
		expectedHCL      string
	}{
		{
			filesystemMethod: NewProviderInstallationFilesystemMirror(tempCacheDir, []string{"registry.terraform.io/*/*", "registry.opentofu.org/*/*"}, nil),
			directMethod:     NewProviderInstallationDirect([]string{"registry.terraform.io/*/*", "registry.opentofu.org/*/*"}, nil),
			hosts: map[string]map[string]string{
				"registry.terraform.io": map[string]string{
					"providers.v1": "http://localhost:5758/v1/providers/registry.terraform.io/",
				},
			},
			config: &Config{
				rawHCL: []byte(`
disable_checkpoint = true
plugin_cache_dir   = "path/to/plugin/cache/dir"`),
			},
			expectedHCL: `
disable_checkpoint = true
plugin_cache_dir = ""

host "registry.terraform.io" {
  services = {
    "providers.v1" = "http://localhost:5758/v1/providers/registry.terraform.io/"
  }
}

provider_installation {

  filesystem_mirror {
    path    = "` + tempCacheDir + `"
    include = ["registry.terraform.io/*/*", "registry.opentofu.org/*/*"]
  }

  direct {
    include = ["registry.terraform.io/*/*", "registry.opentofu.org/*/*"]
  }
}
`,
		},
		{
			config: &Config{
				rawHCL: []byte(`
disable_checkpoint = true
plugin_cache_dir   = "path/to/plugin/cache/dir"`),
				PluginCacheDir: tempCacheDir,
			},
			expectedHCL: `
disable_checkpoint = true
plugin_cache_dir = "` + tempCacheDir + `"
`,
		},
	}

	for i, testCase := range testCases {
		testCase := testCase

		t.Run(fmt.Sprintf("testCase-%d", i), func(t *testing.T) {
			t.Parallel()

			tempDir, err := os.MkdirTemp("", "*")
			assert.NoError(t, err)
			configFile := filepath.Join(tempDir, ".terraformrc")

			for host, service := range testCase.hosts {
				testCase.config.AddHost(host, service)
			}
			testCase.config.SetProviderInstallation(testCase.filesystemMethod, testCase.directMethod)

			err = testCase.config.Save(configFile)
			assert.NoError(t, err)

			hclBytes, err := os.ReadFile(configFile)
			assert.NoError(t, err)

			assert.Equal(t, testCase.expectedHCL, string(hclBytes))
		})
	}
}
