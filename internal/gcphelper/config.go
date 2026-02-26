// Package gcphelper provides helper functions for working with GCP services.
package gcphelper

import (
	"context"
	"encoding/json"
	"os"

	"cloud.google.com/go/storage"
	"github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/gruntwork-io/terragrunt/internal/util"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/pkg/options"
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

// CreateGCSClient creates a GCS client using the provided GCP configuration.
func CreateGCSClient(
	ctx context.Context,
	l log.Logger,
	config *GCPSessionConfig,
	opts *options.TerragruntOptions,
) (*storage.Client, error) {
	clientOpts, err := CreateGCPConfig(ctx, config, opts)
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
	gcpCfg *GCPSessionConfig,
	opts *options.TerragruntOptions,
) ([]option.ClientOption, error) {
	var clientOpts []option.ClientOption

	if envCreds, err := createGCPCredentialsFromEnv(opts); err != nil {
		return nil, err
	} else if envCreds != nil {
		clientOpts = append(clientOpts, envCreds)
	} else if gcpCfg != nil && gcpCfg.Credentials != "" {
		// Use credentials file from config
		credOpt, err := credentialsFileOption(gcpCfg.Credentials)
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
func createGCPCredentialsFromEnv(opts *options.TerragruntOptions) (option.ClientOption, error) {
	if opts == nil || opts.Env == nil {
		return nil, nil
	}

	credentialsFile := opts.Env["GOOGLE_APPLICATION_CREDENTIALS"]
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
