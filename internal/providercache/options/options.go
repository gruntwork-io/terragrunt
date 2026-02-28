// Package options defines the ProviderCacheOptions struct used to configure the Terragrunt provider cache.
package options

// DefaultRegistryNames contains the default list of registries cached by the provider cache server.
var DefaultRegistryNames = []string{
	"registry.terraform.io",
	"registry.opentofu.org",
}

// ProviderCacheOptions groups all provider-cache-related configuration fields.
type ProviderCacheOptions struct {
	Dir           string
	Hostname      string
	Token         string
	RegistryNames []string
	Port          int
	Enabled       bool
}
