// Package options groups provider-cache-specific configuration that is
// resolved at startup and shared with the ProviderCache server and hook
// functions.  It lives in its own package so that both pkg/options and
// internal/providercache can import it without creating a cycle.
package options

// DefaultRegistryNames is the default set of remote registries cached by the
// Terragrunt Provider Cache server.
var DefaultRegistryNames = []string{
	"registry.terraform.io",
	"registry.opentofu.org",
}

// ProviderCacheOptions holds provider-cache-specific configuration that was
// previously spread across several fields on TerragruntOptions.
type ProviderCacheOptions struct {
	Dir           string
	Hostname      string
	Token         string
	RegistryNames []string
	Port          int
	Enabled       bool
}
