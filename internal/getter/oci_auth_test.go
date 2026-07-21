package getter_test

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"path/filepath"
	"strings"
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
		body := `{"ServerURL":"` + testRegistry + `","Username":"AWS","Secret":"fake-secret-minted"}`

		return vexec.Result{Stdout: []byte(body)}
	}, nil)

	home := testHome
	v := credentialVenv(home, nil).WithExec(exec)
	writeHelperConfig(t, v.FS, filepath.Join(home, ".docker", "config.json"),
		map[string]string{testRegistry: "ecr-login"}, "")

	newStore := getter.NewOCIRepositoryStore(logger.CreateLogger(), v)

	store, err := newStore(t.Context(), testRegistry, "modules/vpc")
	require.NoError(t, err)

	want := auth.Credential{Username: "AWS", Password: "fake-secret-minted"}
	assert.Equal(t, want, credentialFor(t, store, testRegistry))
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
		return vexec.Result{Stdout: []byte(`{"Username":"store-user","Secret":"fake-secret-store"}`)}
	}, nil)

	home := testHome
	v := credentialVenv(home, nil).WithExec(exec)
	writeHelperConfig(t, v.FS, filepath.Join(home, ".docker", "config.json"), nil, "osxkeychain")

	newStore := getter.NewOCIRepositoryStore(logger.CreateLogger(), v)

	store, err := newStore(t.Context(), testRegistry, "modules/vpc")
	require.NoError(t, err)

	want := auth.Credential{Username: "store-user", Password: "fake-secret-store"}
	assert.Equal(t, want, credentialFor(t, store, testRegistry))
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
		return vexec.Result{Stdout: []byte(`{"Username":"AWS","Secret":"fake-secret-pass"}`)}
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

// TestOCIHelperCredentialHelperBeatsInline: a configured helper wins over a
// stale inline login for the same registry, matching Docker precedence.
// TestOCIHelperCredentialRepoInlineBeatsHelper: a repo-scoped inline auth outranks a domain helper, which never runs.
func TestOCIHelperCredentialRepoInlineBeatsHelper(t *testing.T) {
	t.Parallel()

	var calls atomic.Int32

	exec := stubHelperExec(t, "ecr-login", func(string) vexec.Result {
		return vexec.Result{Stdout: []byte(`{"Username":"AWS","Secret":"fake-secret-fresh"}`)}
	}, &calls)

	home := testHome
	v := credentialVenv(home, nil).WithExec(exec)
	path := filepath.Join(home, ".docker", "config.json")
	// A repository-scoped inline login alongside a domain helper for the registry.
	writeDockerConfig(t, v.FS, path,
		map[string]string{testRegistry + "/modules/vpc": "repo-user:fake-secret-repo"},
		map[string]string{testRegistry: "ecr-login"}, "")

	newStore := getter.NewOCIRepositoryStore(logger.CreateLogger(), v)

	store, err := newStore(t.Context(), testRegistry, "modules/vpc")
	require.NoError(t, err)

	want := auth.Credential{Username: "repo-user", Password: "fake-secret-repo"}
	assert.Equal(t, want, credentialFor(t, store, testRegistry))
	assert.EqualValues(t, 0, calls.Load(), "the more specific inline auth must win and the helper must not run")
}

// TestOCIHelperCredentialDomainHelperBeatsDomainInline: a per-registry helper outranks a stale domain inline login.
func TestOCIHelperCredentialDomainHelperBeatsDomainInline(t *testing.T) {
	t.Parallel()

	var calls atomic.Int32

	exec := stubHelperExec(t, "ecr-login", func(string) vexec.Result {
		return vexec.Result{Stdout: []byte(`{"Username":"AWS","Secret":"fake-secret-fresh"}`)}
	}, &calls)

	home := testHome
	v := credentialVenv(home, nil).WithExec(exec)
	path := filepath.Join(home, ".docker", "config.json")
	// A stale domain-level inline login alongside a domain helper for the same registry.
	writeDockerConfig(t, v.FS, path,
		map[string]string{testRegistry: "stale-user:fake-secret-stale"},
		map[string]string{testRegistry: "ecr-login"}, "")

	newStore := getter.NewOCIRepositoryStore(logger.CreateLogger(), v)

	store, err := newStore(t.Context(), testRegistry, "modules/vpc")
	require.NoError(t, err)

	want := auth.Credential{Username: "AWS", Password: "fake-secret-fresh"}
	assert.Equal(t, want, credentialFor(t, store, testRegistry))
	assert.EqualValues(t, 1, calls.Load(), "the per-registry helper must win the domain tie and remint")
}

// TestOCIHelperCredentialRepoInlineBeatsCredsStore: a repo inline auth outranks credsStore, which never runs.
func TestOCIHelperCredentialRepoInlineBeatsCredsStore(t *testing.T) {
	t.Parallel()

	var calls atomic.Int32

	exec := stubHelperExec(t, "osxkeychain", func(string) vexec.Result {
		return vexec.Result{Stdout: []byte(`{"Username":"store","Secret":"fake-secret-store"}`)}
	}, &calls)

	home := testHome
	v := credentialVenv(home, nil).WithExec(exec)
	writeDockerConfig(t, v.FS, filepath.Join(home, ".docker", "config.json"),
		map[string]string{testRegistry + "/modules/vpc": "repo-user:fake-secret-repo"},
		nil, "osxkeychain")

	newStore := getter.NewOCIRepositoryStore(logger.CreateLogger(), v)

	store, err := newStore(t.Context(), testRegistry, "modules/vpc")
	require.NoError(t, err)

	want := auth.Credential{Username: "repo-user", Password: "fake-secret-repo"}
	assert.Equal(t, want, credentialFor(t, store, testRegistry))
	assert.EqualValues(t, 0, calls.Load(), "the more specific inline auth must win and credsStore must not run")
}

// TestOCIHelperCredentialDomainHelperBeatsEarlierCredsStore: a later domain helper outranks an earlier credsStore.
func TestOCIHelperCredentialDomainHelperBeatsEarlierCredsStore(t *testing.T) {
	t.Parallel()

	var storeCalls, helperCalls atomic.Int32

	handler := func(_ context.Context, inv vexec.Invocation) vexec.Result {
		switch inv.Name {
		case "docker-credential-ecr-login":
			helperCalls.Add(1)

			return vexec.Result{Stdout: []byte(`{"Username":"registry","Secret":"fake-secret-registry"}`)}
		case "docker-credential-desktop":
			storeCalls.Add(1)

			return vexec.Result{Stdout: []byte(`{"Username":"store","Secret":"fake-secret-store"}`)}
		default:
			assert.Fail(t, "an unexpected credential helper was invoked", inv.Name)

			return vexec.Result{ExitCode: 1}
		}
	}
	exec := vexec.NewMemExec(handler, vexec.WithLookPath(func(file string) (string, error) {
		return "/usr/local/bin/" + file, nil
	}))

	home := testHome
	xdgConfig := testXDGConfig
	v := credentialVenv(home, map[string]string{"XDG_CONFIG_HOME": xdgConfig}).WithExec(exec)
	// Earlier file (XDG) carries only a global credsStore; the later file a domain helper.
	writeHelperConfig(t, v.FS, filepath.Join(xdgConfig, "containers", "auth.json"), nil, "desktop")
	writeHelperConfig(t, v.FS, filepath.Join(home, ".docker", "config.json"),
		map[string]string{testRegistry: "ecr-login"}, "")

	newStore := getter.NewOCIRepositoryStore(logger.CreateLogger(), v)

	store, err := newStore(t.Context(), testRegistry, "modules/vpc")
	require.NoError(t, err)

	want := auth.Credential{Username: "registry", Password: "fake-secret-registry"}
	assert.Equal(t, want, credentialFor(t, store, testRegistry))
	assert.EqualValues(t, 1, helperCalls.Load(), "the domain helper must win over the earlier global credsStore")
	assert.EqualValues(t, 0, storeCalls.Load(), "the lower-specificity credsStore must not run")
}

// TestOCIHelperCredentialCredHelpersBeatsCredsStore: a per-registry helper wins over credsStore.
func TestOCIHelperCredentialCredHelpersBeatsCredsStore(t *testing.T) {
	t.Parallel()

	var registryCalls, storeCalls atomic.Int32

	handler := func(_ context.Context, inv vexec.Invocation) vexec.Result {
		switch inv.Name {
		case "docker-credential-ecr-login":
			registryCalls.Add(1)

			return vexec.Result{Stdout: []byte(`{"Username":"registry","Secret":"fake-secret-registry"}`)}
		case "docker-credential-osxkeychain":
			storeCalls.Add(1)

			return vexec.Result{Stdout: []byte(`{"Username":"store","Secret":"fake-secret-store"}`)}
		default:
			assert.Fail(t, "an unexpected credential helper was invoked", inv.Name)

			return vexec.Result{ExitCode: 1}
		}
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

	want := auth.Credential{Username: "registry", Password: "fake-secret-registry"}
	assert.Equal(t, want, credentialFor(t, store, testRegistry))
	assert.EqualValues(t, 1, registryCalls.Load(), "the per-registry ecr-login helper must be invoked once")
	assert.EqualValues(t, 0, storeCalls.Load(), "the per-registry helper must win over credsStore")
}

// TestOCIHelperCredentialEmptyCredHelperFallsThrough: an empty credHelpers value is skipped, never run as a helper.
func TestOCIHelperCredentialEmptyCredHelperFallsThrough(t *testing.T) {
	t.Parallel()

	handler := func(_ context.Context, inv vexec.Invocation) vexec.Result {
		switch inv.Name {
		case "docker-credential-osxkeychain":
			return vexec.Result{Stdout: []byte(`{"Username":"store","Secret":"fake-secret-store"}`)}
		default:
			assert.Fail(t, "only the credsStore helper may run for an empty credHelpers value", inv.Name)

			return vexec.Result{ExitCode: 1}
		}
	}
	exec := vexec.NewMemExec(handler, vexec.WithLookPath(func(file string) (string, error) {
		assert.NotEqual(t, "docker-credential-", file, "an empty helper name must never be looked up")

		return "/usr/local/bin/" + file, nil
	}))

	home := testHome
	v := credentialVenv(home, nil).WithExec(exec)
	writeHelperConfig(t, v.FS, filepath.Join(home, ".docker", "config.json"),
		map[string]string{testRegistry: ""}, "osxkeychain")

	newStore := getter.NewOCIRepositoryStore(logger.CreateLogger(), v)

	store, err := newStore(t.Context(), testRegistry, "modules/vpc")
	require.NoError(t, err)

	want := auth.Credential{Username: "store", Password: "fake-secret-store"}
	assert.Equal(t, want, credentialFor(t, store, testRegistry))
}

// TestOCIHelperCredentialCredsStoreFailureIsNonFatal: a global credsStore whose
// helper is missing must not break unrelated pulls (a Docker Desktop config on a
// box without the helper still resolves anonymously and via inline auths).
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
	// Only a global credsStore whose helper binary is missing; no inline auth or credHelpers.
	writeHelperConfig(t, v.FS, filepath.Join(home, ".docker", "config.json"), nil, "desktop")

	newStore := getter.NewOCIRepositoryStore(logger.CreateLogger(), v)

	store, err := newStore(t.Context(), testRegistry, "modules/vpc")
	require.NoError(t, err)

	// The broken credsStore is skipped and the pull resolves anonymously.
	cred, credErr := credentialForErr(t, store, testRegistry)
	require.NoError(t, credErr, "a missing credsStore helper must not fail the fetch")
	assert.Equal(t, auth.EmptyCredential, cred)
}

// TestOCIHelperCredentialScopedStaticFallsThroughToHelper: a scoped static
// credential still lets other registries resolve via their helper.
func TestOCIHelperCredentialScopedStaticFallsThroughToHelper(t *testing.T) {
	t.Parallel()

	const helperRegistry = "other.example.com"

	exec := stubHelperExec(t, "ecr-login", func(string) vexec.Result {
		return vexec.Result{Stdout: []byte(`{"Username":"AWS","Secret":"fake-secret-minted-token"}`)}
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

	wantHelper := auth.Credential{Username: "AWS", Password: "fake-secret-minted-token"}
	assert.Equal(t, wantHelper, credentialFor(t, other, helperRegistry))
}

// TestOCIHelperCredentialFirstFileHelperWins: with two config files, the
// higher-precedence file's helper resolves first.
func TestOCIHelperCredentialFirstFileHelperWins(t *testing.T) {
	t.Parallel()

	exec := vexec.NewMemExec(func(_ context.Context, inv vexec.Invocation) vexec.Result {
		if inv.Name == "docker-credential-first" {
			return vexec.Result{Stdout: []byte(`{"Username":"first","Secret":"fake-secret-first"}`)}
		}

		return vexec.Result{Stdout: []byte(`{"Username":"second","Secret":"fake-secret-second"}`)}
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

	want := auth.Credential{Username: "first", Password: "fake-secret-first"}
	assert.Equal(t, want, credentialFor(t, store, testRegistry))
}

// TestOCIHelperCredentialDockerHubSpellingCanonicalized: a docker.io spelling in
// credHelpers serves a registry-1.docker.io lookup, and the helper receives the
// declared index-server address it stored the credentials under.
func TestOCIHelperCredentialDockerHubSpellingCanonicalized(t *testing.T) {
	t.Parallel()

	var stdin string

	exec := stubHelperExec(t, "desktop", func(in string) vexec.Result {
		stdin = in
		return vexec.Result{Stdout: []byte(`{"Username":"hub","Secret":"fake-secret-hub"}`)}
	}, nil)

	home := testHome
	v := credentialVenv(home, nil).WithExec(exec)
	writeHelperConfig(t, v.FS, filepath.Join(home, ".docker", "config.json"),
		map[string]string{"https://index.docker.io/v1/": "desktop"}, "")

	newStore := getter.NewOCIRepositoryStore(logger.CreateLogger(), v)

	store, err := newStore(t.Context(), "registry-1.docker.io", "library/alpine")
	require.NoError(t, err)

	want := auth.Credential{Username: "hub", Password: "fake-secret-hub"}
	assert.Equal(t, want, credentialFor(t, store, "registry-1.docker.io"))
	assert.Equal(t, "https://index.docker.io/v1/", stdin, "the helper must receive the declared index-server address")
}

// TestOCIHelperCredentialReceivesVenvEnv: the helper must run under v.Env so
// process-scoped credentials (e.g. an assumed AWS role) reach ecr-login.
func TestOCIHelperCredentialReceivesVenvEnv(t *testing.T) {
	t.Parallel()

	var gotEnv []string

	exec := vexec.NewMemExec(func(_ context.Context, inv vexec.Invocation) vexec.Result {
		gotEnv = inv.Env
		return vexec.Result{Stdout: []byte(`{"Username":"AWS","Secret":"fake-secret-hub"}`)}
	}, vexec.WithLookPath(func(file string) (string, error) { return "/usr/local/bin/" + file, nil }))

	home := testHome
	// v.Env carries assumed-role AWS variables, as run.go populates before download.
	env := map[string]string{
		"AWS_ACCESS_KEY_ID":     "fake-access-key-id",
		"AWS_SECRET_ACCESS_KEY": "fake-secret-access-key",
		"AWS_SESSION_TOKEN":     "fake-session-token",
	}
	v := credentialVenv(home, env).WithExec(exec)
	writeHelperConfig(t, v.FS, filepath.Join(home, ".docker", "config.json"),
		map[string]string{testRegistry: "ecr-login"}, "")

	newStore := getter.NewOCIRepositoryStore(logger.CreateLogger(), v)

	store, err := newStore(t.Context(), testRegistry, "modules/vpc")
	require.NoError(t, err)

	credentialFor(t, store, testRegistry)

	assert.Contains(t, gotEnv, "AWS_ACCESS_KEY_ID=fake-access-key-id", "the helper must receive the run's AWS env")
	assert.Contains(t, gotEnv, "AWS_SESSION_TOKEN=fake-session-token")
}

// TestOCIHelperCredentialEmptyEnvStaysNonNil: an empty v.Env yields a non-nil helper env, not host inheritance.
func TestOCIHelperCredentialEmptyEnvStaysNonNil(t *testing.T) {
	t.Parallel()

	var gotEnv []string

	exec := vexec.NewMemExec(func(_ context.Context, inv vexec.Invocation) vexec.Result {
		gotEnv = inv.Env
		return vexec.Result{Stdout: []byte(`{"Username":"AWS","Secret":"fake-secret-hub"}`)}
	}, vexec.WithLookPath(func(file string) (string, error) { return "/usr/local/bin/" + file, nil }))

	v := credentialVenv(testHome, nil).WithExec(exec)
	writeHelperConfig(t, v.FS, filepath.Join(testHome, ".docker", "config.json"),
		map[string]string{testRegistry: "ecr-login"}, "")

	newStore := getter.NewOCIRepositoryStore(logger.CreateLogger(), v)

	store, err := newStore(t.Context(), testRegistry, "modules/vpc")
	require.NoError(t, err)

	credentialFor(t, store, testRegistry)

	require.NotNil(t, gotEnv, "an empty v.Env must produce a non-nil helper env, not host inheritance")
	assert.Empty(t, gotEnv, "an empty v.Env must not leak host variables to the helper")
}

// TestOCIHelperCredentialSurfacesStderr: a failing helper surfaces its stderr diagnostic without leaking stdout.
func TestOCIHelperCredentialSurfacesStderr(t *testing.T) {
	t.Parallel()

	exec := stubHelperExec(t, "ecr-login", func(string) vexec.Result {
		return vexec.Result{
			Stdout:   []byte("secret-on-stdout"),
			Stderr:   []byte("could not refresh ECR token: expired"),
			ExitCode: 1,
		}
	}, nil)

	v := credentialVenv(testHome, nil).WithExec(exec)
	writeHelperConfig(t, v.FS, filepath.Join(testHome, ".docker", "config.json"),
		map[string]string{testRegistry: "ecr-login"}, "")

	newStore := getter.NewOCIRepositoryStore(logger.CreateLogger(), v)

	store, err := newStore(t.Context(), testRegistry, "modules/vpc")
	require.NoError(t, err)

	_, err = credentialForErr(t, store, testRegistry)
	require.Error(t, err)

	var helperErr getter.OCICredentialHelperError
	require.ErrorAs(t, err, &helperErr)
	assert.Contains(t, helperErr.Stderr, "could not refresh ECR token: expired", "the helper stderr must be captured")
	assert.Contains(t, err.Error(), "could not refresh ECR token: expired", "stderr must surface in the error")
	assert.NotContains(t, err.Error(), "secret-on-stdout", "the helper stdout must never leak into the error")
}

// TestOCIHelperCredentialStderrTruncated: oversized helper stderr is capped with a trailing ellipsis marker.
func TestOCIHelperCredentialStderrTruncated(t *testing.T) {
	t.Parallel()

	huge := strings.Repeat("x", 5000)
	exec := stubHelperExec(t, "ecr-login", func(string) vexec.Result {
		return vexec.Result{Stdout: []byte("nope"), Stderr: []byte(huge), ExitCode: 1}
	}, nil)

	v := credentialVenv(testHome, nil).WithExec(exec)
	writeHelperConfig(t, v.FS, filepath.Join(testHome, ".docker", "config.json"),
		map[string]string{testRegistry: "ecr-login"}, "")

	newStore := getter.NewOCIRepositoryStore(logger.CreateLogger(), v)

	store, err := newStore(t.Context(), testRegistry, "modules/vpc")
	require.NoError(t, err)

	_, err = credentialForErr(t, store, testRegistry)
	require.Error(t, err)

	var helperErr getter.OCICredentialHelperError
	require.ErrorAs(t, err, &helperErr)
	assert.True(t, strings.HasSuffix(helperErr.Stderr, "..."), "capped stderr must carry a truncation marker")
	assert.Less(t, len(helperErr.Stderr), len(huge), "stderr must be capped below the raw size")
}

// TestOCIStaticCredentialsScopedRegistryCanonicalized: a non-canonical scoped
// registry env value (scheme prefix) still matches the requested host.
func TestOCIStaticCredentialsScopedRegistryCanonicalized(t *testing.T) {
	t.Parallel()

	env := map[string]string{
		getter.EnvOCIToken:    "scoped-token",
		getter.EnvOCIRegistry: "https://ghcr.io",
	}

	newStore := getter.NewOCIRepositoryStore(logger.CreateLogger(), credentialVenv(testHome, env))

	store, err := newStore(t.Context(), "ghcr.io", "acme/vpc")
	require.NoError(t, err)
	assert.Equal(t, auth.Credential{AccessToken: "scoped-token"}, credentialFor(t, store, "ghcr.io"))
}

// TestOCIStaticCredentialsUnscopedReachesEveryHost: unscoped static credentials
// are offered to every registry, matching the documented interim behavior.
func TestOCIStaticCredentialsUnscopedReachesEveryHost(t *testing.T) {
	t.Parallel()

	env := map[string]string{getter.EnvOCIToken: "unscoped-token"}

	newStore := getter.NewOCIRepositoryStore(logger.CreateLogger(), credentialVenv(testHome, env))

	for _, host := range []string{"registry.example.com", "other.example.com", "ghcr.io"} {
		store, err := newStore(t.Context(), host, "modules/vpc")
		require.NoError(t, err)
		assert.Equal(t, auth.Credential{AccessToken: "unscoped-token"}, credentialFor(t, store, host))
	}
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
