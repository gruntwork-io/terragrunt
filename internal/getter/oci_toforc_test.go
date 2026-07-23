package getter_test

import (
	"path/filepath"
	"sync/atomic"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/getter"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/internal/vexec"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"oras.land/oras-go/v2/registry/remote/auth"
)

// TestOCITofuCredentialsBasicAuth: an oci_credentials block supplies username/password.
func TestOCITofuCredentialsBasicAuth(t *testing.T) {
	t.Parallel()

	home := testHome
	v := credentialVenv(home, nil)
	writeTofuConfig(t, v.FS, filepath.Join(home, ".tofurc"), `
oci_credentials "registry.example.com/team" {
  username = "svc"
  password = "fake-secret-tofu"
}
`)

	store := newStoreForRepo(t, v, testRegistry, "team/vpc")
	want := auth.Credential{Username: "svc", Password: "fake-secret-tofu"}
	assert.Equal(t, want, credentialFor(t, store, testRegistry))
}

// TestOCITofuCredentialsOAuth: an oci_credentials block supplies OAuth tokens.
func TestOCITofuCredentialsOAuth(t *testing.T) {
	t.Parallel()

	home := testHome
	v := credentialVenv(home, nil)
	writeTofuConfig(t, v.FS, filepath.Join(home, ".tofurc"), `
oci_credentials "registry.example.com/team" {
  access_token  = "fake-access"
  refresh_token = "fake-refresh"
}
`)

	store := newStoreForRepo(t, v, testRegistry, "team/vpc")
	want := auth.Credential{AccessToken: "fake-access", RefreshToken: "fake-refresh"}
	assert.Equal(t, want, credentialFor(t, store, testRegistry))
}

// TestOCITofuCredentialsRepositoryHelper: a per-block docker_credentials_helper is dispatched.
func TestOCITofuCredentialsRepositoryHelper(t *testing.T) {
	t.Parallel()

	var stdin string

	exec := stubHelperExec(t, "ecr-login", func(in string) vexec.Result {
		stdin = in
		return vexec.Result{Stdout: []byte(`{"Username":"AWS","Secret":"fake-secret-minted"}`)}
	}, nil)

	home := testHome
	v := credentialVenv(home, nil).WithExec(exec)
	writeTofuConfig(t, v.FS, filepath.Join(home, ".tofurc"), `
oci_credentials "registry.example.com/team" {
  docker_credentials_helper = "ecr-login"
}
`)

	store := newStoreForRepo(t, v, testRegistry, "team/vpc")
	want := auth.Credential{Username: "AWS", Password: "fake-secret-minted"}
	assert.Equal(t, want, credentialFor(t, store, testRegistry))
	assert.Equal(t, testRegistry, stdin)
}

// TestOCITofuCredentialsDefaultHelper: oci_default_credentials supplies a fallback helper.
func TestOCITofuCredentialsDefaultHelper(t *testing.T) {
	t.Parallel()

	exec := stubHelperExec(t, "ecr-login", func(string) vexec.Result {
		return vexec.Result{Stdout: []byte(`{"Username":"AWS","Secret":"fake-secret-default"}`)}
	}, nil)

	home := testHome
	v := credentialVenv(home, nil).WithExec(exec)
	writeTofuConfig(t, v.FS, filepath.Join(home, ".tofurc"), `
oci_default_credentials {
  discover_ambient_credentials = true
  docker_credentials_helper    = "ecr-login"
}
`)

	store := newStoreForRepo(t, v, testRegistry, "team/vpc")
	want := auth.Credential{Username: "AWS", Password: "fake-secret-default"}
	assert.Equal(t, want, credentialFor(t, store, testRegistry))
}

// TestOCITofuCredentialsMostSpecificPrefixWins: the longest matching repository prefix wins.
func TestOCITofuCredentialsMostSpecificPrefixWins(t *testing.T) {
	t.Parallel()

	home := testHome
	v := credentialVenv(home, nil)
	writeTofuConfig(t, v.FS, filepath.Join(home, ".tofurc"), `
oci_credentials "registry.example.com" {
  username = "wide"
  password = "fake-secret-wide"
}
oci_credentials "registry.example.com/team/vpc" {
  username = "narrow"
  password = "fake-secret-narrow"
}
`)

	store := newStoreForRepo(t, v, testRegistry, "team/vpc")
	want := auth.Credential{Username: "narrow", Password: "fake-secret-narrow"}
	assert.Equal(t, want, credentialFor(t, store, testRegistry))
}

// TestOCITofuCredentialsPrefixBoundary: a prefix matches only on a path boundary.
func TestOCITofuCredentialsPrefixBoundary(t *testing.T) {
	t.Parallel()

	home := testHome
	v := credentialVenv(home, nil)
	writeTofuConfig(t, v.FS, filepath.Join(home, ".tofurc"), `
oci_credentials "registry.example.com/team" {
  username = "team"
  password = "fake-secret-team"
}
`)

	// "team-internal/vpc" must NOT match the "team" prefix.
	store := newStoreForRepo(t, v, testRegistry, "team-internal/vpc")
	assert.Equal(t, auth.EmptyCredential, credentialFor(t, store, testRegistry))
}

// TestOCITofuCredentialsBeatsAmbient: a tofu block wins over an ambient inline auth.
func TestOCITofuCredentialsBeatsAmbient(t *testing.T) {
	t.Parallel()

	home := testHome
	v := credentialVenv(home, nil)
	writeTofuConfig(t, v.FS, filepath.Join(home, ".tofurc"), `
oci_credentials "registry.example.com" {
  username = "tofu"
  password = "fake-secret-tofu"
}
`)
	writeAuthFile(t, v.FS, filepath.Join(home, ".docker", "config.json"), testRegistry, "ambient", "ambient-pass")

	store := newStoreForRepo(t, v, testRegistry, "team/vpc")
	want := auth.Credential{Username: "tofu", Password: "fake-secret-tofu"}
	assert.Equal(t, want, credentialFor(t, store, testRegistry))
}

// TestOCITofuCredentialsConfigFileOverride: TF_CLI_CONFIG_FILE selects the config path.
func TestOCITofuCredentialsConfigFileOverride(t *testing.T) {
	t.Parallel()

	home := testHome
	custom := "/virtual/custom.tofurc"
	v := credentialVenv(home, map[string]string{"TF_CLI_CONFIG_FILE": custom})
	writeTofuConfig(t, v.FS, custom, `
oci_credentials "registry.example.com" {
  username = "custom"
  password = "fake-secret-custom"
}
`)

	store := newStoreForRepo(t, v, testRegistry, "team/vpc")
	want := auth.Credential{Username: "custom", Password: "fake-secret-custom"}
	assert.Equal(t, want, credentialFor(t, store, testRegistry))
}

// TestOCITofuCredentialsMissingOrUnparsableIsEmpty: a missing or broken config yields no credentials, no error.
func TestOCITofuCredentialsMissingOrUnparsableIsEmpty(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name    string
		content string
		write   bool
	}{
		{name: "no config file", write: false},
		{name: "unparsable config", write: true, content: "oci_credentials {{{ not hcl"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			home := testHome
			v := credentialVenv(home, nil)

			if tc.write {
				writeTofuConfig(t, v.FS, filepath.Join(home, ".tofurc"), tc.content)
			}

			store := newStoreForRepo(t, v, testRegistry, "team/vpc")

			cred, err := credentialForErr(t, store, testRegistry)
			require.NoError(t, err)
			assert.Equal(t, auth.EmptyCredential, cred)
		})
	}
}

// TestOCITofuCredentialsDiscoverAmbientFalseSuppressesAmbient: an explicit
// discover_ambient_credentials=false stops ambient Docker credentials from
// being offered, matching OpenTofu.
func TestOCITofuCredentialsDiscoverAmbientFalseSuppressesAmbient(t *testing.T) {
	t.Parallel()

	home := testHome
	v := credentialVenv(home, nil)
	writeTofuConfig(t, v.FS, filepath.Join(home, ".tofurc"), `
oci_default_credentials {
  discover_ambient_credentials = false
}
`)
	writeAuthFile(t, v.FS, filepath.Join(home, ".docker", "config.json"), testRegistry, "ambient", "ambient-pass")

	store := newStoreForRepo(t, v, testRegistry, "team/vpc")
	assert.Equal(t, auth.EmptyCredential, credentialFor(t, store, testRegistry),
		"discover_ambient_credentials=false must suppress the ambient auth")
}

// TestOCITofuCredentialsDefaultHelperBelowAmbient: the default helper is a
// lower-priority fallback than ambient discovery, matching OpenTofu.
func TestOCITofuCredentialsDefaultHelperBelowAmbient(t *testing.T) {
	t.Parallel()

	var calls atomic.Int32

	exec := stubHelperExec(t, "ecr-login", func(string) vexec.Result {
		return vexec.Result{Stdout: []byte(`{"Username":"AWS","Secret":"fake-secret-default"}`)}
	}, &calls)

	home := testHome
	v := credentialVenv(home, nil).WithExec(exec)
	writeTofuConfig(t, v.FS, filepath.Join(home, ".tofurc"), `
oci_default_credentials {
  docker_credentials_helper = "ecr-login"
}
`)
	writeAuthFile(t, v.FS, filepath.Join(home, ".docker", "config.json"), testRegistry, "ambient", "ambient-pass")

	store := newStoreForRepo(t, v, testRegistry, "team/vpc")
	want := auth.Credential{Username: "ambient", Password: "ambient-pass"}
	assert.Equal(t, want, credentialFor(t, store, testRegistry))
	assert.EqualValues(t, 0, calls.Load(), "the default helper must not run when ambient resolves")
}

// TestOCITofuCredentialsUnknownAttributeTolerated: an unrecognized argument does
// not discard the known blocks, so a config for a newer tofu still loads.
func TestOCITofuCredentialsUnknownAttributeTolerated(t *testing.T) {
	t.Parallel()

	home := testHome
	v := credentialVenv(home, nil)
	writeTofuConfig(t, v.FS, filepath.Join(home, ".tofurc"), `
oci_credentials "registry.example.com" {
  username        = "svc"
  password        = "fake-secret-tofu"
  future_argument = "ignored"
}
`)

	store := newStoreForRepo(t, v, testRegistry, "team/vpc")
	want := auth.Credential{Username: "svc", Password: "fake-secret-tofu"}
	assert.Equal(t, want, credentialFor(t, store, testRegistry))
}

// TestOCITofuCredentialsInvalidHelperNameSkipped: a helper name with a path
// separator is rejected so it cannot execute a non-PATH binary.
func TestOCITofuCredentialsInvalidHelperNameSkipped(t *testing.T) {
	t.Parallel()

	home := testHome
	v := credentialVenv(home, nil)
	writeTofuConfig(t, v.FS, filepath.Join(home, ".tofurc"), `
oci_credentials "registry.example.com" {
  docker_credentials_helper = "../../../tmp/evil"
}
`)

	store := newStoreForRepo(t, v, testRegistry, "team/vpc")
	assert.Equal(t, auth.EmptyCredential, credentialFor(t, store, testRegistry))
}

// TestOCITofuCredentialsMultipleStylesSkipped: a block configuring more than one
// credential style is rejected, matching OpenTofu.
func TestOCITofuCredentialsMultipleStylesSkipped(t *testing.T) {
	t.Parallel()

	home := testHome
	v := credentialVenv(home, nil)
	writeTofuConfig(t, v.FS, filepath.Join(home, ".tofurc"), `
oci_credentials "registry.example.com" {
  username     = "svc"
  password     = "fake-secret-tofu"
  access_token = "fake-access"
}
`)

	store := newStoreForRepo(t, v, testRegistry, "team/vpc")
	assert.Equal(t, auth.EmptyCredential, credentialFor(t, store, testRegistry))
}

// TestOCITofuCredentialsConfigPathResolution: TERRAFORM_CONFIG and the
// .terraformrc fallback resolve, with .tofurc winning when both files exist.
func TestOCITofuCredentialsConfigPathResolution(t *testing.T) {
	t.Parallel()

	t.Run("TERRAFORM_CONFIG override", func(t *testing.T) {
		t.Parallel()

		custom := "/virtual/terraform.rc"
		v := credentialVenv(testHome, map[string]string{"TERRAFORM_CONFIG": custom})
		writeTofuConfig(t, v.FS, custom, `
oci_credentials "registry.example.com" {
  username = "tf-config"
  password = "fake-secret-tf"
}
`)

		store := newStoreForRepo(t, v, testRegistry, "team/vpc")
		assert.Equal(t, "tf-config", credentialFor(t, store, testRegistry).Username)
	})

	t.Run("terraformrc fallback and tofurc precedence", func(t *testing.T) {
		t.Parallel()

		home := testHome
		v := credentialVenv(home, nil)
		writeTofuConfig(t, v.FS, filepath.Join(home, ".terraformrc"), `
oci_credentials "registry.example.com" {
  username = "terraformrc"
  password = "fake-secret-rc"
}
`)

		// Only .terraformrc exists: it resolves.
		store := newStoreForRepo(t, v, testRegistry, "team/vpc")
		assert.Equal(t, "terraformrc", credentialFor(t, store, testRegistry).Username)

		// .tofurc is preferred when both exist.
		writeTofuConfig(t, v.FS, filepath.Join(home, ".tofurc"), `
oci_credentials "registry.example.com" {
  username = "tofurc"
  password = "fake-secret-tofurc"
}
`)

		store = newStoreForRepo(t, v, testRegistry, "team/vpc")
		assert.Equal(t, "tofurc", credentialFor(t, store, testRegistry).Username)
	})
}

// newStoreForRepo builds the default store for one registry/repository.
func newStoreForRepo(t *testing.T, v venv.Venv, registry, repositoryName string) getter.OCIRepositoryStore {
	t.Helper()

	newStore := getter.NewOCIRepositoryStore(logger.CreateLogger(), v)

	store, err := newStore(t.Context(), registry, repositoryName)
	require.NoError(t, err)

	return store
}

// writeTofuConfig writes an OpenTofu CLI config file with the given body.
func writeTofuConfig(t *testing.T, fs vfs.FS, path, body string) {
	t.Helper()

	require.NoError(t, vfs.WriteFile(fs, path, []byte(body), 0o600))
}
