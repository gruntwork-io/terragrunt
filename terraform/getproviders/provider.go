package getproviders

import (
	"context"
)

type Providers []Provider

type Provider interface {
	Address() string

	Version() string

	DocumentSHA256Sums(ctx context.Context) ([]byte, error)

	Signature(ctx context.Context) ([]byte, error)

	PackageDir() string
}
