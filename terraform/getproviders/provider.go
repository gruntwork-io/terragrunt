//go:generate mockery --name Provider

package getproviders

import (
	"context"
)

type Providers []Provider

type Provider interface {
	// Address returns a source address of the provider. e.g.: registry.terraform.io/hashicorp/aws
	Address() string

	// Version returns a version of the provider. e.g.: 5.36.0
	Version() string

	// DocumentSHA256Sums returns a document with providers hashes for different platforms.
	DocumentSHA256Sums(ctx context.Context) ([]byte, error)

	// PackageDir returns a directory with the unpacked provider.
	PackageDir() string
}
