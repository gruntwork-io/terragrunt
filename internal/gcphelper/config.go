// Package gcphelper provides helper functions for working with GCP services.
package gcphelper

import (
	"context"
	"encoding/json"

	"cloud.google.com/go/storage"
	"github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/util"
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

// CreateGCSClient creates a GCS client using the provided GCP configuration.
func CreateGCSClient(
	ctx context.Context,
	l log.Logger,
	config *GCPSessionConfig,
	opts *options.TerragruntOptions,
) (*storage.Client, error) {
	clientOpts, err := CreateGCPConfig(ctx, l, config, opts)
	if err != nil {
		return nil, err
	}

	gcsClient, err := storage.NewClient(ctx, clientOpts...)
	if err != nil {
		return nil, errors.Errorf("Error creating GCS client: %w", err)
	}

	return gcsClient, nil
}

// CreateGCPConfig returns GCP client options for the given GCPSessionConfig and TerragruntOptions.
func CreateGCPConfig(
	ctx context.Context,
	l log.Logger,
	gcpCfg *GCPSessionConfig,
	opts *options.TerragruntOptions,
) ([]option.ClientOption, error) {
	var clientOpts []option.ClientOption

	if envCreds := createGCPCredentialsFromEnv(opts); envCreds != nil {
		clientOpts = append(clientOpts, envCreds)
	} else if gcpCfg != nil && gcpCfg.Credentials != "" {
		// Use credentials file from config
		clientOpts = append(clientOpts, option.WithCredentialsFile(gcpCfg.Credentials))
	} else if gcpCfg != nil && gcpCfg.AccessToken != "" {
		// Use access token from config
		tokenSource := oauth2.StaticTokenSource(&oauth2.Token{
			AccessToken: gcpCfg.AccessToken,
		})
		clientOpts = append(clientOpts, option.WithTokenSource(tokenSource))
	} else if oauthAccessToken := opts.Env["GOOGLE_OAUTH_ACCESS_TOKEN"]; oauthAccessToken != "" {
		// Use OAuth access token from environment
		tokenSource := oauth2.StaticTokenSource(&oauth2.Token{
			AccessToken: oauthAccessToken,
		})
		clientOpts = append(clientOpts, option.WithTokenSource(tokenSource))
	} else if opts.Env["GOOGLE_CREDENTIALS"] != "" {
		// Use GOOGLE_CREDENTIALS from environment (can be file path or JSON content)
		clientOpt, err := createGCPCredentialsFromGoogleCredentialsEnv(ctx, opts)
		if err != nil {
			return nil, err
		}

		if clientOpt != nil {
			clientOpts = append(clientOpts, clientOpt)
		}
	}

	// Handle service account impersonation
	if gcpCfg != nil && gcpCfg.ImpersonateServiceAccount != "" {
		ts, err := impersonate.CredentialsTokenSource(ctx, impersonate.CredentialsConfig{
			TargetPrincipal: gcpCfg.ImpersonateServiceAccount,
			Scopes:          []string{storage.ScopeFullControl},
			Delegates:       gcpCfg.ImpersonateServiceAccountDelegates,
		})
		if err != nil {
			return nil, errors.Errorf("Error creating impersonation token source: %w", err)
		}

		clientOpts = append(clientOpts, option.WithTokenSource(ts))
	}

	return clientOpts, nil
}

// createGCPCredentialsFromEnv creates GCP credentials from GOOGLE_APPLICATION_CREDENTIALS environment variable in opts.Env
// It looks for GOOGLE_APPLICATION_CREDENTIALS and returns a ClientOption that can be used
// with Google Cloud clients. Returns nil if the environment variable is not set.
func createGCPCredentialsFromEnv(opts *options.TerragruntOptions) option.ClientOption {
	if opts == nil || opts.Env == nil {
		return nil
	}

	credentialsFile := opts.Env["GOOGLE_APPLICATION_CREDENTIALS"]
	if credentialsFile == "" {
		return nil
	}

	return option.WithCredentialsFile(credentialsFile)
}

// createGCPCredentialsFromGoogleCredentialsEnv creates GCP credentials from GOOGLE_CREDENTIALS environment variable.
// This can be either a file path or the JSON content directly (to mirror how Terraform works).
func createGCPCredentialsFromGoogleCredentialsEnv(ctx context.Context, opts *options.TerragruntOptions) (option.ClientOption, error) {
	var account = struct {
		PrivateKeyID string `json:"private_key_id"`
		PrivateKey   string `json:"private_key"`
		ClientEmail  string `json:"client_email"`
		ClientID     string `json:"client_id"`
	}{}

	// to mirror how Terraform works, we have to accept either the file path or the contents
	creds := opts.Env["GOOGLE_CREDENTIALS"]

	contents, err := util.FileOrData(creds)
	if err != nil {
		return nil, errors.Errorf("Error loading credentials: %w", err)
	}

	if err := json.Unmarshal([]byte(contents), &account); err != nil {
		return nil, errors.Errorf("Error parsing credentials '%s': %w", contents, err)
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
