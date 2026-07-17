package getter_test

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"path/filepath"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/getter"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/gruntwork-io/terragrunt/test/helpers/venvtest"
	"oras.land/oras-go/v2/registry/remote/auth"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	testDockerConfig = "/virtual/docker-config"
	testHome         = "/virtual/home"
	testRuntimeDir   = "/virtual/run"
	testRegistry     = "registry.example.com"
	testXDGConfig    = "/virtual/config"
)

func TestNewOCIRepositoryStoreReference(t *testing.T) {
	t.Parallel()

	newStore := getter.NewOCIRepositoryStore(logger.CreateLogger(), credentialVenv(testHome, nil))

	store, err := newStore(t.Context(), "127.0.0.1:5000", "org/team/vpc")
	require.NoError(t, err)

	remoteStore, castOK := store.(getter.OCIRemoteStore)
	require.True(t, castOK, "default store must be the oras-backed remote store")
	assert.Equal(t, "127.0.0.1:5000", remoteStore.Repo.Reference.Registry)
	assert.Equal(t, "org/team/vpc", remoteStore.Repo.Reference.Repository)

	_, err = newStore(t.Context(), "127.0.0.1:5000", "ORG/Bad")
	require.ErrorContains(t, err, "parsing OCI repository reference")
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

			home := testHome
			v := credentialVenv(home, tc.env)
			// Ambient file present to prove static credentials win over it.
			writeAuthFile(t, v.FS, filepath.Join(home, ".docker", "config.json"), testRegistry, "ambient-user", "ambient-pass")

			newStore := getter.NewOCIRepositoryStore(logger.CreateLogger(), v)

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

	newStore := getter.NewOCIRepositoryStore(logger.CreateLogger(), credentialVenv(testHome, env))

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

	home := testHome
	env := map[string]string{
		getter.EnvOCIToken:    "scoped-token",
		getter.EnvOCIRegistry: testRegistry,
	}
	v := credentialVenv(home, env)
	writeAuthFile(t, v.FS, filepath.Join(home, ".docker", "config.json"), otherRegistry, "ambient-user", "ambient-pass")

	newStore := getter.NewOCIRepositoryStore(logger.CreateLogger(), v)

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

	home := testHome
	xdgConfig := testXDGConfig
	dockerConfigEnv := testDockerConfig
	env := map[string]string{"XDG_CONFIG_HOME": xdgConfig, "DOCKER_CONFIG": dockerConfigEnv}
	v := credentialVenv(home, env)

	writeAuthFile(t, v.FS, filepath.Join(xdgConfig, "containers", "auth.json"), testRegistry, "xdg-config", "pw")
	writeAuthFile(t, v.FS, filepath.Join(home, ".docker", "config.json"), testRegistry, "home-docker", "pw")
	// DOCKER_CONFIG must be ignored: a credential here must never win.
	writeAuthFile(t, v.FS, filepath.Join(dockerConfigEnv, "config.json"), testRegistry, "docker-config-env", "pw")

	newStore := getter.NewOCIRepositoryStore(logger.CreateLogger(), v)

	store, err := newStore(t.Context(), testRegistry, "modules/vpc")
	require.NoError(t, err)
	assert.Equal(t, "xdg-config", credentialFor(t, store, testRegistry).Username)

	// With no XDG_CONFIG_HOME the Docker config wins and DOCKER_CONFIG stays ignored.
	newStore = getter.NewOCIRepositoryStore(
		logger.CreateLogger(),
		v.WithEnv(map[string]string{"DOCKER_CONFIG": dockerConfigEnv}),
	)

	store, err = newStore(t.Context(), testRegistry, "modules/vpc")
	require.NoError(t, err)
	assert.Equal(t, "home-docker", credentialFor(t, store, testRegistry).Username)
}

// TestOCIAmbientCredentialXDGRuntimeDir: runtime-dir wins on Linux and is ignored elsewhere.
func TestOCIAmbientCredentialXDGRuntimeDir(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string
		goos     string
		wantUser string
	}{
		{name: "linux", goos: "linux", wantUser: "xdg-runtime"},
		{name: "darwin", goos: "darwin", wantUser: "home-docker"},
		{name: "windows", goos: "windows", wantUser: "home-docker"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			home := testHome
			runtimeDir := testRuntimeDir
			v := credentialVenvForGOOS(tc.goos, home, map[string]string{"XDG_RUNTIME_DIR": runtimeDir})
			writeAuthFile(t, v.FS, filepath.Join(runtimeDir, "containers", "auth.json"), testRegistry, "xdg-runtime", "pw")
			writeAuthFile(t, v.FS, filepath.Join(home, ".docker", "config.json"), testRegistry, "home-docker", "pw")

			newStore := getter.NewOCIRepositoryStore(logger.CreateLogger(), v)
			store, err := newStore(t.Context(), testRegistry, "modules/vpc")
			require.NoError(t, err)

			assert.Equal(t, tc.wantUser, credentialFor(t, store, testRegistry).Username)
		})
	}
}

func TestOCIAmbientCredentialPlatformConfigOrder(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string
		goos     string
		wantUser string
	}{
		{name: "linux prefers xdg config", goos: "linux", wantUser: "xdg-config"},
		{name: "darwin prefers literal home config", goos: "darwin", wantUser: "home-config"},
		{name: "windows prefers literal home config", goos: "windows", wantUser: "home-config"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			home := testHome
			xdgConfig := testXDGConfig
			v := credentialVenvForGOOS(tc.goos, home, map[string]string{"XDG_CONFIG_HOME": xdgConfig})
			writeAuthFile(t, v.FS, filepath.Join(home, ".config", "containers", "auth.json"), testRegistry, "home-config", "pw")
			writeAuthFile(t, v.FS, filepath.Join(xdgConfig, "containers", "auth.json"), testRegistry, "xdg-config", "pw")

			newStore := getter.NewOCIRepositoryStore(logger.CreateLogger(), v)
			store, err := newStore(t.Context(), testRegistry, "modules/vpc")
			require.NoError(t, err)

			assert.Equal(t, tc.wantUser, credentialFor(t, store, testRegistry).Username)
		})
	}
}

func TestOCIAmbientCredentialUsesInjectedHomeDirectory(t *testing.T) {
	t.Parallel()

	home := testHome
	v := credentialVenv(home, map[string]string{
		"HOME":        "/decoy-home",
		"USERPROFILE": "/decoy-profile",
	})
	writeAuthFile(t, v.FS, filepath.Join(home, ".docker", "config.json"), testRegistry, "injected-home", "pw")
	writeAuthFile(t, v.FS, filepath.Join("/decoy-home", ".docker", "config.json"), testRegistry, "decoy-home", "pw")

	newStore := getter.NewOCIRepositoryStore(logger.CreateLogger(), v)
	store, err := newStore(t.Context(), testRegistry, "modules/vpc")
	require.NoError(t, err)

	assert.Equal(t, "injected-home", credentialFor(t, store, testRegistry).Username)
}

func TestOCIAmbientCredentialContinuesAfterHomeLookupError(t *testing.T) {
	t.Parallel()

	xdgConfig := testXDGConfig
	v := credentialVenv("", map[string]string{"XDG_CONFIG_HOME": xdgConfig}).
		WithUserHomeDir(func() (string, error) { return "", errors.New("home unavailable") })
	writeAuthFile(t, v.FS, filepath.Join(xdgConfig, "containers", "auth.json"), testRegistry, "xdg-config", "pw")

	newStore := getter.NewOCIRepositoryStore(logger.CreateLogger(), v)
	store, err := newStore(t.Context(), testRegistry, "modules/vpc")
	require.NoError(t, err)

	assert.Equal(t, "xdg-config", credentialFor(t, store, testRegistry).Username)
}

func TestOCIAmbientCredentialScopedToRegistry(t *testing.T) {
	t.Parallel()

	home := testHome
	v := credentialVenv(home, nil)
	writeAuthFile(t, v.FS, filepath.Join(home, ".docker", "config.json"), testRegistry, "scoped-user", "scoped-pass")

	newStore := getter.NewOCIRepositoryStore(logger.CreateLogger(), v)

	store, err := newStore(t.Context(), "other-registry.example.com", "modules/vpc")
	require.NoError(t, err)

	assert.Equal(t, auth.EmptyCredential, credentialFor(t, store, "other-registry.example.com"))
}

func TestOCIAmbientCredentialNoFiles(t *testing.T) {
	t.Parallel()

	newStore := getter.NewOCIRepositoryStore(logger.CreateLogger(), credentialVenv(testHome, nil))

	store, err := newStore(t.Context(), testRegistry, "modules/vpc")
	require.NoError(t, err)

	assert.Equal(t, auth.EmptyCredential, credentialFor(t, store, testRegistry))
}

// TestOCIAmbientCredentialInvalidFileSkipped: an unopenable file is skipped, a valid one still resolves.
func TestOCIAmbientCredentialInvalidFileSkipped(t *testing.T) {
	t.Parallel()

	home := testHome
	xdgConfig := testXDGConfig
	v := credentialVenv(home, map[string]string{"XDG_CONFIG_HOME": xdgConfig})

	// High priority: invalid top-level JSON, so the file cannot be opened as a store.
	badPath := filepath.Join(xdgConfig, "containers", "auth.json")
	require.NoError(t, vfs.WriteFile(v.FS, badPath, []byte("not json"), 0o600))
	// Lower priority: valid.
	writeAuthFile(t, v.FS, filepath.Join(home, ".docker", "config.json"), testRegistry, "good-user", "good-pass")

	newStore := getter.NewOCIRepositoryStore(logger.CreateLogger(), v)

	store, err := newStore(t.Context(), testRegistry, "modules/vpc")
	require.NoError(t, err)
	assert.Equal(t, "good-user", credentialFor(t, store, testRegistry).Username)
}

// TestOCIAmbientCredentialMalformedEntryFallsThrough: a malformed entry falls through, not aborts.
func TestOCIAmbientCredentialMalformedEntryFallsThrough(t *testing.T) {
	t.Parallel()

	home := testHome
	xdgConfig := testXDGConfig
	v := credentialVenv(home, map[string]string{"XDG_CONFIG_HOME": xdgConfig})

	// High priority: valid JSON but the auth value decodes to "nocolon", rejected at lookup.
	writeRawAuthFile(
		t,
		v.FS,
		filepath.Join(xdgConfig, "containers", "auth.json"),
		testRegistry,
		base64.StdEncoding.EncodeToString([]byte("nocolon")),
	)
	// Lower priority: valid.
	writeAuthFile(t, v.FS, filepath.Join(home, ".docker", "config.json"), testRegistry, "good-user", "good-pass")

	newStore := getter.NewOCIRepositoryStore(logger.CreateLogger(), v)

	store, err := newStore(t.Context(), testRegistry, "modules/vpc")
	require.NoError(t, err)
	assert.Equal(t, "good-user", credentialFor(t, store, testRegistry).Username)
}

func TestOCIAmbientCredentialMalformedUnrelatedEntryIgnored(t *testing.T) {
	t.Parallel()

	home := testHome
	v := credentialVenv(home, nil)
	validAuth := base64.StdEncoding.EncodeToString([]byte("valid-user:valid-pass"))
	invalidAuth := base64.StdEncoding.EncodeToString([]byte("missing-colon"))
	data, err := json.Marshal(map[string]any{
		"auths": map[string]any{
			testRegistry:            map[string]string{"auth": validAuth},
			"unrelated.example.com": map[string]string{"auth": invalidAuth},
		},
	})
	require.NoError(t, err)
	require.NoError(t, vfs.WriteFile(v.FS, filepath.Join(home, ".docker", "config.json"), data, 0o600))

	newStore := getter.NewOCIRepositoryStore(logger.CreateLogger(), v)
	store, err := newStore(t.Context(), testRegistry, "modules/vpc")
	require.NoError(t, err)

	assert.Equal(t, "valid-user", credentialFor(t, store, testRegistry).Username)
}

// TestOCIAmbientCredentialPrefersRepositoryScopedEntry: a containers-auth entry
// scoped to the repository must beat the registry-wide entry.
func TestOCIAmbientCredentialPrefersRepositoryScopedEntry(t *testing.T) {
	t.Parallel()

	home := testHome
	v := credentialVenv(home, nil)
	writeAuthFileKeys(t, v.FS, filepath.Join(home, ".docker", "config.json"), map[string]string{
		"quay.io":             "registry-wide",
		"quay.io/myorg":       "org-scoped",
		"quay.io/myorg/vpc":   "repo-scoped",
		"quay.io/otherorg":    "other-org",
		"quay.io/myorg/other": "other-repo",
	})

	newStore := getter.NewOCIRepositoryStore(logger.CreateLogger(), v)

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

	home := testHome
	v := credentialVenv(home, nil)
	writeAuthFileKeys(t, v.FS, filepath.Join(home, ".docker", "config.json"), map[string]string{
		"quay.io/org-a": "user-a",
		"quay.io/org-b": "user-b",
	})

	newStore := getter.NewOCIRepositoryStore(logger.CreateLogger(), v)

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

func TestOCIAmbientCredentialCanonicalAliasFallsThrough(t *testing.T) {
	t.Parallel()

	v := credentialVenv(testHome, nil)
	validAuth := base64.StdEncoding.EncodeToString([]byte("valid-user:valid-pass"))
	invalidAuth := base64.StdEncoding.EncodeToString([]byte("missing-colon"))
	data, err := json.Marshal(map[string]any{
		"auths": map[string]any{
			"http://" + testRegistry + "/":  map[string]string{},
			"https://" + testRegistry + "/": map[string]string{"auth": invalidAuth},
			testRegistry:                    map[string]string{"auth": validAuth},
		},
	})
	require.NoError(t, err)
	require.NoError(t, vfs.WriteFile(v.FS, filepath.Join(testHome, ".docker", "config.json"), data, 0o600))

	newStore := getter.NewOCIRepositoryStore(logger.CreateLogger(), v)
	store, err := newStore(t.Context(), testRegistry, "modules/vpc")
	require.NoError(t, err)

	assert.Equal(t, "valid-user", credentialFor(t, store, testRegistry).Username)
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
		{name: "scheme and trailing slash", authKey: "https://registry-1.docker.io/"},
		{name: "repository-scoped registry alias", authKey: "registry-1.docker.io/myorg"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			home := testHome
			v := credentialVenv(home, nil)
			writeAuthFileKeys(t, v.FS, filepath.Join(home, ".config", "containers", "auth.json"), map[string]string{
				tc.authKey: "hub-user",
			})
			writeAuthFileKeys(t, v.FS, filepath.Join(home, ".docker", "config.json"), map[string]string{
				tc.authKey: "hub-user",
			})

			newStore := getter.NewOCIRepositoryStore(logger.CreateLogger(), v)

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

	home := testHome
	v := credentialVenv(home, nil)
	writeAuthFileKeys(t, v.FS, filepath.Join(home, ".docker", "config.json"), map[string]string{
		"https://" + testRegistry + "/": "legacy-user",
	})

	newStore := getter.NewOCIRepositoryStore(logger.CreateLogger(), v)

	store, err := newStore(t.Context(), testRegistry, "modules/vpc")
	require.NoError(t, err)
	assert.Equal(t, "legacy-user", credentialFor(t, store, testRegistry).Username)
}

// credentialVenv builds a hermetic Linux Venv with home and extra env set.
func credentialVenv(home string, extra map[string]string) venv.Venv {
	return credentialVenvForGOOS("linux", home, extra)
}

// credentialVenvForGOOS builds a hermetic in-memory Venv for goos.
func credentialVenvForGOOS(goos, home string, extra map[string]string) venv.Venv {
	env := map[string]string{}
	for name, value := range extra {
		env[name] = value
	}

	return venvtest.New().
		WithGOOS(goos).
		WithUserHomeDir(func() (string, error) { return home, nil }).
		WithEnv(env)
}

// credentialFor resolves the credential the store would send for registry.
func credentialFor(t *testing.T, store getter.OCIRepositoryStore, registry string) auth.Credential {
	t.Helper()

	remoteStore, castOK := store.(getter.OCIRemoteStore)
	require.True(t, castOK, "default store must be the oras-backed remote store")

	client, castOK := remoteStore.Repo.Client.(*auth.Client)
	require.True(t, castOK, "default store must use an oras auth client")

	cred, err := client.Credential(t.Context(), registry)
	require.NoError(t, err)

	return cred
}

// writeAuthFile writes a config.json-format credential file granting user/pass for registry.
func writeAuthFile(t *testing.T, fs vfs.FS, path, registry, user, pass string) {
	t.Helper()

	writeRawAuthFile(t, fs, path, registry, base64.StdEncoding.EncodeToString([]byte(user+":"+pass)))
}

// writeAuthFileKeys writes a credential file declaring every key in users, each
// granting its own username, so key selection can be observed.
func writeAuthFileKeys(t *testing.T, fs vfs.FS, path string, users map[string]string) {
	t.Helper()

	auths := map[string]any{}

	for key, user := range users {
		auths[key] = map[string]string{
			"auth": base64.StdEncoding.EncodeToString([]byte(user + ":" + user + "-pass")),
		}
	}

	data, err := json.Marshal(map[string]any{"auths": auths})
	require.NoError(t, err)
	require.NoError(t, vfs.WriteFile(fs, path, data, 0o600))
}

// writeRawAuthFile writes a config.json credential file with a raw base64 auth value.
func writeRawAuthFile(t *testing.T, fs vfs.FS, path, registry, encodedAuth string) {
	t.Helper()

	content := map[string]any{
		"auths": map[string]any{
			registry: map[string]string{"auth": encodedAuth},
		},
	}

	data, err := json.Marshal(content)
	require.NoError(t, err)
	require.NoError(t, vfs.WriteFile(fs, path, data, 0o600))
}
