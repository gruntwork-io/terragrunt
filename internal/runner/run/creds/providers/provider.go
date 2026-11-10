// Package providers defines the interface for a provider.
package providers

import (
	"context"

	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/pkg/log"
)

const (
	// AWSCredentials names credentials that set the AWS access key environment variables.
	AWSCredentials CredentialsName = "AWS"
)

// CredentialsName identifies the kind of credentials a [Credentials] holds, such as [AWSCredentials].
type CredentialsName string

// Credentials is a set of environment variables that authenticate as one kind of identity.
type Credentials struct {
	Envs map[string]string
	Name CredentialsName
}

// Provider obtains credentials from one source, such as an external command or STS role assumption.
type Provider interface {
	// Name returns the name of the provider.
	Name() string
	// GetCredentials returns a set of credentials.
	GetCredentials(ctx context.Context, l log.Logger, v *venv.Venv) (*Credentials, error)
}
