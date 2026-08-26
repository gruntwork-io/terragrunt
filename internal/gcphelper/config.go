// Package gcphelper provides helper functions for working with GCP services.
package gcphelper

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"

	"net/http"

	"cloud.google.com/go/auth/credentials"
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
	// envNameGoogleApplicationCredentials names the file holding Application Default Credentials.
	envNameGoogleApplicationCredentials = "GOOGLE_APPLICATION_CREDENTIALS"

	tokenURL = "https://oauth2.googleapis.com/token"

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
	return option.WithScopes(gcsScopes()...)
}

func gcsScopes() []string {
	return []string{storage.ScopeFullControl, cloudPlatformScope}
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
	v.RequireEnv()
	v.RequireFS()
	v.RequireHTTP()

	// Impersonation mints its token inside Build, so the oauth2 client has to be
	// in place before the transport the caller builds from these options.
	ctx = withOAuthClient(ctx, v)

	gcpCfg := b.sessionConfig
	env := v.Env

	var clientOpts []option.ClientOption

	envCreds, err := createGCPCredentialsFromEnv(v)
	if err != nil {
		return nil, err
	}

	switch {
	case envCreds != nil:
		clientOpts = append(clientOpts, envCreds)
	// GOOGLE_APPLICATION_CREDENTIALS named a file that read as empty. Fall through to the
	// ADC chain rather than to the remaining sources, which would authenticate as a
	// different identity than the one the user pointed at.
	case env[envNameGoogleApplicationCredentials] != "":
	case gcpCfg != nil && gcpCfg.Credentials != "":
		// Use credentials file from config
		credOpt, err := credentialsFileOption(v, gcpCfg.Credentials)
		if err != nil {
			return nil, err
		}

		if credOpt != nil {
			clientOpts = append(clientOpts, credOpt)
		}
	case gcpCfg != nil && gcpCfg.AccessToken != "":
		// Use access token from config
		tokenSource := oauth2.StaticTokenSource(&oauth2.Token{
			AccessToken: gcpCfg.AccessToken,
		})
		clientOpts = append(clientOpts, option.WithTokenSource(tokenSource))
	case env["GOOGLE_OAUTH_ACCESS_TOKEN"] != "":
		// Use OAuth access token from environment
		tokenSource := oauth2.StaticTokenSource(&oauth2.Token{
			AccessToken: env["GOOGLE_OAUTH_ACCESS_TOKEN"],
		})
		clientOpts = append(clientOpts, option.WithTokenSource(tokenSource))
	case env["GOOGLE_CREDENTIALS"] != "":
		// Use GOOGLE_CREDENTIALS from environment (can be file path or JSON content)
		clientOpt, err := createGCPCredentialsFromGoogleCredentialsEnv(ctx, v)
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

// createGCPCredentialsFromEnv creates GCP credentials from the
// GOOGLE_APPLICATION_CREDENTIALS variable in v's environment. Returns nil when
// the variable is not set.
func createGCPCredentialsFromEnv(v *venv.Venv) (option.ClientOption, error) {
	credentialsFile := v.Env[envNameGoogleApplicationCredentials]
	if credentialsFile == "" {
		return nil, nil
	}

	return credentialsFileOption(v, credentialsFile)
}

// credentialsFileOption reads a GCP credentials JSON file and returns the
// option that authenticates with it. The parsed bytes are handed to the SDK
// rather than the path, so the file is read once, through v's filesystem,
// instead of the SDK opening it again off the real disk.
func credentialsFileOption(v *venv.Venv, filename string) (option.ClientOption, error) {
	data, err := vfs.ReadFile(v.FS, filename)
	if err != nil {
		return nil, fmt.Errorf("error reading credentials file %s: %w", filename, err)
	}

	return credentialsJSONOption(v, data)
}

// credentialsJSONOption authenticates with a credentials JSON payload, or returns a nil
// option when the payload is empty so the caller falls through to the ADC chain the way
// the SDK's own detection does.
//
// The payload's type is read here rather than by the SDK's generic JSON detection, whose
// CredentialsJSON option is deprecated. The credentials are still built on v.HTTP, because
// the SDK would otherwise put the token exchange on a client of its own making
// (google.golang.org/api/internal.creds) that the venv never sees.
func credentialsJSONOption(v *venv.Venv, data []byte) (option.ClientOption, error) {
	if len(bytes.TrimSpace(data)) == 0 {
		return nil, nil
	}

	var metadata struct {
		Type credentials.CredType `json:"type"`
	}

	if err := json.Unmarshal(data, &metadata); err != nil {
		return nil, ParsingCredentialsError{Err: err}
	}

	// The SDK reports a missing type as an unsupported filetype, which does not say what is wrong.
	if metadata.Type == "" {
		return nil, ParsingCredentialsError{Err: ErrMissingCredentialsType}
	}

	// Every credential type the SDK accepts is passed through; it rejects the rest itself.
	creds, err := credentials.NewCredentialsFromJSON(metadata.Type, data, &credentials.DetectOptions{
		Scopes: gcsScopes(),
		Client: v.HTTP,
	})
	if err != nil {
		return nil, BuildingCredentialsError{CredType: metadata.Type, Err: err}
	}

	return option.WithAuthCredentials(creds), nil
}

// createGCPCredentialsFromGoogleCredentialsEnv creates GCP credentials from GOOGLE_CREDENTIALS environment variable.
// This can be either a file path or the JSON content directly (to mirror how Terraform works).
func createGCPCredentialsFromGoogleCredentialsEnv(
	ctx context.Context,
	v *venv.Venv,
) (option.ClientOption, error) {
	var account = struct {
		PrivateKeyID string `json:"private_key_id"`
		PrivateKey   string `json:"private_key"`
		ClientEmail  string `json:"client_email"`
		ClientID     string `json:"client_id"`
	}{}

	// to mirror how Terraform works, we have to accept either the file path or the contents
	creds := v.Env["GOOGLE_CREDENTIALS"]

	contents, err := util.FileOrData(v, creds)
	if err != nil {
		return nil, fmt.Errorf("error loading credentials: %w", err)
	}

	if err := json.Unmarshal([]byte(contents), &account); err != nil {
		return nil, ParsingCredentialsError{Err: err}
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
