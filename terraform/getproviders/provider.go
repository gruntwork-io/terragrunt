//go:generate mockery --name Provider

// Package getproviders provides an interface for getting providers.
package getproviders

import (
	"context"

	"github.com/gruntwork-io/terragrunt/pkg/log"
)

type Provider interface {
	// Address returns a source address of the provider. e.g.: registry.terraform.io/hashicorp/aws
	Address() string

	// Version returns a version of the provider. e.g.: 5.36.0
	Version() string

	// DocumentSHA256Sums returns a document with providers hashes for different platforms.
	DocumentSHA256Sums(ctx context.Context) ([]byte, error)

	// PackageDir returns a directory with the unpacked provider.
	PackageDir() string

	// Logger returns logger
	Logger() log.Logger
}
