package getter_test

import (
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/getter"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"oras.land/oras-go/v2/registry/remote"
	"oras.land/oras-go/v2/registry/remote/auth"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testRegistry = "registry.example.com"

func TestNewOCIRepositoryStoreReference(t *testing.T) {
	t.Parallel()

	newStore := getter.NewOCIRepositoryStore(logger.CreateLogger(), credentialVenv(t.TempDir(), nil))

	store, err := newStore(t.Context(), "127.0.0.1:5000", "org/team/vpc")
	require.NoError(t, err)

	repo, castOK := store.(*remote.Repository)
	require.True(t, castOK, "default store must be an oras remote repository")
	assert.Equal(t, "127.0.0.1:5000", repo.Reference.Registry)
	assert.Equal(t, "org/team/vpc", repo.Reference.Repository)
}

func TestOCIStaticCredentials(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		wantErrIs error
		env       map[string]string
		wantCred  auth.Credential
		name      string
	}{
		{
			name:     "token yields access token credential",
			env:      map[string]string{getter.EnvOCIToken: "token-value"},
			wantCred: auth.Credential{AccessToken: "token-value"},
		},
		{
			name: "username and password yield basic credential",
			env: map[string]string{
				getter.EnvOCIUsername: "static-user",
				getter.EnvOCIPassword: "static-pass",
			},
			wantCred: auth.Credential{Username: "static-user", Password: "static-pass"},
		},
		{
			name: "token with username conflicts",
			env: map[string]string{
				getter.EnvOCIToken:    "token-value",
				getter.EnvOCIUsername: "static-user",
			},
			wantErrIs: getter.ErrOCIStaticCredentialConflict,
		},
		{
			name: "token with password conflicts",
			env: map[string]string{
				getter.EnvOCIToken:    "token-value",
				getter.EnvOCIPassword: "static-pass",
			},
			wantErrIs: getter.ErrOCIStaticCredentialConflict,
		},
		{
			name:      "username without password is incomplete",
			env:       map[string]string{getter.EnvOCIUsername: "static-user"},
			wantErrIs: getter.ErrOCIStaticCredentialIncomplete,
		},
		{
			name:      "password without username is incomplete",
			env:       map[string]string{getter.EnvOCIPassword: "static-pass"},
			wantErrIs: getter.ErrOCIStaticCredentialIncomplete,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			home := t.TempDir()
			// Ambient file present to prove static credentials win over it.
			writeAuthFile(t, filepath.Join(home, ".docker", "config.json"), testRegistry, "ambient-user", "ambient-pass")

			newStore := getter.NewOCIRepositoryStore(logger.CreateLogger(), credentialVenv(home, tc.env))

			store, err := newStore(t.Context(), testRegistry, "modules/vpc")

			if tc.wantErrIs != nil {
				require.Error(t, err)
				assert.ErrorIs(t, err, tc.wantErrIs)

				return
			}

			require.NoError(t, err)
			assert.Equal(t, tc.wantCred, credentialFor(t, store, testRegistry))
		})
	}
}

func TestOCIStaticCredentialsScopedToRegistry(t *testing.T) {
	t.Parallel()

	env := map[string]string{
		getter.EnvOCIToken:    "scoped-token",
		getter.EnvOCIRegistry: testRegistry,
	}

	newStore := getter.NewOCIRepositoryStore(logger.CreateLogger(), credentialVenv(t.TempDir(), env))

	store, err := newStore(t.Context(), testRegistry, "modules/vpc")
	require.NoError(t, err)
	assert.Equal(t, auth.Credential{AccessToken: "scoped-token"}, credentialFor(t, store, testRegistry))

	// The token must not be offered to a different registry.
	other, err := newStore(t.Context(), "other.example.com", "modules/vpc")
	require.NoError(t, err)
	assert.Equal(t, auth.EmptyCredential, credentialFor(t, other, "other.example.com"))
}

// TestOCIAmbientCredentialConfigOrder covers the platform-agnostic part of the
// search order: XDG_CONFIG_HOME wins over the Docker config, and DOCKER_CONFIG
// is ignored so the literal ~/.docker/config.json is used.
func TestOCIAmbientCredentialConfigOrder(t *testing.T) {
	t.Parallel()

	home := t.TempDir()
	xdgConfig := t.TempDir()
	dockerConfigEnv := t.TempDir()

	writeAuthFile(t, filepath.Join(xdgConfig, "containers", "auth.json"), testRegistry, "xdg-config", "pw")
	writeAuthFile(t, filepath.Join(home, ".docker", "config.json"), testRegistry, "home-docker", "pw")
	// DOCKER_CONFIG must be ignored: a credential here must never win.
	writeAuthFile(t, filepath.Join(dockerConfigEnv, "config.json"), testRegistry, "docker-config-env", "pw")

	env := map[string]string{"XDG_CONFIG_HOME": xdgConfig, "DOCKER_CONFIG": dockerConfigEnv}
	newStore := getter.NewOCIRepositoryStore(logger.CreateLogger(), credentialVenv(home, env))

	store, err := newStore(t.Context(), testRegistry, "modules/vpc")
	require.NoError(t, err)
	assert.Equal(t, "xdg-config", credentialFor(t, store, testRegistry).Username)

	// With no XDG_CONFIG_HOME the Docker config wins and DOCKER_CONFIG stays ignored.
	newStore = getter.NewOCIRepositoryStore(
		logger.CreateLogger(),
		credentialVenv(home, map[string]string{"DOCKER_CONFIG": dockerConfigEnv}),
	)

	store, err = newStore(t.Context(), testRegistry, "modules/vpc")
	require.NoError(t, err)
	assert.Equal(t, "home-docker", credentialFor(t, store, testRegistry).Username)
}

// TestOCIAmbientCredentialXDGRuntimeDir covers the Linux-only runtime-dir
// candidate: highest priority on Linux, ignored elsewhere.
func TestOCIAmbientCredentialXDGRuntimeDir(t *testing.T) {
	t.Parallel()

	home := t.TempDir()
	runtimeDir := t.TempDir()

	writeAuthFile(t, filepath.Join(runtimeDir, "containers", "auth.json"), testRegistry, "xdg-runtime", "pw")
	writeAuthFile(t, filepath.Join(home, ".docker", "config.json"), testRegistry, "home-docker", "pw")

	newStore := getter.NewOCIRepositoryStore(
		logger.CreateLogger(),
		credentialVenv(home, map[string]string{"XDG_RUNTIME_DIR": runtimeDir}),
	)

	store, err := newStore(t.Context(), testRegistry, "modules/vpc")
	require.NoError(t, err)

	wantUser := "home-docker"
	if runtime.GOOS == "linux" {
		wantUser = "xdg-runtime"
	}

	assert.Equal(t, wantUser, credentialFor(t, store, testRegistry).Username)
}

func TestOCIAmbientCredentialScopedToRegistry(t *testing.T) {
	t.Parallel()

	home := t.TempDir()
	writeAuthFile(t, filepath.Join(home, ".docker", "config.json"), testRegistry, "scoped-user", "scoped-pass")

	newStore := getter.NewOCIRepositoryStore(logger.CreateLogger(), credentialVenv(home, nil))

	store, err := newStore(t.Context(), "other-registry.example.com", "modules/vpc")
	require.NoError(t, err)

	assert.Equal(t, auth.EmptyCredential, credentialFor(t, store, "other-registry.example.com"))
}

func TestOCIAmbientCredentialNoFiles(t *testing.T) {
	t.Parallel()

	newStore := getter.NewOCIRepositoryStore(logger.CreateLogger(), credentialVenv(t.TempDir(), nil))

	store, err := newStore(t.Context(), testRegistry, "modules/vpc")
	require.NoError(t, err)

	assert.Equal(t, auth.EmptyCredential, credentialFor(t, store, testRegistry))
}

// TestOCIAmbientCredentialInvalidFileSkipped covers a file that fails to open
// (invalid top-level JSON): it is skipped and a valid lower-priority file
// still resolves.
func TestOCIAmbientCredentialInvalidFileSkipped(t *testing.T) {
	t.Parallel()

	home := t.TempDir()

	badPath := filepath.Join(home, ".docker", "config.json")
	require.NoError(t, os.MkdirAll(filepath.Dir(badPath), 0o755))
	require.NoError(t, os.WriteFile(badPath, []byte("not json"), 0o600))

	newStore := getter.NewOCIRepositoryStore(logger.CreateLogger(), credentialVenv(home, nil))

	store, err := newStore(t.Context(), testRegistry, "modules/vpc")
	require.NoError(t, err)
	assert.Equal(t, auth.EmptyCredential, credentialFor(t, store, testRegistry))
}

// TestOCIAmbientCredentialMalformedEntryFallsThrough covers a file that opens
// cleanly (valid JSON) but whose entry for the registry is malformed: the
// chain must fall through to a valid lower-priority file rather than abort.
func TestOCIAmbientCredentialMalformedEntryFallsThrough(t *testing.T) {
	t.Parallel()

	home := t.TempDir()
	xdgConfig := t.TempDir()

	// High priority: valid JSON, but the auth value decodes to "nocolon"
	// (no username:password separator), which oras rejects at lookup time.
	writeRawAuthFile(
		t,
		filepath.Join(xdgConfig, "containers", "auth.json"),
		testRegistry,
		base64.StdEncoding.EncodeToString([]byte("nocolon")),
	)
	// Lower priority: valid.
	writeAuthFile(t, filepath.Join(home, ".docker", "config.json"), testRegistry, "good-user", "good-pass")

	newStore := getter.NewOCIRepositoryStore(
		logger.CreateLogger(),
		credentialVenv(home, map[string]string{"XDG_CONFIG_HOME": xdgConfig}),
	)

	store, err := newStore(t.Context(), testRegistry, "modules/vpc")
	require.NoError(t, err)
	assert.Equal(t, "good-user", credentialFor(t, store, testRegistry).Username)
}

// credentialVenv builds a hermetic Venv for credential discovery: an OS-backed
// filesystem (oras parses real files), the platform's home variable pointing at
// home, and extra env entries layered on top, so host credentials never leak in
// and tests stay parallel.
func credentialVenv(home string, extra map[string]string) venv.Venv {
	env := map[string]string{}
	if runtime.GOOS == "windows" {
		env["USERPROFILE"] = home
	} else {
		env["HOME"] = home
	}

	for name, value := range extra {
		env[name] = value
	}

	return venv.Venv{FS: vfs.NewOSFS(), Env: env}
}

// credentialFor resolves the credential the store would send for registry.
func credentialFor(t *testing.T, store getter.OCIRepositoryStore, registry string) auth.Credential {
	t.Helper()

	repo, castOK := store.(*remote.Repository)
	require.True(t, castOK, "default store must be an oras remote repository")

	client, castOK := repo.Client.(*auth.Client)
	require.True(t, castOK, "default store must use an oras auth client")

	cred, err := client.Credential(t.Context(), registry)
	require.NoError(t, err)

	return cred
}

// writeAuthFile writes a config.json-format credential file granting
// user/pass for registry.
func writeAuthFile(t *testing.T, path, registry, user, pass string) {
	t.Helper()

	writeRawAuthFile(t, path, registry, base64.StdEncoding.EncodeToString([]byte(user+":"+pass)))
}

// writeRawAuthFile writes a config.json-format credential file with a
// caller-supplied base64 auth value, so tests can plant malformed entries.
func writeRawAuthFile(t *testing.T, path, registry, encodedAuth string) {
	t.Helper()

	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))

	content := map[string]any{
		"auths": map[string]any{
			registry: map[string]string{"auth": encodedAuth},
		},
	}

	data, err := json.Marshal(content)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(path, data, 0o600))
}
