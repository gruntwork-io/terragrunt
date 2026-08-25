//go:build gcp

package gcphelper_test

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"os"
	"path/filepath"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/gcphelper"
	"github.com/gruntwork-io/terragrunt/test/helpers/venvtest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// serviceAccountJSON builds a complete service-account credentials payload.
// Credentials are validated where they are detected, so a payload naming only
// its type is rejected before it can stand in for a real one.
func serviceAccountJSON(t *testing.T) []byte {
	t.Helper()

	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)

	sa, err := json.Marshal(map[string]string{
		"type":         "service_account",
		"project_id":   "test-project",
		"client_email": "test@test-project.iam.gserviceaccount.com",
		"token_uri":    "https://oauth2.googleapis.com/token",
		"private_key": string(pem.EncodeToMemory(&pem.Block{
			Type:  "PRIVATE KEY",
			Bytes: x509.MarshalPKCS1PrivateKey(key),
		})),
	})
	require.NoError(t, err)

	return sa
}

func TestGcpConfigWithApplicationCredentialsEnv(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	// Create a temporary credentials file
	tmpDir := t.TempDir()
	credsFile := filepath.Join(tmpDir, "credentials.json")
	err := os.WriteFile(credsFile, serviceAccountJSON(t), 0644)
	require.NoError(t, err)

	env := map[string]string{
		"GOOGLE_APPLICATION_CREDENTIALS": credsFile,
	}

	clientOpts, err := gcphelper.NewGCPConfigBuilder().Build(ctx, venvtest.NewWithOSFS().WithEnv(env))
	require.NoError(t, err)
	assert.NotEmpty(t, clientOpts)
}

func TestGcpConfigWithOAuthAccessTokenEnv(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	env := map[string]string{
		"GOOGLE_OAUTH_ACCESS_TOKEN": "test-oauth-token",
	}

	clientOpts, err := gcphelper.NewGCPConfigBuilder().Build(ctx, venvtest.NewWithOSFS().WithEnv(env))
	require.NoError(t, err)
	assert.NotEmpty(t, clientOpts)
}

func TestGcpConfigWithGoogleCredentialsEnv(t *testing.T) {
	t.Parallel()

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

	env := map[string]string{
		"GOOGLE_CREDENTIALS": credsJSON,
	}

	clientOpts, err := gcphelper.NewGCPConfigBuilder().Build(ctx, venvtest.NewWithOSFS().WithEnv(env))
	require.NoError(t, err)
	assert.NotEmpty(t, clientOpts)
}

func TestGcpConfigWithCredentialsFileFromConfig(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	// Create a temporary credentials file
	tmpDir := t.TempDir()
	credsFile := filepath.Join(tmpDir, "credentials.json")
	err := os.WriteFile(credsFile, serviceAccountJSON(t), 0644)
	require.NoError(t, err)

	env := map[string]string{}

	gcpCfg := &gcphelper.GCPSessionConfig{
		Credentials: credsFile,
	}

	clientOpts, err := gcphelper.NewGCPConfigBuilder().
		WithSessionConfig(gcpCfg).
		Build(ctx, venvtest.NewWithOSFS().WithEnv(env))
	require.NoError(t, err)
	assert.NotEmpty(t, clientOpts)
}

func TestGcpConfigWithAccessTokenFromConfig(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	env := map[string]string{}

	gcpCfg := &gcphelper.GCPSessionConfig{
		AccessToken: "test-access-token",
	}

	clientOpts, err := gcphelper.NewGCPConfigBuilder().
		WithSessionConfig(gcpCfg).
		Build(ctx, venvtest.NewWithOSFS().WithEnv(env))
	require.NoError(t, err)
	assert.NotEmpty(t, clientOpts)
}

func TestGcpConfigEnvVarsTakePrecedenceOverConfig(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	// Create temporary credentials files
	tmpDir := t.TempDir()
	envCredsFile := filepath.Join(tmpDir, "env-credentials.json")
	configCredsFile := filepath.Join(tmpDir, "config-credentials.json")

	err := os.WriteFile(envCredsFile, serviceAccountJSON(t), 0644)
	require.NoError(t, err)

	err = os.WriteFile(configCredsFile, serviceAccountJSON(t), 0644)
	require.NoError(t, err)

	// Set environment variable - this should take precedence over config
	env := map[string]string{
		"GOOGLE_APPLICATION_CREDENTIALS": envCredsFile,
	}

	// Create config with explicit credentials - but env var should be used instead
	gcpCfg := &gcphelper.GCPSessionConfig{
		Credentials: configCredsFile, // This should be ignored in favor of env var
	}

	clientOpts, err := gcphelper.NewGCPConfigBuilder().
		WithSessionConfig(gcpCfg).
		Build(ctx, venvtest.NewWithOSFS().WithEnv(env))
	require.NoError(t, err)
	assert.NotEmpty(t, clientOpts)

	// In GCP, environment variables take precedence over config values
	// The if-else chain in CreateGcpConfig checks env vars first
}

func TestGcpConfigWithImpersonation(t *testing.T) {
	t.Skip(
		"impersonation succeeds when application default credentials are present, as they are in the GCP CI job",
	)
	t.Parallel()

	ctx := context.Background()

	env := map[string]string{}

	gcpCfg := &gcphelper.GCPSessionConfig{
		ImpersonateServiceAccount:          "test@project.iam.gserviceaccount.com",
		ImpersonateServiceAccountDelegates: []string{"delegate@project.iam.gserviceaccount.com"},
	}

	// This will fail because we don't have real credentials, but we can verify
	// that the impersonation configuration is attempted
	_, err := gcphelper.NewGCPConfigBuilder().WithSessionConfig(gcpCfg).Build(ctx, venvtest.NewWithOSFS().WithEnv(env))
	// We expect an error because impersonation requires valid base credentials
	// The error should be about impersonation, not about missing credentials
	require.Error(t, err)
	assert.Contains(t, err.Error(), "impersonation")
}

func TestGcpConfigWithNoCredentials(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	env := map[string]string{}

	// No credentials provided - should return empty options (will use default credentials)
	clientOpts, err := gcphelper.NewGCPConfigBuilder().Build(ctx, venvtest.NewWithOSFS().WithEnv(env))
	require.NoError(t, err)
	// Should return empty options when no credentials are provided
	// (default credentials will be used by GCP client)
	assert.Empty(t, clientOpts)
}

func TestGcpConfigWithGoogleCredentialsFile(t *testing.T) {
	t.Parallel()

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
	env := map[string]string{
		"GOOGLE_CREDENTIALS": credsFile,
	}

	clientOpts, err := gcphelper.NewGCPConfigBuilder().Build(ctx, venvtest.NewWithOSFS().WithEnv(env))
	require.NoError(t, err)
	assert.NotEmpty(t, clientOpts)
}

func TestGcpConfigCredentialsPayloads(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		expected error
		name     string
		payload  string
	}{
		{name: "unsupported type", payload: `{"type":"gce_metadata"}`, expected: gcphelper.ErrBuildingCredentials},
		{name: "missing type", payload: `{"client_email":"a@b.com"}`, expected: gcphelper.ErrParsingCredentials},
		{name: "not json", payload: `not-json`, expected: gcphelper.ErrParsingCredentials},
		{name: "json array", payload: `["a"]`, expected: gcphelper.ErrParsingCredentials},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			credsFile := filepath.Join(t.TempDir(), "credentials.json")
			require.NoError(t, os.WriteFile(credsFile, []byte(tc.payload), 0o600))

			_, err := gcphelper.NewGCPConfigBuilder().
				WithSessionConfig(&gcphelper.GCPSessionConfig{Credentials: credsFile}).
				Build(context.Background(), venvtest.NewWithOSFS().WithEnv(map[string]string{}))
			require.ErrorIs(t, err, tc.expected)
		})
	}
}

// TestGcpConfigEmptyCredentialsFileFallsBackToADC pins the behaviour an unpopulated secret
// volume depends on: an empty file contributes no option rather than failing the run.
func TestGcpConfigEmptyCredentialsFileFallsBackToADC(t *testing.T) {
	t.Parallel()

	credsFile := filepath.Join(t.TempDir(), "credentials.json")
	require.NoError(t, os.WriteFile(credsFile, nil, 0o600))

	clientOpts, err := gcphelper.NewGCPConfigBuilder().
		WithSessionConfig(&gcphelper.GCPSessionConfig{Credentials: credsFile}).
		Build(context.Background(), venvtest.NewWithOSFS().WithEnv(map[string]string{}))
	require.NoError(t, err)
	assert.Empty(t, clientOpts)
}

// TestGcpConfigEmptyGACDoesNotFallBackToGoogleCredentials pins that an unpopulated
// GOOGLE_APPLICATION_CREDENTIALS file falls through to ADC, not to a leftover
// GOOGLE_CREDENTIALS naming a different service account.
func TestGcpConfigEmptyGACDoesNotFallBackToGoogleCredentials(t *testing.T) {
	t.Parallel()

	gacFile := filepath.Join(t.TempDir(), "gac.json")
	require.NoError(t, os.WriteFile(gacFile, nil, 0o600))

	env := map[string]string{
		"GOOGLE_APPLICATION_CREDENTIALS": gacFile,
		"GOOGLE_CREDENTIALS":             string(serviceAccountJSON(t)),
	}

	clientOpts, err := gcphelper.NewGCPConfigBuilder().
		Build(context.Background(), venvtest.NewWithOSFS().WithEnv(env))
	require.NoError(t, err)
	assert.Empty(t, clientOpts, "leftover GOOGLE_CREDENTIALS must not win over an empty GAC file")
}
