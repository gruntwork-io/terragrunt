package getter_test

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
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

func TestOCIAmbientCredentialPrecedence(t *testing.T) {
	t.Parallel()

	const (
		xdgRuntimeAuth      = "xdg-runtime-containers"
		homeConfigAuth      = "home-config-containers"
		xdgConfigHomeAuth   = "xdg-config-home-containers"
		dockerConfigAuth    = "docker-config"
		dockerConfigEnvAuth = "docker-config-env"
	)

	testCases := []struct {
		name             string
		wantUser         string
		files            []string
		setXDGConfigDir  bool
		setDockerConfig  bool
		setXDGRuntimeDir bool
	}{
		{
			name:             "xdg runtime containers auth wins over everything",
			files:            []string{xdgRuntimeAuth, homeConfigAuth, xdgConfigHomeAuth, dockerConfigAuth},
			setXDGRuntimeDir: true,
			setXDGConfigDir:  true,
			wantUser:         xdgRuntimeAuth,
		},
		{
			name:            "xdg config home wins over docker config",
			files:           []string{homeConfigAuth, xdgConfigHomeAuth, dockerConfigAuth},
			setXDGConfigDir: true,
			wantUser:        xdgConfigHomeAuth,
		},
		{
			name:            "set xdg config home replaces home config",
			files:           []string{homeConfigAuth, dockerConfigAuth},
			setXDGConfigDir: true,
			wantUser:        dockerConfigAuth,
		},
		{
			name:     "home config used when xdg config home is unset",
			files:    []string{homeConfigAuth, dockerConfigAuth},
			wantUser: homeConfigAuth,
		},
		{
			name:     "docker config used when no containers auth exists",
			files:    []string{dockerConfigAuth},
			wantUser: dockerConfigAuth,
		},
		{
			name:            "set docker config replaces home docker config",
			files:           []string{dockerConfigAuth, dockerConfigEnvAuth},
			setDockerConfig: true,
			wantUser:        dockerConfigEnvAuth,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			home := t.TempDir()
			runtimeDir := t.TempDir()
			configHome := t.TempDir()
			dockerConfigDir := t.TempDir()

			env := map[string]string{}
			if tc.setXDGRuntimeDir {
				env["XDG_RUNTIME_DIR"] = runtimeDir
			}

			if tc.setXDGConfigDir {
				env["XDG_CONFIG_HOME"] = configHome
			}

			if tc.setDockerConfig {
				env["DOCKER_CONFIG"] = dockerConfigDir
			}

			authFilePaths := map[string]string{
				xdgRuntimeAuth:      filepath.Join(runtimeDir, "containers", "auth.json"),
				homeConfigAuth:      filepath.Join(home, ".config", "containers", "auth.json"),
				xdgConfigHomeAuth:   filepath.Join(configHome, "containers", "auth.json"),
				dockerConfigAuth:    filepath.Join(home, ".docker", "config.json"),
				dockerConfigEnvAuth: filepath.Join(dockerConfigDir, "config.json"),
			}

			// Each file carries its own id as the username, so the resolved
			// username identifies which file won.
			for _, id := range tc.files {
				writeAuthFile(t, authFilePaths[id], testRegistry, id, "password-"+id)
			}

			newStore := getter.NewOCIRepositoryStore(logger.CreateLogger(), credentialVenv(home, env))

			store, err := newStore(t.Context(), testRegistry, "modules/vpc")
			require.NoError(t, err)

			cred := credentialFor(t, store, testRegistry)
			assert.Equal(t, tc.wantUser, cred.Username)
		})
	}
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

func TestOCIAmbientCredentialInvalidFileSkipped(t *testing.T) {
	t.Parallel()

	home := t.TempDir()

	// An unparseable file is skipped so it never breaks anonymous pulls or
	// higher-priority credentials.
	badPath := filepath.Join(home, ".docker", "config.json")
	require.NoError(t, os.MkdirAll(filepath.Dir(badPath), 0o755))
	require.NoError(t, os.WriteFile(badPath, []byte("not json"), 0o600))

	newStore := getter.NewOCIRepositoryStore(logger.CreateLogger(), credentialVenv(home, nil))

	store, err := newStore(t.Context(), testRegistry, "modules/vpc")
	require.NoError(t, err)
	assert.Equal(t, auth.EmptyCredential, credentialFor(t, store, testRegistry))

	// A valid higher-priority file still provides credentials.
	configHome := t.TempDir()
	writeAuthFile(t, filepath.Join(configHome, "containers", "auth.json"), testRegistry, "good-user", "good-pass")

	newStore = getter.NewOCIRepositoryStore(
		logger.CreateLogger(),
		credentialVenv(home, map[string]string{"XDG_CONFIG_HOME": configHome}),
	)

	store, err = newStore(t.Context(), testRegistry, "modules/vpc")
	require.NoError(t, err)
	assert.Equal(t, "good-user", credentialFor(t, store, testRegistry).Username)
}

// credentialVenv builds a hermetic Venv for credential discovery: an
// OS-backed filesystem (oras parses real files), HOME pointing at home, and
// extra env entries layered on top, so host credentials never leak in and
// tests stay parallel.
func credentialVenv(home string, extra map[string]string) venv.Venv {
	env := map[string]string{"HOME": home}
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

	cred, err := client.Credential(context.Background(), registry)
	require.NoError(t, err)

	return cred
}

// writeAuthFile writes a config.json-format credential file granting
// user/pass for registry.
func writeAuthFile(t *testing.T, path, registry, user, pass string) {
	t.Helper()

	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))

	encoded := base64.StdEncoding.EncodeToString([]byte(user + ":" + pass))
	content := map[string]any{
		"auths": map[string]any{
			registry: map[string]string{"auth": encoded},
		},
	}

	data, err := json.Marshal(content)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(path, data, 0o600))
}
