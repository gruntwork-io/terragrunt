// Package gcphelper provides helper functions for working with GCP services.
package gcphelper

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"net/http"

	"cloud.google.com/go/storage"
	"github.com/gruntwork-io/terragrunt/internal/util"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/jwt"
	"google.golang.org/api/impersonate"
	"google.golang.org/api/option"
	htransport "google.golang.org/api/transport/http"
)

const (
	tokenURL = "https://oauth2.googleapis.com/token"

	credTypeServiceAccount             = "service_account"
	credTypeAuthorizedUser             = "authorized_user"
	credTypeImpersonatedServiceAccount = "impersonated_service_account"
	credTypeExternalAccount            = "external_account"

	cloudPlatformScope = "https://www.googleapis.com/auth/cloud-platform"
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

// BuildGCSClient builds a GCS storage client from the configured options.
func (b *GCPConfigBuilder) BuildGCSClient(
	ctx context.Context,
	v *venv.Venv,
) (*storage.Client, error) {
	v.RequireEnv()
	v.RequireFS()
	v.RequireHTTP()

	ctx = withOAuthClient(ctx, v)

	clientOpts, err := b.Build(ctx, v)
	if err != nil {
		return nil, err
	}

	// Auth is layered over the venv's transport rather than the venv client
	// being passed straight to storage.NewClient: a bare client carries no
	// credentials, so every request would go out unauthenticated.
	//
	// The scopes lead so that a caller's own scopes still win.
	transportOpts := append([]option.ClientOption{GCSScopes()}, clientOpts...)

	trans, err := htransport.NewTransport(ctx, v.HTTP.Transport, transportOpts...)
	if err != nil {
		return nil, fmt.Errorf("error building GCS transport: %w", err)
	}

	//nolint:forbidigo // This is the wrapper the rule points callers at; trans is built on the venv's transport.
	gcsClient, err := storage.NewClient(
		ctx,
		option.WithHTTPClient(&http.Client{Transport: trans}),
	)
	if err != nil {
		return nil, fmt.Errorf("error creating GCS client: %w", err)
	}

	return gcsClient, nil
}

// GCSScopes returns the scopes storage.NewClient applies to clients whose
// transport it builds itself. Credentials resolve while the transport is being
// built, which is where those scopes would normally be attached, so anything
// building the transport has to name them. Without them the token exchange asks
// for no scope at all and Google rejects it with `invalid_scope`.
func GCSScopes() option.ClientOption {
	return option.WithScopes(storage.ScopeFullControl, cloudPlatformScope)
}

// withOAuthClient points the oauth2 library at v's client. oauth2 mints tokens
// through http.DefaultClient otherwise, independently of the transport the API
// client is built on.
func withOAuthClient(ctx context.Context, v *venv.Venv) context.Context {
	return context.WithValue(ctx, oauth2.HTTPClient, v.HTTP)
}

// Build returns GCP client options from the configured session config and env.
func (b *GCPConfigBuilder) Build(
	ctx context.Context,
	v *venv.Venv,
) ([]option.ClientOption, error) {
	gcpCfg := b.sessionConfig
	env := v.Env

	var clientOpts []option.ClientOption

	envCreds, err := createGCPCredentialsFromEnv(v.FS, env)
	if err != nil {
		return nil, err
	}

	if envCreds != nil {
		clientOpts = append(clientOpts, envCreds)
	} else if gcpCfg != nil && gcpCfg.Credentials != "" {
		// Use credentials file from config
		credOpt, err := credentialsFileOption(v.FS, gcpCfg.Credentials)
		if err != nil {
			return nil, err
		}

		clientOpts = append(clientOpts, credOpt)
	} else if gcpCfg != nil && gcpCfg.AccessToken != "" {
		// Use access token from config
		tokenSource := oauth2.StaticTokenSource(&oauth2.Token{
			AccessToken: gcpCfg.AccessToken,
		})
		clientOpts = append(clientOpts, option.WithTokenSource(tokenSource))
	} else if oauthAccessToken := env["GOOGLE_OAUTH_ACCESS_TOKEN"]; oauthAccessToken != "" {
		// Use OAuth access token from environment
		tokenSource := oauth2.StaticTokenSource(&oauth2.Token{
			AccessToken: oauthAccessToken,
		})
		clientOpts = append(clientOpts, option.WithTokenSource(tokenSource))
	} else if env["GOOGLE_CREDENTIALS"] != "" {
		// Use GOOGLE_CREDENTIALS from environment (can be file path or JSON content)
		clientOpt, err := createGCPCredentialsFromGoogleCredentialsEnv(ctx, env)
		if err != nil {
			return nil, err
		}

		if clientOpt != nil {
			clientOpts = append(clientOpts, clientOpt)
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
			return nil, fmt.Errorf("error creating impersonation token source: %w", err)
		}

		clientOpts = []option.ClientOption{option.WithTokenSource(ts)}
	}

	return clientOpts, nil
}

// createGCPCredentialsFromEnv creates GCP credentials from GOOGLE_APPLICATION_CREDENTIALS environment variable in env
// It looks for GOOGLE_APPLICATION_CREDENTIALS and returns a ClientOption that can be used
// with Google Cloud clients. Returns nil if the environment variable is not set.
func createGCPCredentialsFromEnv(fsys vfs.FS, env map[string]string) (option.ClientOption, error) {
	credentialsFile := env["GOOGLE_APPLICATION_CREDENTIALS"]
	if credentialsFile == "" {
		return nil, nil
	}

	return credentialsFileOption(fsys, credentialsFile)
}

// credentialsFileOption reads a GCP credentials JSON file, detects its type,
// and returns the appropriate ClientOption. The parsed bytes are handed to the
// SDK rather than the path, so the file is read once, through fsys, instead of
// the SDK opening it again off the real disk.
func credentialsFileOption(fsys vfs.FS, filename string) (option.ClientOption, error) {
	data, err := vfs.ReadFile(fsys, filename)
	if err != nil {
		return nil, fmt.Errorf("error reading credentials file %s: %w", filename, err)
	}

	var meta struct {
		Type string `json:"type"`
	}

	if err := json.Unmarshal(data, &meta); err != nil {
		return nil, fmt.Errorf("error parsing credentials file %s: %w", filename, err)
	}

	credType, err := credentialsTypeFromString(meta.Type)
	if err != nil {
		return nil, err
	}

	return option.WithAuthCredentialsJSON(credType, data), nil
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
		return "", fmt.Errorf("unsupported GCP credentials type: %q", t)
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
		return nil, fmt.Errorf("error loading credentials: %w", err)
	}

	if err := json.Unmarshal([]byte(contents), &account); err != nil {
		return nil, errors.New("error parsing GCP credentials")
	}

	conf := jwt.Config{
		Email:      account.ClientEmail,
		PrivateKey: []byte(account.PrivateKey),
		// We need the FullControl scope to be able to add metadata such as labels
		Scopes:   []string{storage.ScopeFullControl},
		TokenURL: tokenURL,
	}

	// A token source rather than conf.Client: the latter is a whole
	// http.Client, which would displace the transport the caller layers auth
	// onto. The source mints its tokens over the client in ctx.
	return option.WithTokenSource(conf.TokenSource(ctx)), nil
}
