// Package providers defines the interface for a provider.
package providers

import (
	"context"

	"github.com/gruntwork-io/terragrunt/pkg/log"
)

const (
	AWSCredentials CredentialsName = "AWS"
)

type CredentialsName string

type Credentials struct {
	Envs map[string]string
	Name CredentialsName
}

type Provider interface {
	// Name returns the name of the provider.
	Name() string
	// GetCredentials returns a set of credentials.
	GetCredentials(ctx context.Context, l log.Logger) (*Credentials, error)
}
