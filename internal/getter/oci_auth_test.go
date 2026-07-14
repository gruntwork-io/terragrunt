//nolint:paralleltest,tparallel // Every test here uses t.Setenv and therefore can't run in parallel.
package getter_test

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/getter"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"oras.land/oras-go/v2/registry/remote"
	"oras.land/oras-go/v2/registry/remote/auth"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testRegistry = "registry.example.com"

func TestNewOCIRepositoryStoreReference(t *testing.T) {
	setHermeticCredentialEnv(t, t.TempDir())

	store, err := getter.NewOCIRepositoryStore(vfs.NewOSFS())(t.Context(), "127.0.0.1:5000", "org/team/vpc")
	require.NoError(t, err)

	repo, castOK := store.(*remote.Repository)
	require.True(t, castOK, "default store must be an oras remote repository")
	assert.Equal(t, "127.0.0.1:5000", repo.Reference.Registry)
	assert.Equal(t, "org/team/vpc", repo.Reference.Repository)
}

func TestOCIStaticCredentials(t *testing.T) {
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
			home := t.TempDir()
			setHermeticCredentialEnv(t, home)
			// Ambient file present to prove static credentials win over it.
			writeAuthFile(t, filepath.Join(home, ".docker", "config.json"), testRegistry, "ambient-user", "ambient-pass")

			for name, value := range tc.env {
				t.Setenv(name, value)
			}

			store, err := getter.NewOCIRepositoryStore(vfs.NewOSFS())(t.Context(), testRegistry, "modules/vpc")

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
	const (
		xdgRuntimeAuth    = "xdg-runtime-containers"
		homeConfigAuth    = "home-config-containers"
		xdgConfigHomeAuth = "xdg-config-home-containers"
		dockerConfigAuth  = "docker-config"
	)

	testCases := []struct {
		name              string
		wantUser          string
		files             []string
		unsetXDGConfigDir bool
	}{
		{
			name:     "xdg runtime containers auth wins over everything",
			files:    []string{xdgRuntimeAuth, homeConfigAuth, xdgConfigHomeAuth, dockerConfigAuth},
			wantUser: xdgRuntimeAuth,
		},
		{
			name:     "xdg config home wins over docker config",
			files:    []string{homeConfigAuth, xdgConfigHomeAuth, dockerConfigAuth},
			wantUser: xdgConfigHomeAuth,
		},
		{
			name:     "set xdg config home replaces home config",
			files:    []string{homeConfigAuth, dockerConfigAuth},
			wantUser: dockerConfigAuth,
		},
		{
			name:              "home config used when xdg config home is unset",
			files:             []string{homeConfigAuth, dockerConfigAuth},
			unsetXDGConfigDir: true,
			wantUser:          homeConfigAuth,
		},
		{
			name:              "docker config used when no containers auth exists",
			files:             []string{dockerConfigAuth},
			unsetXDGConfigDir: true,
			wantUser:          dockerConfigAuth,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			home := t.TempDir()
			runtimeDir := t.TempDir()
			configHome := t.TempDir()

			setHermeticCredentialEnv(t, home)
			t.Setenv("XDG_RUNTIME_DIR", runtimeDir)
			t.Setenv("XDG_CONFIG_HOME", configHome)

			if tc.unsetXDGConfigDir {
				t.Setenv("XDG_CONFIG_HOME", "")
			}

			authFilePaths := map[string]string{
				xdgRuntimeAuth:    filepath.Join(runtimeDir, "containers", "auth.json"),
				homeConfigAuth:    filepath.Join(home, ".config", "containers", "auth.json"),
				xdgConfigHomeAuth: filepath.Join(configHome, "containers", "auth.json"),
				dockerConfigAuth:  filepath.Join(home, ".docker", "config.json"),
			}

			// Each file carries its own id as the username, so the resolved
			// username identifies which file won.
			for _, id := range tc.files {
				writeAuthFile(t, authFilePaths[id], testRegistry, id, "password-"+id)
			}

			store, err := getter.NewOCIRepositoryStore(vfs.NewOSFS())(t.Context(), testRegistry, "modules/vpc")
			require.NoError(t, err)

			cred := credentialFor(t, store, testRegistry)
			assert.Equal(t, tc.wantUser, cred.Username)
		})
	}
}

func TestOCIAmbientCredentialStatErrorSurfaces(t *testing.T) {
	home := t.TempDir()
	setHermeticCredentialEnv(t, home)

	// An unreadable parent directory makes Stat fail with a permission
	// error, which must surface instead of silently selecting weaker
	// credentials.
	dockerDir := filepath.Join(home, ".docker")
	require.NoError(t, os.MkdirAll(dockerDir, 0o755))
	writeAuthFile(t, filepath.Join(dockerDir, "config.json"), testRegistry, "blocked-user", "blocked-pass")
	require.NoError(t, os.Chmod(dockerDir, 0o000))

	t.Cleanup(func() {
		if err := os.Chmod(dockerDir, 0o755); err != nil {
			t.Logf("restoring permissions on %s: %v", dockerDir, err)
		}
	})

	_, err := getter.NewOCIRepositoryStore(vfs.NewOSFS())(t.Context(), testRegistry, "modules/vpc")
	require.Error(t, err)

	var fileErr getter.OCICredentialFileError

	require.ErrorAs(t, err, &fileErr)
	assert.Equal(t, filepath.Join(dockerDir, "config.json"), fileErr.Path)
}

func TestOCIAmbientCredentialScopedToRegistry(t *testing.T) {
	home := t.TempDir()
	setHermeticCredentialEnv(t, home)
	writeAuthFile(t, filepath.Join(home, ".docker", "config.json"), testRegistry, "scoped-user", "scoped-pass")

	store, err := getter.NewOCIRepositoryStore(vfs.NewOSFS())(t.Context(), "other-registry.example.com", "modules/vpc")
	require.NoError(t, err)

	assert.Equal(t, auth.EmptyCredential, credentialFor(t, store, "other-registry.example.com"))
}

func TestOCIAmbientCredentialNoFiles(t *testing.T) {
	setHermeticCredentialEnv(t, t.TempDir())

	store, err := getter.NewOCIRepositoryStore(vfs.NewOSFS())(t.Context(), testRegistry, "modules/vpc")
	require.NoError(t, err)

	assert.Equal(t, auth.EmptyCredential, credentialFor(t, store, testRegistry))
}

func TestOCIAmbientCredentialInvalidFile(t *testing.T) {
	home := t.TempDir()
	setHermeticCredentialEnv(t, home)

	path := filepath.Join(home, ".docker", "config.json")
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte("not json"), 0o600))

	_, err := getter.NewOCIRepositoryStore(vfs.NewOSFS())(t.Context(), testRegistry, "modules/vpc")
	require.Error(t, err)

	var fileErr getter.OCICredentialFileError

	require.ErrorAs(t, err, &fileErr)
	assert.Equal(t, path, fileErr.Path)
}

// setHermeticCredentialEnv points every environment variable the credential
// search reads at hermetic values so host credentials never leak into tests.
func setHermeticCredentialEnv(t *testing.T, home string) {
	t.Helper()

	t.Setenv("HOME", home)
	t.Setenv("XDG_RUNTIME_DIR", filepath.Join(home, "no-runtime"))
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "no-config-home"))
	t.Setenv(getter.EnvOCIUsername, "")
	t.Setenv(getter.EnvOCIPassword, "")
	t.Setenv(getter.EnvOCIToken, "")
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
