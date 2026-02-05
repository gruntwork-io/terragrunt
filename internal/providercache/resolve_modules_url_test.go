package providercache_test

import (
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/providercache"
	"github.com/stretchr/testify/assert"
)

func TestResolveModulesURL(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name         string
		registryName string
		modulesV1    string
		expected     string
	}{
		{
			name:         "relative path",
			registryName: "registry.terraform.io",
			modulesV1:    "/v1/modules",
			expected:     "https://registry.terraform.io/v1/modules",
		},
		{
			name:         "relative path with trailing slash",
			registryName: "private.registry.com",
			modulesV1:    "/custom/modules/",
			expected:     "https://private.registry.com/custom/modules/",
		},
		{
			name:         "absolute URL same host",
			registryName: "packages.syncron.team",
			modulesV1:    "https://packages.syncron.team/somepath/modules/",
			expected:     "https://packages.syncron.team/somepath/modules/",
		},
		{
			name:         "absolute URL different host",
			registryName: "registry.example.com",
			modulesV1:    "https://other.host.com/modules/v1/",
			expected:     "https://other.host.com/modules/v1/",
		},
		{
			name:         "absolute URL with http scheme",
			registryName: "registry.example.com",
			modulesV1:    "http://internal.host.com/modules/",
			expected:     "http://internal.host.com/modules/",
		},
		{
			name:         "empty path",
			registryName: "registry.terraform.io",
			modulesV1:    "",
			expected:     "https://registry.terraform.io",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			result := providercache.ResolveModulesURL(tc.registryName, tc.modulesV1)
			assert.Equal(t, tc.expected, result)
		})
	}
}
