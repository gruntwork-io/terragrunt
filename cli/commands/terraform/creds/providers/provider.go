package providers

import (
	"context"
)

const (
	AWSCredentials CredentialsName = "AWS"
)

type CredentialsName string

type Credentials struct {
	Name CredentialsName
	Envs map[string]string
}

type Provider interface {
	// Name returns the name of the provider.
	Name() string
	// GetCredentials returns a set of credentials.
	GetCredentials(ctx context.Context) (*Credentials, error)
}
