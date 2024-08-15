// Package providers defines the generic interface for credential providers.
package providers

import (
	"context"
)

const (
	// AWSCredentials is the name of the AWS credentials provider.
	AWSCredentials CredentialsName = "AWS"
)

// CredentialsName is the name of a set of credentials.
type CredentialsName string

// Credentials represents a set of credentials.
type Credentials struct {
	Name CredentialsName
	Envs map[string]string
}

// Provider is the generic interface for credential providers.
type Provider interface {
	// Name returns the name of the provider.
	Name() string
	// GetCredentials returns a set of credentials.
	GetCredentials(ctx context.Context) (*Credentials, error)
}
