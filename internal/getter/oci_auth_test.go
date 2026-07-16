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
				require.ErrorIs(t, err, tc.wantErrIs)

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

// TestOCIStaticCredentialsScopedStillResolvesAmbient: scoping a static credential
// must not disable ambient discovery for the registries it declines.
func TestOCIStaticCredentialsScopedStillResolvesAmbient(t *testing.T) {
	t.Parallel()

	const otherRegistry = "other.example.com"

	home := t.TempDir()
	writeAuthFile(t, filepath.Join(home, ".docker", "config.json"), otherRegistry, "ambient-user", "ambient-pass")

	env := map[string]string{
		getter.EnvOCIToken:    "scoped-token",
		getter.EnvOCIRegistry: testRegistry,
	}

	newStore := getter.NewOCIRepositoryStore(logger.CreateLogger(), credentialVenv(home, env))

	// The scoped registry uses the static token.
	store, err := newStore(t.Context(), testRegistry, "modules/vpc")
	require.NoError(t, err)
	assert.Equal(t, auth.Credential{AccessToken: "scoped-token"}, credentialFor(t, store, testRegistry))

	// A registry the static credential declines still falls through to ambient.
	other, err := newStore(t.Context(), otherRegistry, "modules/vpc")
	require.NoError(t, err)
	assert.Equal(
		t,
		auth.Credential{Username: "ambient-user", Password: "ambient-pass"},
		credentialFor(t, other, otherRegistry),
	)
}

// TestOCIAmbientCredentialConfigOrder: XDG_CONFIG_HOME beats Docker config; DOCKER_CONFIG ignored.
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

// TestOCIAmbientCredentialXDGRuntimeDir: runtime-dir wins on Linux, ignored elsewhere.
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

func TestNewOCIRepositoryStoreRejectsNonOSFilesystem(t *testing.T) {
	t.Parallel()

	v := venv.Venv{FS: vfs.NewMemMapFS(), Env: map[string]string{"HOME": t.TempDir()}}
	newStore := getter.NewOCIRepositoryStore(logger.CreateLogger(), v)

	_, err := newStore(t.Context(), testRegistry, "modules/vpc")
	require.ErrorIs(t, err, getter.ErrOCINonOSFilesystem)
}

func TestOCIAmbientCredentialNoFiles(t *testing.T) {
	t.Parallel()

	newStore := getter.NewOCIRepositoryStore(logger.CreateLogger(), credentialVenv(t.TempDir(), nil))

	store, err := newStore(t.Context(), testRegistry, "modules/vpc")
	require.NoError(t, err)

	assert.Equal(t, auth.EmptyCredential, credentialFor(t, store, testRegistry))
}

// TestOCIAmbientCredentialInvalidFileSkipped: an unopenable file is skipped, a valid one still resolves.
func TestOCIAmbientCredentialInvalidFileSkipped(t *testing.T) {
	t.Parallel()

	home := t.TempDir()
	xdgConfig := t.TempDir()

	// High priority: invalid top-level JSON, so the file cannot be opened as a store.
	badPath := filepath.Join(xdgConfig, "containers", "auth.json")
	require.NoError(t, os.MkdirAll(filepath.Dir(badPath), 0o755))
	require.NoError(t, os.WriteFile(badPath, []byte("not json"), 0o600))
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

// TestOCIAmbientCredentialMalformedEntryFallsThrough: a malformed entry falls through, not aborts.
func TestOCIAmbientCredentialMalformedEntryFallsThrough(t *testing.T) {
	t.Parallel()

	home := t.TempDir()
	xdgConfig := t.TempDir()

	// High priority: valid JSON but the auth value decodes to "nocolon", rejected at lookup.
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

// TestOCIAmbientCredentialPrefersRepositoryScopedEntry: a containers-auth entry
// scoped to the repository must beat the registry-wide entry.
func TestOCIAmbientCredentialPrefersRepositoryScopedEntry(t *testing.T) {
	t.Parallel()

	home := t.TempDir()
	writeAuthFileKeys(t, filepath.Join(home, ".docker", "config.json"), map[string]string{
		"quay.io":             "registry-wide",
		"quay.io/myorg":       "org-scoped",
		"quay.io/myorg/vpc":   "repo-scoped",
		"quay.io/otherorg":    "other-org",
		"quay.io/myorg/other": "other-repo",
	})

	newStore := getter.NewOCIRepositoryStore(logger.CreateLogger(), credentialVenv(home, nil))

	store, err := newStore(t.Context(), "quay.io", "myorg/vpc")
	require.NoError(t, err)
	assert.Equal(t, "repo-scoped", credentialFor(t, store, "quay.io").Username)

	// One segment less: the org-scoped entry is the most specific match.
	store, err = newStore(t.Context(), "quay.io", "myorg/unlisted")
	require.NoError(t, err)
	assert.Equal(t, "org-scoped", credentialFor(t, store, "quay.io").Username)

	// No scoped entry matches: the registry-wide entry serves.
	store, err = newStore(t.Context(), "quay.io", "unlisted/vpc")
	require.NoError(t, err)
	assert.Equal(t, "registry-wide", credentialFor(t, store, "quay.io").Username)
}

// TestOCIAmbientCredentialIgnoresOtherNamespaceEntry: with only repository-scoped
// entries present, an unrelated namespace must resolve to no credential rather
// than borrowing another namespace's identity.
func TestOCIAmbientCredentialIgnoresOtherNamespaceEntry(t *testing.T) {
	t.Parallel()

	home := t.TempDir()
	writeAuthFileKeys(t, filepath.Join(home, ".docker", "config.json"), map[string]string{
		"quay.io/org-a": "user-a",
		"quay.io/org-b": "user-b",
	})

	newStore := getter.NewOCIRepositoryStore(logger.CreateLogger(), credentialVenv(home, nil))

	// Repeated, because the credential a collapsing lookup returns varies per call.
	for range 20 {
		store, err := newStore(t.Context(), "quay.io", "org-c/vpc")
		require.NoError(t, err)
		assert.Equal(t, auth.EmptyCredential, credentialFor(t, store, "quay.io"))
	}

	// The namespaces that do have entries still resolve, each to its own.
	store, err := newStore(t.Context(), "quay.io", "org-a/vpc")
	require.NoError(t, err)
	assert.Equal(t, "user-a", credentialFor(t, store, "quay.io").Username)

	store, err = newStore(t.Context(), "quay.io", "org-b/vpc")
	require.NoError(t, err)
	assert.Equal(t, "user-b", credentialFor(t, store, "quay.io").Username)
}

// TestOCIAmbientCredentialDockerHubSpellings: oras sends Docker Hub traffic to
// registry-1.docker.io, so every spelling a login tool writes must resolve.
func TestOCIAmbientCredentialDockerHubSpellings(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name    string
		authKey string
	}{
		{name: "docker login writes the legacy index url", authKey: "https://index.docker.io/v1/"},
		{name: "podman login writes the bare domain", authKey: "docker.io"},
		{name: "index.docker.io", authKey: "index.docker.io"},
		{name: "registry-1.docker.io", authKey: "registry-1.docker.io"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			home := t.TempDir()
			writeAuthFileKeys(t, filepath.Join(home, ".config", "containers", "auth.json"), map[string]string{
				tc.authKey: "hub-user",
			})
			writeAuthFileKeys(t, filepath.Join(home, ".docker", "config.json"), map[string]string{
				tc.authKey: "hub-user",
			})

			newStore := getter.NewOCIRepositoryStore(logger.CreateLogger(), credentialVenv(home, nil))

			store, err := newStore(t.Context(), "docker.io", "myorg/vpc")
			require.NoError(t, err)

			// oras rewrites the Docker Hub host before asking for a credential.
			assert.Equal(t, "hub-user", credentialFor(t, store, "registry-1.docker.io").Username)
		})
	}
}

// TestOCIAmbientCredentialLegacySchemePrefixedEntry: a legacy Docker config
// spells the registry as a URL, which must still resolve for the bare host.
func TestOCIAmbientCredentialLegacySchemePrefixedEntry(t *testing.T) {
	t.Parallel()

	home := t.TempDir()
	writeAuthFileKeys(t, filepath.Join(home, ".docker", "config.json"), map[string]string{
		"https://" + testRegistry + "/": "legacy-user",
	})

	newStore := getter.NewOCIRepositoryStore(logger.CreateLogger(), credentialVenv(home, nil))

	store, err := newStore(t.Context(), testRegistry, "modules/vpc")
	require.NoError(t, err)
	assert.Equal(t, "legacy-user", credentialFor(t, store, testRegistry).Username)
}

// credentialVenv builds a hermetic OS-backed Venv with home and extra env set.
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

// writeAuthFile writes a config.json-format credential file granting user/pass for registry.
func writeAuthFile(t *testing.T, path, registry, user, pass string) {
	t.Helper()

	writeRawAuthFile(t, path, registry, base64.StdEncoding.EncodeToString([]byte(user+":"+pass)))
}

// writeAuthFileKeys writes a credential file declaring every key in users, each
// granting its own username, so key selection can be observed.
func writeAuthFileKeys(t *testing.T, path string, users map[string]string) {
	t.Helper()

	auths := map[string]any{}

	for key, user := range users {
		auths[key] = map[string]string{
			"auth": base64.StdEncoding.EncodeToString([]byte(user + ":" + user + "-pass")),
		}
	}

	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))

	data, err := json.Marshal(map[string]any{"auths": auths})
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(path, data, 0o600))
}

// writeRawAuthFile writes a config.json credential file with a raw base64 auth value.
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
