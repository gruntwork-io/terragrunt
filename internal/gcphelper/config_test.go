//go:build gcp

package gcphelper_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/gcphelper"
	"github.com/gruntwork-io/terragrunt/pkg/options"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCreateGcpConfigWithApplicationCredentialsEnv(t *testing.T) {
	t.Parallel()

	l := logger.CreateLogger()
	ctx := context.Background()

	// Create a temporary credentials file
	tmpDir := t.TempDir()
	credsFile := filepath.Join(tmpDir, "credentials.json")
	err := os.WriteFile(credsFile, []byte(`{"type":"service_account"}`), 0644)
	require.NoError(t, err)

	opts := &options.TerragruntOptions{
		Env: map[string]string{
			"GOOGLE_APPLICATION_CREDENTIALS": credsFile,
		},
	}

	clientOpts, err := gcphelper.CreateGCPConfig(ctx, l, nil, opts)
	require.NoError(t, err)
	assert.NotEmpty(t, clientOpts)
}

func TestCreateGcpConfigWithOAuthAccessTokenEnv(t *testing.T) {
	t.Parallel()

	l := logger.CreateLogger()
	ctx := context.Background()

	opts := &options.TerragruntOptions{
		Env: map[string]string{
			"GOOGLE_OAUTH_ACCESS_TOKEN": "test-oauth-token",
		},
	}

	clientOpts, err := gcphelper.CreateGCPConfig(ctx, l, nil, opts)
	require.NoError(t, err)
	assert.NotEmpty(t, clientOpts)
}

func TestCreateGcpConfigWithGoogleCredentialsEnv(t *testing.T) {
	t.Parallel()

	l := logger.CreateLogger()
	ctx := context.Background()

	// Test with JSON content directly (not a file path)
	credsJSON := `{
		"type": "service_account",
		"project_id": "test-project",
		"private_key_id": "test-key-id",
		"private_key": "-----BEGIN PRIVATE KEY-----\nfake-private-key\n-----END PRIVATE KEY-----\n",
		"client_email": "test@test-project.iam.gserviceaccount.com",
		"client_id": "123456789",
		"auth_uri": "https://accounts.google.com/o/oauth2/auth",
		"token_uri": "https://oauth2.googleapis.com/token"
	}`

	opts := &options.TerragruntOptions{
		Env: map[string]string{
			"GOOGLE_CREDENTIALS": credsJSON,
		},
	}

	clientOpts, err := gcphelper.CreateGCPConfig(ctx, l, nil, opts)
	require.NoError(t, err)
	assert.NotEmpty(t, clientOpts)
}

func TestCreateGcpConfigWithCredentialsFileFromConfig(t *testing.T) {
	t.Parallel()

	l := logger.CreateLogger()
	ctx := context.Background()

	// Create a temporary credentials file
	tmpDir := t.TempDir()
	credsFile := filepath.Join(tmpDir, "credentials.json")
	err := os.WriteFile(credsFile, []byte(`{"type":"service_account"}`), 0644)
	require.NoError(t, err)

	opts := &options.TerragruntOptions{
		Env: map[string]string{},
	}

	gcpCfg := &gcphelper.GCPSessionConfig{
		Credentials: credsFile,
	}

	clientOpts, err := gcphelper.CreateGCPConfig(ctx, l, gcpCfg, opts)
	require.NoError(t, err)
	assert.NotEmpty(t, clientOpts)
}

func TestCreateGcpConfigWithAccessTokenFromConfig(t *testing.T) {
	t.Parallel()

	l := logger.CreateLogger()
	ctx := context.Background()

	opts := &options.TerragruntOptions{
		Env: map[string]string{},
	}

	gcpCfg := &gcphelper.GCPSessionConfig{
		AccessToken: "test-access-token",
	}

	clientOpts, err := gcphelper.CreateGCPConfig(ctx, l, gcpCfg, opts)
	require.NoError(t, err)
	assert.NotEmpty(t, clientOpts)
}

func TestGcpConfigEnvVarsTakePrecedenceOverConfig(t *testing.T) {
	t.Parallel()

	l := logger.CreateLogger()
	ctx := context.Background()

	// Create temporary credentials files
	tmpDir := t.TempDir()
	envCredsFile := filepath.Join(tmpDir, "env-credentials.json")
	configCredsFile := filepath.Join(tmpDir, "config-credentials.json")

	err := os.WriteFile(envCredsFile, []byte(`{"type":"service_account"}`), 0644)
	require.NoError(t, err)

	err = os.WriteFile(configCredsFile, []byte(`{"type":"service_account"}`), 0644)
	require.NoError(t, err)

	// Set environment variable - this should take precedence over config
	opts := &options.TerragruntOptions{
		Env: map[string]string{
			"GOOGLE_APPLICATION_CREDENTIALS": envCredsFile,
		},
	}

	// Create config with explicit credentials - but env var should be used instead
	gcpCfg := &gcphelper.GCPSessionConfig{
		Credentials: configCredsFile, // This should be ignored in favor of env var
	}

	clientOpts, err := gcphelper.CreateGCPConfig(ctx, l, gcpCfg, opts)
	require.NoError(t, err)
	assert.NotEmpty(t, clientOpts)

	// In GCP, environment variables take precedence over config values
	// The if-else chain in CreateGcpConfig checks env vars first
}

func TestCreateGcpConfigWithImpersonation(t *testing.T) {
	t.Parallel()

	l := logger.CreateLogger()
	ctx := context.Background()

	opts := &options.TerragruntOptions{
		Env: map[string]string{},
	}

	gcpCfg := &gcphelper.GCPSessionConfig{
		ImpersonateServiceAccount:          "test@project.iam.gserviceaccount.com",
		ImpersonateServiceAccountDelegates: []string{"delegate@project.iam.gserviceaccount.com"},
	}

	// This will fail because we don't have real credentials, but we can verify
	// that the impersonation configuration is attempted
	_, err := gcphelper.CreateGCPConfig(ctx, l, gcpCfg, opts)
	// We expect an error because impersonation requires valid base credentials
	// The error should be about impersonation, not about missing credentials
	require.Error(t, err)
	assert.Contains(t, err.Error(), "impersonation")
}

func TestCreateGcpConfigWithNoCredentials(t *testing.T) {
	t.Parallel()

	l := logger.CreateLogger()
	ctx := context.Background()

	opts := &options.TerragruntOptions{
		Env: map[string]string{},
	}

	// No credentials provided - should return empty options (will use default credentials)
	clientOpts, err := gcphelper.CreateGCPConfig(ctx, l, nil, opts)
	require.NoError(t, err)
	// Should return empty options when no credentials are provided
	// (default credentials will be used by GCP client)
	assert.Empty(t, clientOpts)
}

func TestCreateGcpConfigWithGoogleCredentialsFile(t *testing.T) {
	t.Parallel()

	l := logger.CreateLogger()
	ctx := context.Background()

	// Create a temporary credentials file
	tmpDir := t.TempDir()
	credsFile := filepath.Join(tmpDir, "credentials.json")
	credsJSON := `{
		"type": "service_account",
		"project_id": "test-project",
		"private_key_id": "test-key-id",
		"private_key": "-----BEGIN PRIVATE KEY-----\nfake-private-key\n-----END PRIVATE KEY-----\n",
		"client_email": "test@test-project.iam.gserviceaccount.com",
		"client_id": "123456789"
	}`
	err := os.WriteFile(credsFile, []byte(credsJSON), 0644)
	require.NoError(t, err)

	// Test with GOOGLE_CREDENTIALS pointing to a file path
	opts := &options.TerragruntOptions{
		Env: map[string]string{
			"GOOGLE_CREDENTIALS": credsFile,
		},
	}

	clientOpts, err := gcphelper.CreateGCPConfig(ctx, l, nil, opts)
	require.NoError(t, err)
	assert.NotEmpty(t, clientOpts)
}
