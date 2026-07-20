package getter_test

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"path/filepath"
	"sync/atomic"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/getter"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/internal/vexec"
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

// TestOCIHelperCredentialFromCredHelpers: a per-registry credHelpers entry mints a credential.
func TestOCIHelperCredentialFromCredHelpers(t *testing.T) {
	t.Parallel()

	var stdin string

	exec := stubHelperExec(t, "ecr-login", func(in string) vexec.Result {
		stdin = in
		return vexec.Result{Stdout: []byte(`{"ServerURL":"` + testRegistry + `","Username":"AWS","Secret":"minted-pass"}`)}
	}, nil)

	home := testHome
	v := credentialVenv(home, nil).WithExec(exec)
	writeHelperConfig(t, v.FS, filepath.Join(home, ".docker", "config.json"),
		map[string]string{testRegistry: "ecr-login"}, "")

	newStore := getter.NewOCIRepositoryStore(logger.CreateLogger(), v)

	store, err := newStore(t.Context(), testRegistry, "modules/vpc")
	require.NoError(t, err)

	assert.Equal(t, auth.Credential{Username: "AWS", Password: "minted-pass"}, credentialFor(t, store, testRegistry))
	assert.Equal(t, testRegistry, stdin, "the helper must receive the registry host on stdin")
}

// TestOCIHelperCredentialBearerToken: a "<token>" username marks a bearer token.
func TestOCIHelperCredentialBearerToken(t *testing.T) {
	t.Parallel()

	exec := stubHelperExec(t, "ecr-login", func(string) vexec.Result {
		return vexec.Result{Stdout: []byte(`{"Username":"<token>","Secret":"refresh-token"}`)}
	}, nil)

	home := testHome
	v := credentialVenv(home, nil).WithExec(exec)
	writeHelperConfig(t, v.FS, filepath.Join(home, ".docker", "config.json"),
		map[string]string{testRegistry: "ecr-login"}, "")

	newStore := getter.NewOCIRepositoryStore(logger.CreateLogger(), v)

	store, err := newStore(t.Context(), testRegistry, "modules/vpc")
	require.NoError(t, err)

	assert.Equal(t, auth.Credential{RefreshToken: "refresh-token"}, credentialFor(t, store, testRegistry))
}

// TestOCIHelperCredentialFromCredsStore: credsStore serves hosts with no credHelpers entry.
func TestOCIHelperCredentialFromCredsStore(t *testing.T) {
	t.Parallel()

	exec := stubHelperExec(t, "osxkeychain", func(string) vexec.Result {
		return vexec.Result{Stdout: []byte(`{"Username":"store-user","Secret":"store-pass"}`)}
	}, nil)

	home := testHome
	v := credentialVenv(home, nil).WithExec(exec)
	writeHelperConfig(t, v.FS, filepath.Join(home, ".docker", "config.json"), nil, "osxkeychain")

	newStore := getter.NewOCIRepositoryStore(logger.CreateLogger(), v)

	store, err := newStore(t.Context(), testRegistry, "modules/vpc")
	require.NoError(t, err)

	assert.Equal(t, auth.Credential{Username: "store-user", Password: "store-pass"}, credentialFor(t, store, testRegistry))
}

// TestOCIHelperCredentialNotInKeychainIsEmpty: a not-found helper result is empty, not an error.
func TestOCIHelperCredentialNotInKeychainIsEmpty(t *testing.T) {
	t.Parallel()

	// Helpers report not-found on stdout and exit non-zero, per the protocol.
	exec := stubHelperExec(t, "osxkeychain", func(string) vexec.Result {
		return vexec.Result{Stdout: []byte("credentials not found in native keychain\n"), ExitCode: 1}
	}, nil)

	home := testHome
	v := credentialVenv(home, nil).WithExec(exec)
	writeHelperConfig(t, v.FS, filepath.Join(home, ".docker", "config.json"),
		map[string]string{testRegistry: "osxkeychain"}, "")

	newStore := getter.NewOCIRepositoryStore(logger.CreateLogger(), v)

	store, err := newStore(t.Context(), testRegistry, "modules/vpc")
	require.NoError(t, err)

	assert.Equal(t, auth.EmptyCredential, credentialFor(t, store, testRegistry))
}

// TestOCIHelperCredentialBinaryMissing: a configured helper not on PATH is a typed error.
func TestOCIHelperCredentialBinaryMissing(t *testing.T) {
	t.Parallel()

	exec := vexec.NewMemExec(
		func(context.Context, vexec.Invocation) vexec.Result { return vexec.Result{} },
		vexec.WithLookPath(func(string) (string, error) {
			return "", errors.New("executable file not found in $PATH")
		}),
	)

	home := testHome
	v := credentialVenv(home, nil).WithExec(exec)
	writeHelperConfig(t, v.FS, filepath.Join(home, ".docker", "config.json"),
		map[string]string{testRegistry: "ecr-login"}, "")

	newStore := getter.NewOCIRepositoryStore(logger.CreateLogger(), v)

	store, err := newStore(t.Context(), testRegistry, "modules/vpc")
	require.NoError(t, err)

	_, credErr := credentialForErr(t, store, testRegistry)

	var helperErr getter.OCICredentialHelperError

	require.ErrorAs(t, credErr, &helperErr)
	assert.Equal(t, "ecr-login", helperErr.Helper)
}

// TestOCIHelperCredentialMalformedOutput: non-JSON helper output is a typed error.
func TestOCIHelperCredentialMalformedOutput(t *testing.T) {
	t.Parallel()

	exec := stubHelperExec(t, "ecr-login", func(string) vexec.Result {
		return vexec.Result{Stdout: []byte("not-json-secret-material")}
	}, nil)

	home := testHome
	v := credentialVenv(home, nil).WithExec(exec)
	writeHelperConfig(t, v.FS, filepath.Join(home, ".docker", "config.json"),
		map[string]string{testRegistry: "ecr-login"}, "")

	newStore := getter.NewOCIRepositoryStore(logger.CreateLogger(), v)

	store, err := newStore(t.Context(), testRegistry, "modules/vpc")
	require.NoError(t, err)

	_, credErr := credentialForErr(t, store, testRegistry)
	require.ErrorIs(t, credErr, getter.ErrOCIHelperMalformedOutput)
	assert.NotContains(t, credErr.Error(), "not-json-secret-material", "the error must not carry the helper output")
}

// TestOCIHelperCredentialMintedPerResolution: the helper runs on every resolution.
func TestOCIHelperCredentialMintedPerResolution(t *testing.T) {
	t.Parallel()

	var calls atomic.Int32

	exec := stubHelperExec(t, "ecr-login", func(string) vexec.Result {
		return vexec.Result{Stdout: []byte(`{"Username":"AWS","Secret":"pass"}`)}
	}, &calls)

	home := testHome
	v := credentialVenv(home, nil).WithExec(exec)
	writeHelperConfig(t, v.FS, filepath.Join(home, ".docker", "config.json"),
		map[string]string{testRegistry: "ecr-login"}, "")

	newStore := getter.NewOCIRepositoryStore(logger.CreateLogger(), v)

	store, err := newStore(t.Context(), testRegistry, "modules/vpc")
	require.NoError(t, err)

	credentialFor(t, store, testRegistry)
	credentialFor(t, store, testRegistry)

	assert.EqualValues(t, 2, calls.Load(), "the helper must re-mint on every resolution")
}

// TestOCIHelperCredentialInlineBeatsHelper: an inline auth entry wins, the helper never runs.
func TestOCIHelperCredentialInlineBeatsHelper(t *testing.T) {
	t.Parallel()

	var calls atomic.Int32

	exec := stubHelperExec(t, "ecr-login", func(string) vexec.Result {
		return vexec.Result{Stdout: []byte(`{"Username":"AWS","Secret":"helper-pass"}`)}
	}, &calls)

	home := testHome
	v := credentialVenv(home, nil).WithExec(exec)
	path := filepath.Join(home, ".docker", "config.json")
	// One file carrying both an inline auth and a helper for the same registry.
	writeDockerConfig(t, v.FS, path,
		map[string]string{testRegistry: "inline-user:inline-pass"},
		map[string]string{testRegistry: "ecr-login"}, "")

	newStore := getter.NewOCIRepositoryStore(logger.CreateLogger(), v)

	store, err := newStore(t.Context(), testRegistry, "modules/vpc")
	require.NoError(t, err)

	want := auth.Credential{Username: "inline-user", Password: "inline-pass"}
	assert.Equal(t, want, credentialFor(t, store, testRegistry))
	assert.EqualValues(t, 0, calls.Load(), "an inline credential must short-circuit before the helper")
}

// TestOCIHelperCredentialCredHelpersBeatsCredsStore: a per-registry helper wins over credsStore.
func TestOCIHelperCredentialCredHelpersBeatsCredsStore(t *testing.T) {
	t.Parallel()

	var storeCalls atomic.Int32

	handler := func(_ context.Context, inv vexec.Invocation) vexec.Result {
		if inv.Name == "docker-credential-osxkeychain" {
			storeCalls.Add(1)

			return vexec.Result{Stdout: []byte(`{"Username":"store","Secret":"store-pass"}`)}
		}

		return vexec.Result{Stdout: []byte(`{"Username":"registry","Secret":"registry-pass"}`)}
	}
	exec := vexec.NewMemExec(handler, vexec.WithLookPath(func(file string) (string, error) {
		return "/usr/local/bin/" + file, nil
	}))

	home := testHome
	v := credentialVenv(home, nil).WithExec(exec)
	writeHelperConfig(t, v.FS, filepath.Join(home, ".docker", "config.json"),
		map[string]string{testRegistry: "ecr-login"}, "osxkeychain")

	newStore := getter.NewOCIRepositoryStore(logger.CreateLogger(), v)

	store, err := newStore(t.Context(), testRegistry, "modules/vpc")
	require.NoError(t, err)

	want := auth.Credential{Username: "registry", Password: "registry-pass"}
	assert.Equal(t, want, credentialFor(t, store, testRegistry))
	assert.EqualValues(t, 0, storeCalls.Load(), "the per-registry helper must win over credsStore")
}

// TestOCIHelperCredentialCredsStoreFailureIsNonFatal: a global credsStore helper
// failure must not break a fetch, unlike an explicit per-registry helper.
func TestOCIHelperCredentialCredsStoreFailureIsNonFatal(t *testing.T) {
	t.Parallel()

	exec := vexec.NewMemExec(
		func(context.Context, vexec.Invocation) vexec.Result { return vexec.Result{} },
		vexec.WithLookPath(func(string) (string, error) {
			return "", errors.New("executable file not found in $PATH")
		}),
	)

	home := testHome
	v := credentialVenv(home, nil).WithExec(exec)
	writeHelperConfig(t, v.FS, filepath.Join(home, ".docker", "config.json"), nil, "desktop")

	newStore := getter.NewOCIRepositoryStore(logger.CreateLogger(), v)

	store, err := newStore(t.Context(), testRegistry, "modules/vpc")
	require.NoError(t, err)

	cred, credErr := credentialForErr(t, store, testRegistry)
	require.NoError(t, credErr, "a missing credsStore helper must fall through, not fail the fetch")
	assert.Equal(t, auth.EmptyCredential, cred)
}

// TestOCIHelperCredentialScopedStaticFallsThroughToHelper: a scoped static
// credential still lets other registries resolve via their helper.
func TestOCIHelperCredentialScopedStaticFallsThroughToHelper(t *testing.T) {
	t.Parallel()

	const helperRegistry = "other.example.com"

	exec := stubHelperExec(t, "ecr-login", func(string) vexec.Result {
		return vexec.Result{Stdout: []byte(`{"Username":"AWS","Secret":"minted"}`)}
	}, nil)

	home := testHome
	env := map[string]string{getter.EnvOCIToken: "scoped-token", getter.EnvOCIRegistry: testRegistry}
	v := credentialVenv(home, env).WithExec(exec)
	writeHelperConfig(t, v.FS, filepath.Join(home, ".docker", "config.json"),
		map[string]string{helperRegistry: "ecr-login"}, "")

	newStore := getter.NewOCIRepositoryStore(logger.CreateLogger(), v)

	scoped, err := newStore(t.Context(), testRegistry, "modules/vpc")
	require.NoError(t, err)
	assert.Equal(t, auth.Credential{AccessToken: "scoped-token"}, credentialFor(t, scoped, testRegistry))

	other, err := newStore(t.Context(), helperRegistry, "modules/vpc")
	require.NoError(t, err)
	assert.Equal(t, auth.Credential{Username: "AWS", Password: "minted"}, credentialFor(t, other, helperRegistry))
}

// TestOCIHelperCredentialFirstFileHelperWins: with two config files, the
// higher-precedence file's helper resolves first.
func TestOCIHelperCredentialFirstFileHelperWins(t *testing.T) {
	t.Parallel()

	exec := vexec.NewMemExec(func(_ context.Context, inv vexec.Invocation) vexec.Result {
		if inv.Name == "docker-credential-first" {
			return vexec.Result{Stdout: []byte(`{"Username":"first","Secret":"first-pass"}`)}
		}

		return vexec.Result{Stdout: []byte(`{"Username":"second","Secret":"second-pass"}`)}
	}, vexec.WithLookPath(func(file string) (string, error) { return "/usr/local/bin/" + file, nil }))

	home := testHome
	xdgConfig := testXDGConfig
	v := credentialVenv(home, map[string]string{"XDG_CONFIG_HOME": xdgConfig}).WithExec(exec)
	// XDG containers auth is searched before the Docker config.
	writeHelperConfig(t, v.FS, filepath.Join(xdgConfig, "containers", "auth.json"),
		map[string]string{testRegistry: "first"}, "")
	writeHelperConfig(t, v.FS, filepath.Join(home, ".docker", "config.json"),
		map[string]string{testRegistry: "second"}, "")

	newStore := getter.NewOCIRepositoryStore(logger.CreateLogger(), v)

	store, err := newStore(t.Context(), testRegistry, "modules/vpc")
	require.NoError(t, err)
	assert.Equal(t, auth.Credential{Username: "first", Password: "first-pass"}, credentialFor(t, store, testRegistry))
}

// TestOCIHelperCredentialDockerHubSpellingCanonicalized: a docker.io spelling in
// credHelpers serves a registry-1.docker.io lookup.
func TestOCIHelperCredentialDockerHubSpellingCanonicalized(t *testing.T) {
	t.Parallel()

	exec := stubHelperExec(t, "desktop", func(string) vexec.Result {
		return vexec.Result{Stdout: []byte(`{"Username":"hub","Secret":"hub-pass"}`)}
	}, nil)

	home := testHome
	v := credentialVenv(home, nil).WithExec(exec)
	writeHelperConfig(t, v.FS, filepath.Join(home, ".docker", "config.json"),
		map[string]string{"https://index.docker.io/v1/": "desktop"}, "")

	newStore := getter.NewOCIRepositoryStore(logger.CreateLogger(), v)

	store, err := newStore(t.Context(), "registry-1.docker.io", "library/alpine")
	require.NoError(t, err)

	want := auth.Credential{Username: "hub", Password: "hub-pass"}
	assert.Equal(t, want, credentialFor(t, store, "registry-1.docker.io"))
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

	cred, err := credentialForErr(t, store, registry)
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

// stubHelperExec builds a MemExec that dispatches docker-credential-<name> get
// to reply, records invocation count in calls, and asserts the invocation shape.
func stubHelperExec(t *testing.T, name string, reply func(stdin string) vexec.Result, calls *atomic.Int32) vexec.Exec {
	t.Helper()

	bin := "docker-credential-" + name

	handler := func(_ context.Context, inv vexec.Invocation) vexec.Result {
		assert.Equal(t, bin, inv.Name, "the helper binary must carry the docker-credential prefix")
		assert.Equal(t, []string{"get"}, inv.Args, "the helper must be invoked with the get action")

		if calls != nil {
			calls.Add(1)
		}

		in, err := io.ReadAll(inv.Stdin)
		require.NoError(t, err)

		return reply(string(in))
	}

	lookPath := func(file string) (string, error) {
		if file == bin {
			return "/usr/local/bin/" + file, nil
		}

		return "", errors.New("executable file not found in $PATH")
	}

	return vexec.NewMemExec(handler, vexec.WithLookPath(lookPath))
}

// writeHelperConfig writes a docker config.json with only credHelpers/credsStore.
func writeHelperConfig(t *testing.T, fs vfs.FS, path string, credHelpers map[string]string, credsStore string) {
	t.Helper()

	writeDockerConfig(t, fs, path, nil, credHelpers, credsStore)
}

// writeDockerConfig writes a docker config.json with inline auths (registry ->
// "user:pass"), credHelpers, and credsStore, omitting empty sections.
func writeDockerConfig(
	t *testing.T,
	fs vfs.FS,
	path string,
	inlineAuths, credHelpers map[string]string,
	credsStore string,
) {
	t.Helper()

	config := map[string]any{}

	if len(inlineAuths) > 0 {
		auths := map[string]any{}
		for registry, userpass := range inlineAuths {
			auths[registry] = map[string]string{"auth": base64.StdEncoding.EncodeToString([]byte(userpass))}
		}

		config["auths"] = auths
	}

	if len(credHelpers) > 0 {
		config["credHelpers"] = credHelpers
	}

	if credsStore != "" {
		config["credsStore"] = credsStore
	}

	data, err := json.Marshal(config)
	require.NoError(t, err)
	require.NoError(t, vfs.WriteFile(fs, path, data, 0o600))
}

// credentialForErr resolves the credential the store would send, returning any error.
func credentialForErr(t *testing.T, store getter.OCIRepositoryStore, registry string) (auth.Credential, error) {
	t.Helper()

	remoteStore, castOK := store.(getter.OCIRemoteStore)
	require.True(t, castOK, "default store must be the oras-backed remote store")

	client, castOK := remoteStore.Repo.Client.(*auth.Client)
	require.True(t, castOK, "default store must use an oras auth client")

	return client.Credential(t.Context(), registry)
}
