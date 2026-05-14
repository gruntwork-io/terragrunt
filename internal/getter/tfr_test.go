package getter_test

import (
	"archive/zip"
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/getter"
	"github.com/gruntwork-io/terragrunt/internal/tfimpl"
	"github.com/gruntwork-io/terragrunt/internal/util"
	"github.com/gruntwork-io/terragrunt/test/helpers"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	gogetter "github.com/hashicorp/go-getter/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRegistryGetterRootDir(t *testing.T) {
	t.Parallel()

	server := newRegistryTestServer(t)

	dstPath := helpers.TmpDirWOSymlinks(t)
	moduleDestPath := filepath.Join(dstPath, "terraform-aws-vpc")
	require.False(t, util.FileExists(filepath.Join(moduleDestPath, "main.tf")))

	src := "tfr://" + server.Listener.Addr().String() + "/terraform-aws-modules/vpc/aws?version=3.3.0"
	client := newRegistryTestClient(t, server.Client(), tfimpl.Terraform)

	_, err := client.Get(t.Context(), &getter.Request{
		Src:     src,
		Dst:     moduleDestPath,
		GetMode: getter.ModeDir,
	})
	require.NoError(t, err)
	assert.True(t, util.FileExists(filepath.Join(moduleDestPath, "main.tf")))
}

func TestRegistryGetterSubModule(t *testing.T) {
	t.Parallel()

	server := newRegistryTestServer(t)

	dstPath := helpers.TmpDirWOSymlinks(t)
	moduleDestPath := filepath.Join(dstPath, "terraform-aws-vpc")
	require.False(t, util.FileExists(filepath.Join(moduleDestPath, "main.tf")))

	src := "tfr://" + server.Listener.Addr().String() + "/terraform-aws-modules/vpc/aws//modules/vpc-endpoints?version=3.3.0"
	client := newRegistryTestClient(t, server.Client(), tfimpl.Terraform)

	_, err := client.Get(t.Context(), &getter.Request{
		Src:     src,
		Dst:     moduleDestPath,
		GetMode: getter.ModeDir,
	})
	require.NoError(t, err)
	assert.True(t, util.FileExists(filepath.Join(moduleDestPath, "main.tf")))
}

// TestRegistryGetterSubdirInTerraformGetHeader pins the path where the
// X-Terraform-Get header itself carries a "//subdir" selector. v2's outer
// client only strips the subdir from req.Src, so a subdir hidden in the
// registry's redirect target is left for RegistryGetter.getSubdir to resolve.
func TestRegistryGetterSubdirInTerraformGetHeader(t *testing.T) {
	t.Parallel()

	zipBody := buildModuleZip(t)

	mux := http.NewServeMux()

	mux.HandleFunc("/.well-known/terraform.json", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, err := w.Write([]byte(`{"modules.v1":"/v1/modules/"}`))
		assert.NoError(t, err)
	})

	mux.HandleFunc("/v1/modules/terraform-aws-modules/vpc/aws/3.3.0/download", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Terraform-Get", "https://"+r.Host+"/download/terraform-aws-vpc.zip//modules/vpc-endpoints")
		w.WriteHeader(http.StatusNoContent)
	})

	mux.HandleFunc("/download/terraform-aws-vpc.zip", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/zip")

		_, err := w.Write(zipBody)
		assert.NoError(t, err)
	})

	server := httptest.NewTLSServer(mux)
	t.Cleanup(server.Close)

	dstPath := helpers.TmpDirWOSymlinks(t)
	moduleDestPath := filepath.Join(dstPath, "terraform-aws-vpc")

	src := "tfr://" + server.Listener.Addr().String() + "/terraform-aws-modules/vpc/aws?version=3.3.0"
	client := newRegistryTestClient(t, server.Client(), tfimpl.Terraform)

	_, err := client.Get(t.Context(), &getter.Request{
		Src:     src,
		Dst:     moduleDestPath,
		GetMode: getter.ModeDir,
	})
	require.NoError(t, err)

	body, err := os.ReadFile(filepath.Join(moduleDestPath, "main.tf"))
	require.NoError(t, err)
	assert.Equal(t, "# vpc-endpoints submodule\n", string(body))
}

// TestRegistryGetterMultipleVersions pins the typed error when more than one
// version query value is supplied. The registry protocol expects exactly one.
func TestRegistryGetterMultipleVersions(t *testing.T) {
	t.Parallel()

	server := newRegistryTestServer(t)
	client := newRegistryTestClient(t, server.Client(), tfimpl.Terraform)

	_, err := client.Get(t.Context(), &getter.Request{
		Src:     "tfr://" + server.Listener.Addr().String() + "/foo/bar/baz?version=1&version=2",
		Dst:     helpers.TmpDirWOSymlinks(t),
		GetMode: getter.ModeDir,
	})
	require.Error(t, err)

	var typed getter.MalformedRegistryURLErr

	require.ErrorAs(t, err, &typed)
}

// TestRegistryGetterWithoutVersion verifies that omitting ?version= causes the
// getter to resolve and use the latest version from the registry.
func TestRegistryGetterWithoutVersion(t *testing.T) {
	t.Parallel()

	server := newRegistryTestServer(t)

	dstPath := helpers.TmpDirWOSymlinks(t)
	moduleDestPath := filepath.Join(dstPath, "terraform-aws-vpc")
	require.False(t, util.FileExists(filepath.Join(moduleDestPath, "main.tf")))

	// No ?version= — getter resolves latest (3.3.0) via the versions endpoint.
	src := "tfr://" + server.Listener.Addr().String() + "/terraform-aws-modules/vpc/aws"
	client := newRegistryTestClient(t, server.Client(), tfimpl.Terraform)

	_, err := client.Get(t.Context(), &getter.Request{
		Src:     src,
		Dst:     moduleDestPath,
		GetMode: getter.ModeDir,
	})
	require.NoError(t, err)
	assert.True(t, util.FileExists(filepath.Join(moduleDestPath, "main.tf")))
}

// TestRegistryGetterEmptyVersion pins the typed error returned when
// ?version= is present but empty.
func TestRegistryGetterEmptyVersion(t *testing.T) {
	t.Parallel()

	server := newRegistryTestServer(t)
	client := newRegistryTestClient(t, server.Client(), tfimpl.Terraform)

	_, err := client.Get(t.Context(), &getter.Request{
		Src:     "tfr://" + server.Listener.Addr().String() + "/foo/bar/baz?version=",
		Dst:     helpers.TmpDirWOSymlinks(t),
		GetMode: getter.ModeDir,
	})
	require.Error(t, err)

	var typed getter.MalformedRegistryURLErr
	require.ErrorAs(t, err, &typed)
}

// newRegistryTestClient builds a Client wired to the supplied http.Client so
// it trusts the test server's self-signed TLS certificate, both for the
// registry-protocol calls (via RegistryGetter.HTTPClient) and for the module
// archive download (via a prepended HttpGetter). The prepend ordering is what
// guarantees the test client's HttpGetter wins detection over the default one.
func newRegistryTestClient(t *testing.T, httpClient *http.Client, impl tfimpl.Type) *getter.Client {
	t.Helper()

	l := logger.CreateLogger()

	tfr := getter.NewRegistryGetter(l).
		WithHTTPClient(httpClient).
		WithTofuImplementation(impl)

	return getter.NewClient(
		getter.WithCustomGettersPrepended(
			tfr,
			&gogetter.HttpGetter{Client: httpClient, Netrc: true},
		),
	)
}

// buildModuleZip builds an in-memory zip archive that mirrors the shape of a
// terraform-aws-vpc module: a root main.tf plus a submodule under
// modules/vpc-endpoints/main.tf. It is served by the mock registry in place
// of a real GitHub tarball.
func buildModuleZip(t *testing.T) []byte {
	t.Helper()

	var buf bytes.Buffer

	zw := zip.NewWriter(&buf)

	files := map[string]string{
		"main.tf":                       "# root module\n",
		"modules/vpc-endpoints/main.tf": "# vpc-endpoints submodule\n",
	}

	for name, content := range files {
		w, err := zw.Create(name)
		require.NoError(t, err)

		_, err = w.Write([]byte(content))
		require.NoError(t, err)
	}

	require.NoError(t, zw.Close())

	return buf.Bytes()
}
