package providercache_test

import (
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/providercache"
	"github.com/gruntwork-io/terragrunt/internal/tf/cliconfig"
	"github.com/gruntwork-io/terragrunt/internal/tfimpl"
	"github.com/stretchr/testify/assert"
)

func TestFilterRegistriesByImplementation(t *testing.T) {
	t.Parallel()

	defaultRegistries := []string{"registry.terraform.io", "registry.opentofu.org"}

	tests := []struct {
		name           string
		registryNames  []string
		implementation tfimpl.Type
		expected       []string
	}{
		{
			name:           "defaults + OpenTofu returns only opentofu registry",
			registryNames:  defaultRegistries,
			implementation: tfimpl.OpenTofu,
			expected:       []string{"registry.opentofu.org"},
		},
		{
			name:           "defaults + Terraform returns only terraform registry",
			registryNames:  defaultRegistries,
			implementation: tfimpl.Terraform,
			expected:       []string{"registry.terraform.io"},
		},
		{
			name:           "defaults + Unknown returns both",
			registryNames:  defaultRegistries,
			implementation: tfimpl.Unknown,
			expected:       defaultRegistries,
		},
		{
			name:           "user-replaced list returned as-is for OpenTofu",
			registryNames:  []string{"registry.terraform.io"},
			implementation: tfimpl.OpenTofu,
			expected:       []string{"registry.terraform.io"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := providercache.FilterRegistriesByImplementation(tt.registryNames, tt.implementation)

			assert.Equal(t, tt.expected, got)
		})
	}
}

func TestFilterRegistriesByImplementationWithCustomHosts(t *testing.T) {
	t.Parallel()

	withCustom := []string{"registry.terraform.io", "registry.opentofu.org", "nexus.corp"}

	tests := []struct {
		name           string
		implementation tfimpl.Type
		expected       []string
	}{
		{
			name:           "custom host + OpenTofu: only opentofu + custom",
			implementation: tfimpl.OpenTofu,
			expected:       []string{"registry.opentofu.org", "nexus.corp"},
		},
		{
			name:           "custom host + Terraform: only terraform + custom",
			implementation: tfimpl.Terraform,
			expected:       []string{"registry.terraform.io", "nexus.corp"},
		},
		{
			name:           "custom host + Unknown: all three",
			implementation: tfimpl.Unknown,
			expected:       withCustom,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// Simulate what Init does: standard registries already in opts, custom host added separately.
			// FilterRegistriesByImplementation must NOT receive custom hosts mixed in — it receives
			// pc.opts.RegistryNames which stays clean; custom hosts come from cliCfg.Hosts.
			baseRegistries := []string{"registry.terraform.io", "registry.opentofu.org"}
			customHosts := []cliconfig.ConfigHost{{Name: "nexus.corp"}}

			filtered := providercache.FilterRegistriesByImplementation(baseRegistries, tt.implementation)
			got := providercache.AppendCustomHostRegistries(customHosts, filtered)

			assert.Equal(t, tt.expected, got)
		})
	}
}
