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
	// Name returns provdier name.
	Name() string
	// GetCredentials returns a credentials.
	GetCredentials(ctx context.Context) (*Credentials, error)
}
