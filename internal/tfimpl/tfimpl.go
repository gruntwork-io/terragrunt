// Package tfimpl defines the Terraform implementation type constants.
package tfimpl

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
