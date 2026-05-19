// Package tfimpl defines the Terraform implementation type constants.
package tfimpl

import "os"

// Type represents which Terraform implementation is being used.
type Type string

const (
	// Terraform indicates the HashiCorp Terraform binary.
	Terraform Type = "terraform"
	// OpenTofu indicates the OpenTofu binary.
	OpenTofu Type = "tofu"
	// Unknown indicates an unrecognized implementation.
	Unknown Type = "unknown"
)

// Default registry hosts used when a tfr:// URL omits its host.
const (
	defaultRegistryDomain   = "registry.terraform.io"
	defaultOtRegistryDomain = "registry.opentofu.org"
	// defaultRegistryEnvName overrides the default registry host at runtime.
	defaultRegistryEnvName = "TG_TF_DEFAULT_REGISTRY_HOST"
)

// DefaultRegistryDomain returns the registry host to use when a tfr://
// source URL omits its host.
//
// The TG_TF_DEFAULT_REGISTRY_HOST env var wins if set; otherwise the choice
// follows impl: OpenTofu → registry.opentofu.org, anything else →
// registry.terraform.io.
func DefaultRegistryDomain(impl Type) string {
	if v := os.Getenv(defaultRegistryEnvName); v != "" {
		return v
	}

	if impl == OpenTofu {
		return defaultOtRegistryDomain
	}

	return defaultRegistryDomain
}
