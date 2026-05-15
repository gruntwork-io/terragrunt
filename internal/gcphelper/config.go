// Package gcphelper provides helper functions for working with GCP services.
package gcphelper

import (
	"context"
	"encoding/json"
	"net/http"
	"os"

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

	credTypeServiceAccount             = "service_account"
	credTypeAuthorizedUser             = "authorized_user"
	credTypeImpersonatedServiceAccount = "impersonated_service_account"
	credTypeExternalAccount            = "external_account"
)

// GCPSessionConfig is a representation of the configuration options for a GCP Config
type GCPSessionConfig struct {
	Credentials                        string
	AccessToken                        string
	ImpersonateServiceAccount          string
	ImpersonateServiceAccountDelegates []string
}

// GCPConfigBuilder constructs GCP client options using the builder pattern.
type GCPConfigBuilder struct {
	sessionConfig *GCPSessionConfig
	httpClient    *http.Client
	env           map[string]string
}

// NewGCPConfigBuilder creates a new GCPConfigBuilder.
func NewGCPConfigBuilder() *GCPConfigBuilder {
	return &GCPConfigBuilder{}
}

// WithSessionConfig sets the GCP session configuration.
func (b *GCPConfigBuilder) WithSessionConfig(config *GCPSessionConfig) *GCPConfigBuilder {
	b.sessionConfig = config
	return b
}

// WithEnv sets the environment variables to use for credential resolution.
func (b *GCPConfigBuilder) WithEnv(env map[string]string) *GCPConfigBuilder {
	b.env = env
	return b
}

// WithHTTPClient routes GCP traffic through c, the same handle that the rest
// of Terragrunt threads through [github.com/gruntwork-io/terragrunt/internal/venv.Venv].
// Tests substitute c with one built by
// [github.com/gruntwork-io/terragrunt/internal/vhttp.NewMemClient] so GCP
// SDK calls never reach the network. The JWT branch in
// [createGCPCredentialsFromGoogleCredentialsEnv] builds its own transport
// from the parsed service-account key and is intentionally not overridden.
func (b *GCPConfigBuilder) WithHTTPClient(c *http.Client) *GCPConfigBuilder {
	b.httpClient = c
	return b
}

// BuildGCSClient builds a GCS storage client from the configured options.
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

// Build returns GCP client options from the configured session config and env.
func (b *GCPConfigBuilder) Build(ctx context.Context) ([]option.ClientOption, error) {
	gcpCfg := b.sessionConfig
	env := b.env

	var (
		clientOpts      []option.ClientOption
		httpClientSetBy string
	)

	envCreds, err := createGCPCredentialsFromEnv(env)
	if err != nil {
		return nil, err
	}

	if envCreds != nil {
		clientOpts = append(clientOpts, envCreds)
	} else if gcpCfg != nil && gcpCfg.Credentials != "" {
		credOpt, err := credentialsFileOption(gcpCfg.Credentials)
		if err != nil {
			return nil, err
		}

		clientOpts = append(clientOpts, credOpt)
	} else if gcpCfg != nil && gcpCfg.AccessToken != "" {
		tokenSource := oauth2.StaticTokenSource(&oauth2.Token{
			AccessToken: gcpCfg.AccessToken,
		})
		clientOpts = append(clientOpts, option.WithTokenSource(tokenSource))
	} else if oauthAccessToken := env["GOOGLE_OAUTH_ACCESS_TOKEN"]; oauthAccessToken != "" {
		tokenSource := oauth2.StaticTokenSource(&oauth2.Token{
			AccessToken: oauthAccessToken,
		})
		clientOpts = append(clientOpts, option.WithTokenSource(tokenSource))
	} else if env["GOOGLE_CREDENTIALS"] != "" {
		clientOpt, err := createGCPCredentialsFromGoogleCredentialsEnv(ctx, env)
		if err != nil {
			return nil, err
		}

		if clientOpt != nil {
			clientOpts = append(clientOpts, clientOpt)
			// The JWT branch installs its own *http.Client from the parsed
			// service-account key. Honor it and skip our injected one.
			httpClientSetBy = "GOOGLE_CREDENTIALS"
		}
	}

	// Handle service account impersonation.
	// When impersonation is configured, the impersonation token source replaces
	// any base credentials. The impersonate library uses Application Default
	// Credentials internally as the source identity.
	if gcpCfg != nil && gcpCfg.ImpersonateServiceAccount != "" {
		ts, err := impersonate.CredentialsTokenSource(ctx, impersonate.CredentialsConfig{
			TargetPrincipal: gcpCfg.ImpersonateServiceAccount,
			Scopes:          []string{storage.ScopeFullControl},
			Delegates:       gcpCfg.ImpersonateServiceAccountDelegates,
		}, clientOpts...)
		if err != nil {
			return nil, errors.Errorf("Error creating impersonation token source: %w", err)
		}

		clientOpts = []option.ClientOption{option.WithTokenSource(ts)}
		httpClientSetBy = ""
	}

	if b.httpClient != nil && httpClientSetBy == "" {
		clientOpts = append(clientOpts, option.WithHTTPClient(b.httpClient))
	}

	return clientOpts, nil
}

// createGCPCredentialsFromEnv creates GCP credentials from GOOGLE_APPLICATION_CREDENTIALS environment variable in env
// It looks for GOOGLE_APPLICATION_CREDENTIALS and returns a ClientOption that can be used
// with Google Cloud clients. Returns nil if the environment variable is not set.
func createGCPCredentialsFromEnv(env map[string]string) (option.ClientOption, error) {
	if len(env) == 0 {
		return nil, nil
	}

	credentialsFile := env["GOOGLE_APPLICATION_CREDENTIALS"]
	if credentialsFile == "" {
		return nil, nil
	}

	return credentialsFileOption(credentialsFile)
}

// credentialsFileOption reads a GCP credentials JSON file, detects its type,
// and returns the appropriate ClientOption.
func credentialsFileOption(filename string) (option.ClientOption, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, errors.Errorf("Error reading credentials file %s: %w", filename, err)
	}

	var meta struct {
		Type string `json:"type"`
	}

	if err := json.Unmarshal(data, &meta); err != nil {
		return nil, errors.Errorf("Error parsing credentials file %s: %w", filename, err)
	}

	credType, err := credentialsTypeFromString(meta.Type)
	if err != nil {
		return nil, err
	}

	return option.WithAuthCredentialsFile(credType, filename), nil
}

// credentialsTypeFromString maps the "type" field in a GCP credentials JSON
// file to the corresponding option.CredentialsType.
func credentialsTypeFromString(t string) (option.CredentialsType, error) {
	switch t {
	case credTypeServiceAccount:
		return option.ServiceAccount, nil
	case credTypeAuthorizedUser:
		return option.AuthorizedUser, nil
	case credTypeImpersonatedServiceAccount:
		return option.ImpersonatedServiceAccount, nil
	case credTypeExternalAccount:
		return option.ExternalAccount, nil
	default:
		return "", errors.Errorf("Unsupported GCP credentials type: %q", t)
	}
}

// createGCPCredentialsFromGoogleCredentialsEnv creates GCP credentials from GOOGLE_CREDENTIALS environment variable.
// This can be either a file path or the JSON content directly (to mirror how Terraform works).
func createGCPCredentialsFromGoogleCredentialsEnv(
	ctx context.Context,
	env map[string]string,
) (option.ClientOption, error) {
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
