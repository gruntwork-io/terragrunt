package tf_test

import (
	"archive/zip"
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/tf"
	"github.com/gruntwork-io/terragrunt/internal/tfimpl"
	"github.com/gruntwork-io/terragrunt/internal/util"
	"github.com/gruntwork-io/terragrunt/test/helpers"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/hashicorp/go-getter"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetModuleRegistryURLBasePath(t *testing.T) {
	t.Parallel()

	server := newRegistryTestServer(t)

	basePath, err := tf.GetModuleRegistryURLBasePath(t.Context(), logger.CreateLogger(), server.Client(), server.Listener.Addr().String())
	require.NoError(t, err)
	assert.Equal(t, "/v1/modules/", basePath)
}

func TestGetTerraformHeader(t *testing.T) {
	t.Parallel()

	server := newRegistryTestServer(t)

	testModuleURL := url.URL{
		Scheme: "https",
		Host:   server.Listener.Addr().String(),
		Path:   "/v1/modules/terraform-aws-modules/vpc/aws/3.3.0/download",
	}
	terraformGetHeader, err := tf.GetTerraformGetHeader(t.Context(), logger.CreateLogger(), server.Client(), &testModuleURL)
	require.NoError(t, err)
	assert.Contains(t, terraformGetHeader, "/download/terraform-aws-vpc.zip")
}

func TestGetDownloadURLFromHeader(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name           string
		terraformGet   string
		expectedResult string
		moduleURL      url.URL
	}{
		{
			name: "BaseWithRoot",
			moduleURL: url.URL{
				Scheme: "https",
				Host:   "registry.terraform.io",
			},
			terraformGet:   "/terraform-aws-modules/terraform-aws-vpc",
			expectedResult: "https://registry.terraform.io/terraform-aws-modules/terraform-aws-vpc",
		},
		{
			name:           "PrefixedURL",
			moduleURL:      url.URL{},
			terraformGet:   "github.com/terraform-aws-modules/terraform-aws-vpc",
			expectedResult: "github.com/terraform-aws-modules/terraform-aws-vpc",
		},
		{
			name: "PathWithRoot",
			moduleURL: url.URL{
				Scheme: "https",
				Host:   "registry.terraform.io",
				Path:   "modules/foo/bar",
			},
			terraformGet:   "/terraform-aws-modules/terraform-aws-vpc",
			expectedResult: "https://registry.terraform.io/terraform-aws-modules/terraform-aws-vpc",
		},
		{
			name: "PathWithRelativeRoot",
			moduleURL: url.URL{
				Scheme: "https",
				Host:   "registry.terraform.io",
				Path:   "modules/foo/bar",
			},
			terraformGet:   "./terraform-aws-modules/terraform-aws-vpc",
			expectedResult: "https://registry.terraform.io/modules/foo/terraform-aws-modules/terraform-aws-vpc",
		},
		{
			name: "PathWithRelativeParent",
			moduleURL: url.URL{
				Scheme: "https",
				Host:   "registry.terraform.io",
				Path:   "modules/foo/bar",
			},
			terraformGet:   "../terraform-aws-modules/terraform-aws-vpc",
			expectedResult: "https://registry.terraform.io/modules/terraform-aws-modules/terraform-aws-vpc",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			downloadURL, err := tf.GetDownloadURLFromHeader(&tc.moduleURL, tc.terraformGet)
			require.NoError(t, err)
			assert.Equal(t, tc.expectedResult, downloadURL)
		})
	}
}

func TestTFRGetterRootDir(t *testing.T) {
	t.Parallel()

	server := newRegistryTestServer(t)

	testModuleURL, err := url.Parse("tfr://" + server.Listener.Addr().String() + "/terraform-aws-modules/vpc/aws?version=3.3.0")
	require.NoError(t, err)

	dstPath := helpers.TmpDirWOSymlinks(t)

	// The dest path must not exist for go getter to work
	moduleDestPath := filepath.Join(dstPath, "terraform-aws-vpc")
	assert.False(t, util.FileExists(filepath.Join(moduleDestPath, "main.tf")))

	tfrGetter := tf.NewRegistryGetter(logger.CreateLogger()).
		WithTofuImplementation(tfimpl.Terraform).
		WithHTTPClient(server.Client())
	tfrGetter.SetClient(newGetterClientWithHTTPClient(t.Context(), server.Client()))

	require.NoError(t, tfrGetter.Get(moduleDestPath, testModuleURL))
	assert.True(t, util.FileExists(filepath.Join(moduleDestPath, "main.tf")))
}

func TestTFRGetterSubModule(t *testing.T) {
	t.Parallel()

	server := newRegistryTestServer(t)

	testModuleURL, err := url.Parse("tfr://" + server.Listener.Addr().String() + "/terraform-aws-modules/vpc/aws//modules/vpc-endpoints?version=3.3.0")
	require.NoError(t, err)

	dstPath := helpers.TmpDirWOSymlinks(t)

	// The dest path must not exist for go getter to work
	moduleDestPath := filepath.Join(dstPath, "terraform-aws-vpc")
	assert.False(t, util.FileExists(filepath.Join(moduleDestPath, "main.tf")))

	tfrGetter := tf.NewRegistryGetter(logger.CreateLogger()).
		WithTofuImplementation(tfimpl.Terraform).
		WithHTTPClient(server.Client())
	tfrGetter.SetClient(newGetterClientWithHTTPClient(t.Context(), server.Client()))

	require.NoError(t, tfrGetter.Get(moduleDestPath, testModuleURL))
	assert.True(t, util.FileExists(filepath.Join(moduleDestPath, "main.tf")))
}

func TestBuildRequestUrlFullPath(t *testing.T) {
	t.Parallel()

	requestURL, err := tf.BuildRequestURL("gruntwork.io", "https://gruntwork.io/registry/modules/v1/", "/tfr-project/terraform-aws-tfr", "6.6.6")
	require.NoError(t, err)
	assert.Equal(t, "https://gruntwork.io/registry/modules/v1/tfr-project/terraform-aws-tfr/6.6.6/download", requestURL.String())
}

func TestBuildRequestUrlRelativePath(t *testing.T) {
	t.Parallel()

	requestURL, err := tf.BuildRequestURL("gruntwork.io", "/registry/modules/v1", "/tfr-project/terraform-aws-tfr", "6.6.6")
	require.NoError(t, err)
	assert.Equal(t, "https://gruntwork.io/registry/modules/v1/tfr-project/terraform-aws-tfr/6.6.6/download", requestURL.String())
}

// buildModuleZip builds an in-memory zip archive that mirrors the shape of a
// terraform-aws-vpc module: a root main.tf plus a submodule under
// modules/vpc-endpoints/main.tf. It is served by the mock registry in place of
// a real GitHub tarball.
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

		n, err := w.Write([]byte(content))
		require.NoError(t, err)

		require.Equal(t, len(content), n)
	}

	require.NoError(t, zw.Close())

	return buf.Bytes()
}

// newRegistryTestServer stands up an httptest TLS server that speaks enough of
// the Terraform module-registry protocol to satisfy the RegistryGetter: the
// service-discovery document, a module download endpoint that returns an
// X-Terraform-Get header, and the zip archive the header points at.
func newRegistryTestServer(t *testing.T) *httptest.Server {
	t.Helper()

	zipBody := buildModuleZip(t)

	mux := http.NewServeMux()

	mux.HandleFunc("/.well-known/terraform.json", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"modules.v1":"/v1/modules/"}`)
	})

	mux.HandleFunc("/v1/modules/terraform-aws-modules/vpc/aws/3.3.0/download", func(w http.ResponseWriter, r *http.Request) {
		// Resolve against the request host so the downloader hits the same
		// test server we are about to shut down at end-of-test.
		w.Header().Set("X-Terraform-Get", "https://"+r.Host+"/download/terraform-aws-vpc.zip")
		w.WriteHeader(http.StatusNoContent)
	})

	mux.HandleFunc("/download/terraform-aws-vpc.zip", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/zip")

		n, err := w.Write(zipBody)
		assert.NoError(t, err)

		assert.Equal(t, len(zipBody), n)
	})

	server := httptest.NewTLSServer(mux)
	t.Cleanup(server.Close)

	return server
}

// newGetterClientWithHTTPClient returns a *getter.Client whose Options install
// a custom HttpGetter that trusts the test server's self-signed cert.
func newGetterClientWithHTTPClient(ctx context.Context, c *http.Client) *getter.Client {
	httpGetter := &getter.HttpGetter{Client: c}

	return &getter.Client{
		Ctx: ctx,
		Options: []getter.ClientOption{
			getter.WithGetters(map[string]getter.Getter{
				"http":  httpGetter,
				"https": httpGetter,
				"file":  new(getter.FileGetter),
			}),
		},
	}
}
