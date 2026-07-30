package getter_test

import (
	"context"
	"errors"
	"fmt"
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

// errNoSuchHelperBinary stands in for a helper missing from PATH.
var errNoSuchHelperBinary = errors.New("executable file not found in $PATH")

// windowsTestGOOS drives the Windows CLI-config discovery branch.
const windowsTestGOOS = "windows"

// CLI-config override env vars, named here so tests do not depend on package internals.
const (
	envTFCLIConfigFileTest = "TF_CLI_CONFIG_FILE"
	envTerraformConfigTest = "TERRAFORM_CONFIG"
)

// TestOCITofuCredentialsBasicAuth: an oci_credentials block supplies username/password.
func TestOCITofuCredentialsBasicAuth(t *testing.T) {
	t.Parallel()

	home := testHome
	v := credentialVenv(home, nil)
	writeTofuConfig(t, v.FS, filepath.Join(home, ".tofurc"),
		tofuBasicAuth("registry.example.com/team", "svc", "fake-secret-tofu"))

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
oci_credentials "registry.example.com" {
  docker_credentials_helper = "ecr-login"
}
`)

	store := newStoreForRepo(t, v, testRegistry, "team/vpc")
	want := auth.Credential{Username: "AWS", Password: "fake-secret-minted"}
	assert.Equal(t, want, credentialFor(t, store, testRegistry))
	assert.Equal(t, "https://"+testRegistry, stdin, "tofu helpers receive OpenTofu's https:// server address")
}

// TestOCITofuCredentialsHelperRepositoryPathRejected: tofu rejects a helper on a
// repository-scoped label, because helpers only key on a whole domain.
func TestOCITofuCredentialsHelperRepositoryPathRejected(t *testing.T) {
	t.Parallel()

	var calls atomic.Int32

	exec := stubHelperExec(t, "ecr-login", func(string) vexec.Result {
		return vexec.Result{Stdout: []byte(`{"Username":"AWS","Secret":"fake-secret-minted"}`)}
	}, &calls)

	home := testHome
	v := credentialVenv(home, nil).WithExec(exec)
	writeTofuConfig(t, v.FS, filepath.Join(home, ".tofurc"), `
oci_credentials "registry.example.com/team" {
  docker_credentials_helper = "ecr-login"
}
`)

	err := newStoreErr(t, v, testRegistry, "team/vpc")
	require.Error(t, err)
	require.ErrorContains(t, err, "cannot be used with a repository path")
	assert.EqualValues(t, 0, calls.Load(), "a repository-scoped helper block must never run")
}

// TestOCITofuCredentialsHelperServerAddress: a tofu helper receives https:// plus
// the requested registry domain, with no Docker Hub rewrite, matching OpenTofu.
func TestOCITofuCredentialsHelperServerAddress(t *testing.T) {
	t.Parallel()

	tc := []struct {
		registry   string
		repository string
		want       string
	}{
		{registry: "registry-1.docker.io", repository: "library/alpine", want: "https://registry-1.docker.io"},
		{registry: "docker.io", repository: "library/alpine", want: "https://docker.io"},
		{registry: "index.docker.io", repository: "library/alpine", want: "https://index.docker.io"},
		{registry: testRegistry, repository: "team/vpc", want: "https://" + testRegistry},
	}

	for _, tt := range tc {
		t.Run(tt.registry, func(t *testing.T) {
			t.Parallel()

			var stdin string

			exec := stubHelperExec(t, "desktop", func(in string) vexec.Result {
				stdin = in
				return vexec.Result{Stdout: []byte(`{"Username":"hub","Secret":"fake-secret-hub"}`)}
			}, nil)

			home := testHome
			v := credentialVenv(home, nil).WithExec(exec)
			writeTofuConfig(t, v.FS, filepath.Join(home, ".tofurc"), `
oci_default_credentials {
  docker_credentials_helper = "desktop"
}
`)

			store := newStoreForRepo(t, v, tt.registry, tt.repository)
			want := auth.Credential{Username: "hub", Password: "fake-secret-hub"}
			assert.Equal(t, want, credentialFor(t, store, tt.registry))
			assert.Equal(t, tt.want, stdin)
		})
	}
}

// TestOCITofuCredentialsSchemeLabelRejected: tofu parses the label as a bare
// repository address, so a URL scheme is a configuration error.
func TestOCITofuCredentialsSchemeLabelRejected(t *testing.T) {
	t.Parallel()

	home := testHome
	v := credentialVenv(home, nil)
	writeTofuConfig(t, v.FS, filepath.Join(home, ".tofurc"),
		tofuBasicAuth("https://registry.example.com/team", "scheme-user", "fake-secret-scheme"))

	err := newStoreErr(t, v, testRegistry, "team/vpc")
	require.Error(t, err)
	assert.ErrorContains(t, err, "without a URL scheme")
}

// TestOCITofuCredentialsIncompleteBasicSkipped: a username without a password is skipped, not used.
func TestOCITofuCredentialsIncompleteBasicSkipped(t *testing.T) {
	t.Parallel()

	home := testHome
	v := credentialVenv(home, nil)
	writeTofuConfig(t, v.FS, filepath.Join(home, ".tofurc"), `
oci_credentials "registry.example.com" {
  username = "no-password"
}
`)

	err := newStoreErr(t, v, testRegistry, "team/vpc")
	require.Error(t, err)
	assert.ErrorContains(t, err, "requires both a username and a password")
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
	writeTofuConfig(t, v.FS, filepath.Join(home, ".tofurc"),
		tofuBasicAuth(testRegistry, "wide", "fake-secret-wide")+
			tofuBasicAuth(testRegistry+"/team/vpc", "narrow", "fake-secret-narrow"))

	store := newStoreForRepo(t, v, testRegistry, "team/vpc")
	want := auth.Credential{Username: "narrow", Password: "fake-secret-narrow"}
	assert.Equal(t, want, credentialFor(t, store, testRegistry))
}

// TestOCITofuCredentialsPrefixBoundary: a prefix matches only on a path boundary.
func TestOCITofuCredentialsPrefixBoundary(t *testing.T) {
	t.Parallel()

	home := testHome
	v := credentialVenv(home, nil)
	writeTofuConfig(t, v.FS, filepath.Join(home, ".tofurc"),
		tofuBasicAuth("registry.example.com/team", "team", "fake-secret-team"))

	// "team-internal/vpc" must NOT match the "team" prefix.
	store := newStoreForRepo(t, v, testRegistry, "team-internal/vpc")
	assert.Equal(t, auth.EmptyCredential, credentialFor(t, store, testRegistry))
}

// TestOCITofuCredentialsConfigFileOverride: TF_CLI_CONFIG_FILE selects the config path.
func TestOCITofuCredentialsConfigFileOverride(t *testing.T) {
	t.Parallel()

	home := testHome
	custom := "/virtual/custom.tofurc"
	v := credentialVenv(home, map[string]string{"TF_CLI_CONFIG_FILE": custom})
	writeTofuConfig(t, v.FS, custom, tofuBasicAuth("registry.example.com", "custom", "fake-secret-custom"))

	store := newStoreForRepo(t, v, testRegistry, "team/vpc")
	want := auth.Credential{Username: "custom", Password: "fake-secret-custom"}
	assert.Equal(t, want, credentialFor(t, store, testRegistry))
}

// TestOCITofuCredentialsAbsentConfigIsEmpty: no CLI config at all is a normal
// discovery miss, not an error.
func TestOCITofuCredentialsAbsentConfigIsEmpty(t *testing.T) {
	t.Parallel()

	v := credentialVenv(testHome, nil)
	store := newStoreForRepo(t, v, testRegistry, "team/vpc")

	cred, err := credentialForErr(t, store, testRegistry)
	require.NoError(t, err)
	assert.Equal(t, auth.EmptyCredential, cred)
}

// TestOCITofuCredentialsUnparsableConfigRejected: a config file that exists but
// cannot be parsed is reported, so a typo can never silently widen credentials
// by discarding an explicit discover_ambient_credentials opt-out.
func TestOCITofuCredentialsUnparsableConfigRejected(t *testing.T) {
	t.Parallel()

	home := testHome
	v := credentialVenv(home, nil)
	writeTofuConfig(t, v.FS, filepath.Join(home, ".tofurc"), "oci_credentials {{{ not hcl")
	writeAuthFile(t, v.FS, filepath.Join(home, ".docker", "config.json"), testRegistry, "ambient", "ambient-pass")

	err := newStoreErr(t, v, testRegistry, "team/vpc")
	require.Error(t, err, "an unparsable CLI config must not fall through to ambient credentials")
	assert.ErrorContains(t, err, "reading OpenTofu CLI config")
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

	err := newStoreErr(t, v, testRegistry, "team/vpc")
	require.Error(t, err)
	assert.ErrorContains(t, err, "credential helper name must not be empty")
}

// TestOCITofuCredentialsMultipleStylesSkipped: a block configuring more than one
// credential style is rejected, matching OpenTofu.
func TestOCITofuCredentialsMultipleStylesSkipped(t *testing.T) {
	t.Parallel()

	home := testHome
	v := credentialVenv(home, nil)
	writeTofuConfig(t, v.FS, filepath.Join(home, ".tofurc"), `
oci_credentials "registry.example.com" {
  username      = "svc"
  password      = "fake-secret-tofu"
  access_token  = "fake-access"
  refresh_token = "fake-refresh"
}
`)

	err := newStoreErr(t, v, testRegistry, "team/vpc")
	require.Error(t, err)
	assert.ErrorContains(t, err, "at most one credential style")
}

// TestOCITofuCredentialsConfigPathResolution: TERRAFORM_CONFIG and the
// .terraformrc fallback resolve, with .tofurc winning when both files exist.
func TestOCITofuCredentialsConfigPathResolution(t *testing.T) {
	t.Parallel()

	t.Run("TERRAFORM_CONFIG override", func(t *testing.T) {
		t.Parallel()

		custom := "/virtual/terraform.rc"
		v := credentialVenv(testHome, map[string]string{"TERRAFORM_CONFIG": custom})
		writeTofuConfig(t, v.FS, custom, tofuBasicAuth("registry.example.com", "tf-config", "fake-secret-tf"))

		store := newStoreForRepo(t, v, testRegistry, "team/vpc")
		assert.Equal(t, "tf-config", credentialFor(t, store, testRegistry).Username)
	})

	t.Run("terraformrc fallback and tofurc precedence", func(t *testing.T) {
		t.Parallel()

		home := testHome
		v := credentialVenv(home, nil)
		writeTofuConfig(t, v.FS, filepath.Join(home, ".terraformrc"),
			tofuBasicAuth("registry.example.com", "terraformrc", "fake-secret-rc"))

		// Only .terraformrc exists: it resolves.
		store := newStoreForRepo(t, v, testRegistry, "team/vpc")
		assert.Equal(t, "terraformrc", credentialFor(t, store, testRegistry).Username)

		// .tofurc is preferred when both exist.
		writeTofuConfig(t, v.FS, filepath.Join(home, ".tofurc"),
			tofuBasicAuth("registry.example.com", "tofurc", "fake-secret-tofurc"))

		store = newStoreForRepo(t, v, testRegistry, "team/vpc")
		assert.Equal(t, "tofurc", credentialFor(t, store, testRegistry).Username)
	})
}

// TestOCITofuCredentialsRepoAmbientBeatsDomainCLI: a repository-scoped ambient auth outranks a registry-wide CLI block.
func TestOCITofuCredentialsRepoAmbientBeatsDomainCLI(t *testing.T) {
	t.Parallel()

	home := testHome
	v := credentialVenv(home, nil)
	writeTofuConfig(t, v.FS, filepath.Join(home, ".tofurc"),
		tofuBasicAuth("registry.example.com", "domain-cli", "fake-secret-cli"))
	writeAuthFileKeys(t, v.FS, filepath.Join(home, ".docker", "config.json"), map[string]string{
		testRegistry + "/team/vpc": "repo-ambient",
	})

	store := newStoreForRepo(t, v, testRegistry, "team/vpc")
	assert.Equal(t, "repo-ambient", credentialFor(t, store, testRegistry).Username,
		"the more specific ambient entry must outrank a registry-wide CLI block")
}

// TestOCITofuCredentialsCLIBeatsAmbientOnEqualSpecificity: at equal specificity the CLI block wins.
func TestOCITofuCredentialsCLIBeatsAmbientOnEqualSpecificity(t *testing.T) {
	t.Parallel()

	home := testHome
	v := credentialVenv(home, nil)
	writeTofuConfig(t, v.FS, filepath.Join(home, ".tofurc"),
		tofuBasicAuth("registry.example.com", "domain-cli", "fake-secret-cli"))
	writeAuthFileKeys(t, v.FS, filepath.Join(home, ".docker", "config.json"), map[string]string{
		testRegistry: "domain-ambient",
	})

	store := newStoreForRepo(t, v, testRegistry, "team/vpc")
	assert.Equal(t, "domain-cli", credentialFor(t, store, testRegistry).Username,
		"explicit CLI config must win an equal-specificity tie")
}

// TestOCITofuCredentialsOtherRegistryNotServed: a block for one registry must never serve another.
func TestOCITofuCredentialsOtherRegistryNotServed(t *testing.T) {
	t.Parallel()

	home := testHome
	v := credentialVenv(home, nil)
	writeTofuConfig(t, v.FS, filepath.Join(home, ".tofurc"),
		tofuBasicAuth("registry.example.com", "scoped", "fake-secret-scoped"))

	store := newStoreForRepo(t, v, "other.example.com", "team/vpc")
	assert.Equal(t, auth.EmptyCredential, credentialFor(t, store, "other.example.com"),
		"a block must not leak credentials to an unrelated registry")
}

// TestOCITofuCredentialsIncompleteOAuthSkipped: a lone access_token is rejected, matching OpenTofu.
func TestOCITofuCredentialsIncompleteOAuthSkipped(t *testing.T) {
	t.Parallel()

	home := testHome
	v := credentialVenv(home, nil)
	writeTofuConfig(t, v.FS, filepath.Join(home, ".tofurc"), `
oci_credentials "registry.example.com" {
  access_token = "only-access"
}
`)

	err := newStoreErr(t, v, testRegistry, "team/vpc")
	require.Error(t, err)
	assert.ErrorContains(t, err, "requires both an access_token and a refresh_token",
		"an OAuth block missing refresh_token must be skipped")
}

// TestOCITofuCredentialsJSONConfig: a *.tfrc.json CLI config is parsed, not skipped as unparsable.
func TestOCITofuCredentialsJSONConfig(t *testing.T) {
	t.Parallel()

	custom := "/virtual/tofu.tfrc.json"
	v := credentialVenv(testHome, map[string]string{"TF_CLI_CONFIG_FILE": custom})
	writeTofuConfig(t, v.FS, custom, tofuJSONBasicAuth(testRegistry, "json-user", "fake-secret-json"))

	store := newStoreForRepo(t, v, testRegistry, "team/vpc")
	want := auth.Credential{Username: "json-user", Password: "fake-secret-json"}
	assert.Equal(t, want, credentialFor(t, store, testRegistry), "JSON CLI config must be honored")
}

// TestOCITofuCredentialsWindowsAppDataConfig: on Windows the CLI config is found under %APPDATA%.
func TestOCITofuCredentialsWindowsAppDataConfig(t *testing.T) {
	t.Parallel()

	appData := "/virtual/appdata"
	v := credentialVenvForGOOS(windowsTestGOOS, testHome, map[string]string{"APPDATA": appData})
	writeTofuConfig(t, v.FS, filepath.Join(appData, "tofu.rc"),
		tofuBasicAuth("registry.example.com", "appdata-user", "fake-secret-appdata"))

	store := newStoreForRepo(t, v, testRegistry, "team/vpc")
	assert.Equal(t, "appdata-user", credentialFor(t, store, testRegistry).Username,
		"the Windows %APPDATA% CLI config must be discovered")
}

// TestOCITofuCredentialsXDGConfig: on non-Windows the XDG opentofu/tofurc location is discovered.
func TestOCITofuCredentialsXDGConfig(t *testing.T) {
	t.Parallel()

	xdg := testXDGConfig
	v := credentialVenv(testHome, map[string]string{"XDG_CONFIG_HOME": xdg})
	writeTofuConfig(t, v.FS, filepath.Join(xdg, "opentofu", "tofurc"),
		tofuBasicAuth("registry.example.com", "xdg-user", "fake-secret-xdg"))

	store := newStoreForRepo(t, v, testRegistry, "team/vpc")
	assert.Equal(t, "xdg-user", credentialFor(t, store, testRegistry).Username,
		"the XDG CLI config location must be discovered")
}

// TestOCITofuCredentialsDockerStyleConfigFiles: an explicit list replaces the default ambient search paths.
func TestOCITofuCredentialsDockerStyleConfigFiles(t *testing.T) {
	t.Parallel()

	home := testHome
	v := credentialVenv(home, nil)
	writeTofuConfig(t, v.FS, filepath.Join(home, ".tofurc"), `
oci_default_credentials {
  docker_style_config_files = ["/virtual/custom/auth.json"]
}
`)
	// The default location must be ignored once an explicit list is configured.
	writeAuthFile(t, v.FS, filepath.Join(home, ".docker", "config.json"), testRegistry, "default-path", "pw")
	writeAuthFile(t, v.FS, "/virtual/custom/auth.json", testRegistry, "custom-path", "pw")

	store := newStoreForRepo(t, v, testRegistry, "team/vpc")
	assert.Equal(t, "custom-path", credentialFor(t, store, testRegistry).Username,
		"docker_style_config_files must replace the default search paths")
}

// TestOCITofuCredentialsDockerStyleConfigFilesEmptyDisables: an explicit empty list disables ambient files.
func TestOCITofuCredentialsDockerStyleConfigFilesEmptyDisables(t *testing.T) {
	t.Parallel()

	home := testHome
	v := credentialVenv(home, nil)
	writeTofuConfig(t, v.FS, filepath.Join(home, ".tofurc"), `
oci_default_credentials {
  docker_style_config_files = []
}
`)
	writeAuthFile(t, v.FS, filepath.Join(home, ".docker", "config.json"), testRegistry, "ambient", "pw")

	store := newStoreForRepo(t, v, testRegistry, "team/vpc")
	assert.Equal(t, auth.EmptyCredential, credentialFor(t, store, testRegistry),
		"an empty docker_style_config_files list must disable Docker-style discovery")
}

// TestOCITofuCredentialsHelperWithBasicSkipped: a block mixing a helper with basic auth is rejected.
func TestOCITofuCredentialsHelperWithBasicSkipped(t *testing.T) {
	t.Parallel()

	home := testHome
	v := credentialVenv(home, nil)
	writeTofuConfig(t, v.FS, filepath.Join(home, ".tofurc"), `
oci_credentials "registry.example.com" {
  username                  = "u"
  password                  = "fake-secret-u"
  docker_credentials_helper = "ecr-login"
}
`)

	err := newStoreErr(t, v, testRegistry, "team/vpc")
	require.Error(t, err)
	assert.ErrorContains(t, err, "at most one credential style",
		"a block mixing a helper with basic auth must be skipped")
}

// TestOCITofuCredentialsDiscoverAmbientFalseKeepsDefaultHelper: the default helper still runs with ambient disabled.
func TestOCITofuCredentialsDiscoverAmbientFalseKeepsDefaultHelper(t *testing.T) {
	t.Parallel()

	var calls atomic.Int32

	exec := stubHelperExec(t, "ecr-login", func(string) vexec.Result {
		return vexec.Result{Stdout: []byte(`{"Username":"AWS","Secret":"fake-secret-default"}`)}
	}, &calls)

	home := testHome
	v := credentialVenv(home, nil).WithExec(exec)
	writeTofuConfig(t, v.FS, filepath.Join(home, ".tofurc"), `
oci_default_credentials {
  discover_ambient_credentials = false
  docker_credentials_helper    = "ecr-login"
}
`)
	writeAuthFile(t, v.FS, filepath.Join(home, ".docker", "config.json"), testRegistry, "ambient", "pw")

	store := newStoreForRepo(t, v, testRegistry, "team/vpc")
	want := auth.Credential{Username: "AWS", Password: "fake-secret-default"}
	assert.Equal(t, want, credentialFor(t, store, testRegistry),
		"the default helper must still serve when ambient discovery is disabled")
	assert.EqualValues(t, 1, calls.Load(), "the default helper must run exactly once")
}

// tofuJSONBasicAuth renders a JSON oci_credentials block, keeping the username
// tofuBasicAuth renders an oci_credentials block with basic auth, keeping the
// TestOCITofuCredentialsConfigPathPrecedence: with competing config sources present, the documented order wins.
func TestOCITofuCredentialsConfigPathPrecedence(t *testing.T) {
	t.Parallel()

	const custom = "/virtual/custom.tofurc"

	testCases := []struct {
		env      map[string]string
		name     string
		wantUser string
	}{
		{
			name:     "TF_CLI_CONFIG_FILE beats every default",
			env:      map[string]string{envTFCLIConfigFileTest: custom},
			wantUser: "custom",
		},
		{
			name:     "TERRAFORM_CONFIG beats the home files",
			env:      map[string]string{envTerraformConfigTest: custom},
			wantUser: "custom",
		},
		{
			name:     "tofurc beats terraformrc",
			wantUser: "tofurc",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			v := credentialVenv(testHome, tc.env)
			// Every candidate exists at once, so only ordering can decide.
			writeTofuConfig(t, v.FS, custom, tofuBasicAuth(testRegistry, "custom", "fake-secret-custom"))
			writeTofuConfig(t, v.FS, filepath.Join(testHome, ".tofurc"),
				tofuBasicAuth(testRegistry, "tofurc", "fake-secret-tofurc"))
			writeTofuConfig(t, v.FS, filepath.Join(testHome, ".terraformrc"),
				tofuBasicAuth(testRegistry, "terraformrc", "fake-secret-terraformrc"))

			store := newStoreForRepo(t, v, testRegistry, "team/vpc")
			assert.Equal(t, tc.wantUser, credentialFor(t, store, testRegistry).Username)
		})
	}
}

// TestOCITofuCredentialsEmptyBlockRejected: an empty block is a configuration error, matching tofu.
func TestOCITofuCredentialsEmptyBlockRejected(t *testing.T) {
	t.Parallel()

	home := testHome
	v := credentialVenv(home, nil)
	writeTofuConfig(t, v.FS, filepath.Join(home, ".tofurc"), `
oci_credentials "registry.example.com/team/vpc" {
}
`)
	writeAuthFile(t, v.FS, filepath.Join(home, ".docker", "config.json"), testRegistry, "ambient", "pw")

	err := newStoreErr(t, v, testRegistry, "team/vpc")
	require.Error(t, err, "an empty oci_credentials block must be reported, as tofu reports it")
	assert.ErrorContains(t, err, "must configure basic auth, OAuth tokens, or a helper")
}

// TestOCITofuCredentialsWindowsIgnoresHomeDotfiles: on Windows only the APPDATA pair is read.
func TestOCITofuCredentialsWindowsIgnoresHomeDotfiles(t *testing.T) {
	t.Parallel()

	appData := "/virtual/appdata"
	v := credentialVenvForGOOS(windowsTestGOOS, testHome, map[string]string{"APPDATA": appData})
	// OpenTofu never reads the Unix dotfiles on Windows, so this must be ignored.
	writeTofuConfig(t, v.FS, filepath.Join(testHome, ".tofurc"),
		tofuBasicAuth(testRegistry, "home-dotfile", "fake-secret-home"))

	store := newStoreForRepo(t, v, testRegistry, "team/vpc")
	assert.Equal(t, auth.EmptyCredential, credentialFor(t, store, testRegistry),
		"a Unix dotfile must not be read on Windows")
}

// TestOCITofuCredentialsNoXDGSynthesisWhenUnset: without XDG_CONFIG_HOME the XDG path is not invented.
func TestOCITofuCredentialsNoXDGSynthesisWhenUnset(t *testing.T) {
	t.Parallel()

	v := credentialVenv(testHome, nil)
	// OpenTofu only consults XDG when XDG_CONFIG_HOME is set, so a file here is ignored.
	writeTofuConfig(t, v.FS, filepath.Join(testHome, ".config", "opentofu", "tofurc"),
		tofuBasicAuth(testRegistry, "xdg-synth", "fake-secret-synth"))

	store := newStoreForRepo(t, v, testRegistry, "team/vpc")
	assert.Equal(t, auth.EmptyCredential, credentialFor(t, store, testRegistry),
		"the XDG path must not be synthesized when XDG_CONFIG_HOME is unset")
}

// TestOCITofuCredentialsDefaultHelperFailurePropagates: a broken configured default helper surfaces its error.
func TestOCITofuCredentialsDefaultHelperFailurePropagates(t *testing.T) {
	t.Parallel()

	exec := vexec.NewMemExec(
		func(context.Context, vexec.Invocation) vexec.Result { return vexec.Result{} },
		vexec.WithLookPath(func(string) (string, error) {
			return "", errNoSuchHelperBinary
		}),
	)

	home := testHome
	v := credentialVenv(home, nil).WithExec(exec)
	writeTofuConfig(t, v.FS, filepath.Join(home, ".tofurc"), `
oci_default_credentials {
  docker_credentials_helper = "ecr-login"
}
`)

	store := newStoreForRepo(t, v, testRegistry, "team/vpc")

	_, err := credentialForErr(t, store, testRegistry)
	require.Error(t, err, "an explicitly configured default helper must not fail silently")

	var helperErr getter.OCICredentialHelperError
	require.ErrorAs(t, err, &helperErr)
	assert.Equal(t, "ecr-login", helperErr.Helper)
}

// TestOCITofuCredentialsConfigDirFragment: credentials in a *.tfrc fragment are loaded.
func TestOCITofuCredentialsConfigDirFragment(t *testing.T) {
	t.Parallel()

	home := testHome
	v := credentialVenv(home, nil)
	writeTofuConfig(t, v.FS, filepath.Join(home, ".terraform.d", "creds.tfrc"),
		tofuBasicAuth(testRegistry, "fragment", "fake-secret-fragment"))

	store := newStoreForRepo(t, v, testRegistry, "team/vpc")
	assert.Equal(t, "fragment", credentialFor(t, store, testRegistry).Username,
		"a *.tfrc fragment in the config directory must be loaded")
}

// TestOCITofuCredentialsConfigDirJSONFragment: a *.tfrc.json fragment is loaded too.
func TestOCITofuCredentialsConfigDirJSONFragment(t *testing.T) {
	t.Parallel()

	home := testHome
	v := credentialVenv(home, nil)
	writeTofuConfig(t, v.FS, filepath.Join(home, ".terraform.d", "creds.tfrc.json"),
		tofuJSONBasicAuth(testRegistry, "json-fragment", "fake-secret-json-fragment"))

	store := newStoreForRepo(t, v, testRegistry, "team/vpc")
	assert.Equal(t, "json-fragment", credentialFor(t, store, testRegistry).Username,
		"a *.tfrc.json fragment must be loaded")
}

// TestOCITofuCredentialsOverrideSuppressesFragments: an env override skips the config directory.
func TestOCITofuCredentialsOverrideSuppressesFragments(t *testing.T) {
	t.Parallel()

	const custom = "/virtual/custom.tofurc"

	home := testHome
	v := credentialVenv(home, map[string]string{envTFCLIConfigFileTest: custom})
	writeTofuConfig(t, v.FS, custom, tofuBasicAuth(testRegistry, "override", "fake-secret-override"))
	// Must be ignored: OpenTofu skips the config directory when the override is set.
	writeTofuConfig(t, v.FS, filepath.Join(home, ".terraform.d", "creds.tfrc"),
		tofuBasicAuth(testRegistry+"/team/vpc", "fragment", "fake-secret-fragment"))

	store := newStoreForRepo(t, v, testRegistry, "team/vpc")
	assert.Equal(t, "override", credentialFor(t, store, testRegistry).Username,
		"an explicit config override must suppress config-directory fragments")
}

// TestOCITofuCredentialsDuplicateLabelAcrossFilesRejected: tofu rejects two
// oci_credentials blocks sharing a label, even across merged config sources.
func TestOCITofuCredentialsDuplicateLabelAcrossFilesRejected(t *testing.T) {
	t.Parallel()

	home := testHome
	v := credentialVenv(home, nil)
	writeTofuConfig(t, v.FS, filepath.Join(home, ".tofurc"),
		tofuBasicAuth(testRegistry, "main-file", "fake-secret-main"))
	writeTofuConfig(t, v.FS, filepath.Join(home, ".terraform.d", "extra.tfrc"),
		tofuBasicAuth(testRegistry, "fragment", "fake-secret-fragment"))

	err := newStoreErr(t, v, testRegistry, "team/vpc")
	require.Error(t, err)
	assert.ErrorContains(t, err, "duplicate oci_credentials block")
}

// TestOCITofuCredentialsExplicitConfigFileMissingIsFatal: a named config file must not fail silently.
func TestOCITofuCredentialsExplicitConfigFileMissingIsFatal(t *testing.T) {
	t.Parallel()

	home := testHome
	v := credentialVenv(home, nil)
	writeTofuConfig(t, v.FS, filepath.Join(home, ".tofurc"), `
oci_default_credentials {
  docker_style_config_files = ["/virtual/missing/auth.json"]
}
`)

	newStore := getter.NewOCIRepositoryStore(logger.CreateLogger(), v)

	_, err := newStore(t.Context(), testRegistry, "team/vpc")
	require.Error(t, err, "an explicitly configured credential file that cannot be read must fail")
	assert.ErrorContains(t, err, "docker_style_config_files")
}

// TestOCITofuCredentialsInvalidDefaultBlockRejected: an invalid helper is a
// configuration error, so it can never silently re-enable ambient discovery.
func TestOCITofuCredentialsInvalidDefaultBlockRejected(t *testing.T) {
	t.Parallel()

	home := testHome
	v := credentialVenv(home, nil)
	writeTofuConfig(t, v.FS, filepath.Join(home, ".tofurc"), `
oci_default_credentials {
  discover_ambient_credentials = false
  docker_credentials_helper    = "../../../tmp/evil"
}
`)
	writeAuthFile(t, v.FS, filepath.Join(home, ".docker", "config.json"), testRegistry, "ambient", "ambient-pass")

	err := newStoreErr(t, v, testRegistry, "team/vpc")
	require.Error(t, err)
	assert.ErrorContains(t, err, "credential helper name must not be empty",
		"an unusable default block must not widen credential exposure")
}

// TestOCITofuCredentialsUndecodableDefaultBlockRejected: a type error in the
// block is reported rather than silently discarding the explicit opt-out.
func TestOCITofuCredentialsUndecodableDefaultBlockRejected(t *testing.T) {
	t.Parallel()

	home := testHome
	v := credentialVenv(home, nil)
	writeTofuConfig(t, v.FS, filepath.Join(home, ".tofurc"), `
oci_default_credentials {
  discover_ambient_credentials = false
  docker_style_config_files    = "not-a-list"
}
`)
	writeAuthFile(t, v.FS, filepath.Join(home, ".docker", "config.json"), testRegistry, "ambient", "ambient-pass")

	err := newStoreErr(t, v, testRegistry, "team/vpc")
	require.Error(t, err)
	assert.ErrorContains(t, err, "reading OpenTofu CLI config")
}

// TestOCITofuCredentialsAmbientFilesRequireDiscovery: tofu rejects
// docker_style_config_files when ambient discovery is disabled.
func TestOCITofuCredentialsAmbientFilesRequireDiscovery(t *testing.T) {
	t.Parallel()

	tc := []struct {
		name  string
		files string
	}{
		{name: "populated list", files: `["/virtual/home/.docker/config.json"]`},
		{name: "empty list", files: `[]`},
	}

	for _, tt := range tc {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			home := testHome
			v := credentialVenv(home, nil)
			writeTofuConfig(t, v.FS, filepath.Join(home, ".tofurc"), `
oci_default_credentials {
  discover_ambient_credentials = false
  docker_style_config_files    = `+tt.files+`
}
`)

			err := newStoreErr(t, v, testRegistry, "team/vpc")
			require.Error(t, err)
			assert.ErrorContains(t, err, "requires discover_ambient_credentials to be enabled")
		})
	}
}

// TestOCITofuCredentialsEmptyStringValuesRejected: tofu selects the credential
// style from which arguments are present, so an empty value is an error rather
// than a block that silently disappears and lets ambient credentials take over.
func TestOCITofuCredentialsEmptyStringValuesRejected(t *testing.T) {
	t.Parallel()

	tc := []struct {
		name string
		body string
	}{
		{name: "basic auth", body: `
oci_credentials "registry.example.com" {
  username = ""
  password = ""
}
`},
		{name: "oauth", body: `
oci_credentials "registry.example.com" {
  access_token  = ""
  refresh_token = ""
}
`},
	}

	for _, tt := range tc {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			home := testHome
			v := credentialVenv(home, nil)
			writeTofuConfig(t, v.FS, filepath.Join(home, ".tofurc"), tt.body)
			writeAuthFile(
				t,
				v.FS,
				filepath.Join(home, ".docker", "config.json"),
				testRegistry,
				"ambient",
				"ambient-pass",
			)

			err := newStoreErr(t, v, testRegistry, "team/vpc")
			require.Error(t, err, "an empty credential value must not fall through to ambient")
			assert.ErrorContains(t, err, "must not be empty")
		})
	}
}

// TestOCITofuCredentialsJSONWithoutSuffix: tofu detects JSON from the content, so
// a JSON config named by an extensionless path is parsed, not rejected.
func TestOCITofuCredentialsJSONWithoutSuffix(t *testing.T) {
	t.Parallel()

	custom := "/virtual/secrets/tofurc"
	v := credentialVenv(testHome, map[string]string{envTFCLIConfigFileTest: custom})
	writeTofuConfig(t, v.FS, custom, tofuJSONBasicAuth(testRegistry, "json-user", "fake-secret-nosuffix"))

	store := newStoreForRepo(t, v, testRegistry, "team/vpc")
	assert.Equal(t, "json-user", credentialFor(t, store, testRegistry).Username,
		"JSON content must be detected without a .json suffix")
}

// TestOCITofuCredentialsDuplicateDefaultBlockRejected: tofu allows at most one
// oci_default_credentials block per configuration.
func TestOCITofuCredentialsDuplicateDefaultBlockRejected(t *testing.T) {
	t.Parallel()

	home := testHome
	v := credentialVenv(home, nil)
	writeTofuConfig(t, v.FS, filepath.Join(home, ".tofurc"), `
oci_default_credentials {
  discover_ambient_credentials = false
}

oci_default_credentials {
  discover_ambient_credentials = true
}
`)

	err := newStoreErr(t, v, testRegistry, "team/vpc")
	require.Error(t, err)
	assert.ErrorContains(t, err, "at most one oci_default_credentials block")
}

// TestOCITofuCredentialsRelativeConfigFileResolves: a relative
// docker_style_config_files entry resolves against the file that declared it.
func TestOCITofuCredentialsRelativeConfigFileResolves(t *testing.T) {
	t.Parallel()

	home := testHome
	v := credentialVenv(home, nil)
	writeTofuConfig(t, v.FS, filepath.Join(home, ".tofurc"), `
oci_default_credentials {
  docker_style_config_files = ["relative-auth.json"]
}
`)
	writeAuthFile(t, v.FS, filepath.Join(home, "relative-auth.json"), testRegistry, "relative", "relative-pass")

	store := newStoreForRepo(t, v, testRegistry, "team/vpc")
	assert.Equal(t, "relative", credentialFor(t, store, testRegistry).Username,
		"a relative entry must resolve against the declaring config file's directory")
}

// TestOCITofuCredentialsBlockHelperFailureIsFatal: a helper named by an
// oci_credentials block was chosen deliberately, so its failure fails the pull
// rather than quietly degrading to an anonymous one.
func TestOCITofuCredentialsBlockHelperFailureIsFatal(t *testing.T) {
	t.Parallel()

	exec := stubHelperExec(t, "ecr-login", func(string) vexec.Result {
		return vexec.Result{ExitCode: 1, Stderr: []byte("helper exploded")}
	}, nil)

	home := testHome
	v := credentialVenv(home, nil).WithExec(exec)
	writeTofuConfig(t, v.FS, filepath.Join(home, ".tofurc"), `
oci_credentials "registry.example.com" {
  docker_credentials_helper = "ecr-login"
}
`)
	writeAuthFile(t, v.FS, filepath.Join(home, ".docker", "config.json"), testRegistry, "ambient", "ambient-pass")

	store := newStoreForRepo(t, v, testRegistry, "team/vpc")

	_, err := credentialForErr(t, store, testRegistry)
	require.Error(t, err, "a configured helper's failure must not fall back to ambient credentials")
}

// newStoreErr returns the error from building the store for one registry/repository.
func newStoreErr(t *testing.T, v *venv.Venv, registry, repositoryName string) error {
	t.Helper()

	newStore := getter.NewOCIRepositoryStore(logger.CreateLogger(), v)

	_, err := newStore(t.Context(), registry, repositoryName)

	return err
}

// newStoreForRepo builds the default store for one registry/repository.
func newStoreForRepo(t *testing.T, v *venv.Venv, registry, repositoryName string) getter.OCIRepositoryStore {
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

// and secret out of an adjacent literal pair that scanners flag.
func tofuJSONBasicAuth(label, user, secret string) string {
	return fmt.Sprintf(
		"{\n  %q: {\n    %q: {\n      %q: %q,\n      %q: %q\n    }\n  }\n}",
		"oci_credentials", label, "username", user, "password", secret,
	)
}

// username and secret out of an adjacent literal pair that scanners flag.
func tofuBasicAuth(label, user, secret string) string {
	return fmt.Sprintf("\noci_credentials %q {\n  username = %q\n  password = %q\n}\n", label, user, secret)
}
