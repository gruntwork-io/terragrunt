// Package gcphelper provides helper functions for working with GCP services.
package gcphelper

import (
	"context"
	"encoding/json"

	"cloud.google.com/go/storage"
	"github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/gruntwork-io/terragrunt/internal/util"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/jwt"
	"google.golang.org/api/impersonate"
	"google.golang.org/api/option"
)

const (
	tokenURL = "https://oauth2.googleapis.com/token"
)

// GCPSessionConfig is a representation of the configuration options for a GCP Config
type GCPSessionConfig struct {
	Credentials                        string
	AccessToken                        string
	ImpersonateServiceAccount          string
	ImpersonateServiceAccountDelegates []string
}

// GCPConfigBuilder builds GCP client options using the builder pattern.
// Use NewGCPConfigBuilder to create, chain With* methods for optional parameters, then call Build().
type GCPConfigBuilder struct {
	sessionConfig *GCPSessionConfig
	env           map[string]string
}

// NewGCPConfigBuilder creates a new builder for GCP config.
func NewGCPConfigBuilder() *GCPConfigBuilder {
	return &GCPConfigBuilder{
		env: make(map[string]string),
	}
}

// WithSessionConfig sets the GCP session configuration (credentials, access token, impersonation, etc.).
func (b *GCPConfigBuilder) WithSessionConfig(cfg *GCPSessionConfig) *GCPConfigBuilder {
	b.sessionConfig = cfg
	return b
}

// WithEnv sets environment variables used for credential resolution.
func (b *GCPConfigBuilder) WithEnv(env map[string]string) *GCPConfigBuilder {
	b.env = env
	return b
}

// Build creates the GCP client options from the builder's configuration.
func (b *GCPConfigBuilder) Build(ctx context.Context) ([]option.ClientOption, error) {
	var clientOpts []option.ClientOption

	if envCreds := createGCPCredentialsFromEnv(b.env); envCreds != nil {
		clientOpts = append(clientOpts, envCreds)
	} else if b.sessionConfig != nil && b.sessionConfig.Credentials != "" {
		// Use credentials file from config
		clientOpts = append(clientOpts, option.WithCredentialsFile(b.sessionConfig.Credentials))
	} else if b.sessionConfig != nil && b.sessionConfig.AccessToken != "" {
		// Use access token from config
		tokenSource := oauth2.StaticTokenSource(&oauth2.Token{
			AccessToken: b.sessionConfig.AccessToken,
		})
		clientOpts = append(clientOpts, option.WithTokenSource(tokenSource))
	} else if oauthAccessToken := b.env["GOOGLE_OAUTH_ACCESS_TOKEN"]; oauthAccessToken != "" {
		// Use OAuth access token from environment
		tokenSource := oauth2.StaticTokenSource(&oauth2.Token{
			AccessToken: oauthAccessToken,
		})
		clientOpts = append(clientOpts, option.WithTokenSource(tokenSource))
	} else if b.env["GOOGLE_CREDENTIALS"] != "" {
		// Use GOOGLE_CREDENTIALS from environment (can be file path or JSON content)
		clientOpt, err := createGCPCredentialsFromGoogleCredentialsEnv(ctx, b.env)
		if err != nil {
			return nil, err
		}

		if clientOpt != nil {
			clientOpts = append(clientOpts, clientOpt)
		}
	}

	// Handle service account impersonation
	if b.sessionConfig != nil && b.sessionConfig.ImpersonateServiceAccount != "" {
		ts, err := impersonate.CredentialsTokenSource(ctx, impersonate.CredentialsConfig{
			TargetPrincipal: b.sessionConfig.ImpersonateServiceAccount,
			Scopes:          []string{storage.ScopeFullControl},
			Delegates:       b.sessionConfig.ImpersonateServiceAccountDelegates,
		})
		if err != nil {
			return nil, errors.Errorf("Error creating impersonation token source: %w", err)
		}

		clientOpts = append(clientOpts, option.WithTokenSource(ts))
	}

	return clientOpts, nil
}

// BuildGCSClient creates a GCS storage client from the builder's configuration.
func (b *GCPConfigBuilder) BuildGCSClient(ctx context.Context) (*storage.Client, error) {
	clientOpts, err := b.Build(ctx)
	if err != nil {
		return nil, err
	}

	gcsClient, err := storage.NewClient(ctx, clientOpts...)
	if err != nil {
		return nil, errors.Errorf("Error creating GCS client: %w", err)
	}

	return gcsClient, nil
}

// createGCPCredentialsFromEnv creates GCP credentials from GOOGLE_APPLICATION_CREDENTIALS environment variable.
// It looks for GOOGLE_APPLICATION_CREDENTIALS and returns a ClientOption that can be used
// with Google Cloud clients. Returns nil if the environment variable is not set.
func createGCPCredentialsFromEnv(env map[string]string) option.ClientOption {
	if len(env) == 0 {
		return nil
	}

	credentialsFile := env["GOOGLE_APPLICATION_CREDENTIALS"]
	if credentialsFile == "" {
		return nil
	}

	return option.WithCredentialsFile(credentialsFile)
}

// createGCPCredentialsFromGoogleCredentialsEnv creates GCP credentials from GOOGLE_CREDENTIALS environment variable.
// This can be either a file path or the JSON content directly (to mirror how Terraform works).
func createGCPCredentialsFromGoogleCredentialsEnv(ctx context.Context, env map[string]string) (option.ClientOption, error) {
	var account = struct {
		PrivateKeyID string `json:"private_key_id"`
		PrivateKey   string `json:"private_key"`
		ClientEmail  string `json:"client_email"`
		ClientID     string `json:"client_id"`
	}{}

	// to mirror how Terraform works, we have to accept either the file path or the contents
	creds := env["GOOGLE_CREDENTIALS"]

	contents, err := util.FileOrData(creds)
	if err != nil {
		return nil, errors.Errorf("Error loading credentials: %w", err)
	}

	if err := json.Unmarshal([]byte(contents), &account); err != nil {
		return nil, errors.Errorf("Error parsing GCP credentials.")
	}

	conf := jwt.Config{
		Email:      account.ClientEmail,
		PrivateKey: []byte(account.PrivateKey),
		// We need the FullControl scope to be able to add metadata such as labels
		Scopes:   []string{storage.ScopeFullControl},
		TokenURL: tokenURL,
	}

	return option.WithHTTPClient(conf.Client(ctx)), nil
}
