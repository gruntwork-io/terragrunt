package getter_test

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/getter"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetModuleRegistryURLBasePath(t *testing.T) {
	t.Parallel()

	server := newRegistryTestServer(t)

	basePath, err := getter.GetModuleRegistryURLBasePath(
		t.Context(), logger.CreateLogger(), server.Client(), server.Listener.Addr().String(),
	)
	require.NoError(t, err)
	assert.Equal(t, "/v1/modules/", basePath)
}

func TestGetTerraformGetHeader(t *testing.T) {
	t.Parallel()

	server := newRegistryTestServer(t)

	moduleURL := url.URL{
		Scheme: "https",
		Host:   server.Listener.Addr().String(),
		Path:   "/v1/modules/terraform-aws-modules/vpc/aws/3.3.0/download",
	}

	header, err := getter.GetTerraformGetHeader(t.Context(), logger.CreateLogger(), server.Client(), &moduleURL)
	require.NoError(t, err)
	assert.Contains(t, header, "/download/terraform-aws-vpc.zip")
}

func TestGetDownloadURLFromHeader(t *testing.T) {
	t.Parallel()

	tests := []struct {
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

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			downloadURL, err := getter.GetDownloadURLFromHeader(&tc.moduleURL, tc.terraformGet)
			require.NoError(t, err)
			assert.Equal(t, tc.expectedResult, downloadURL)
		})
	}
}

func TestBuildRequestURLFullPath(t *testing.T) {
	t.Parallel()

	requestURL, err := getter.BuildRequestURL(
		"gruntwork.io",
		"https://gruntwork.io/registry/modules/v1/",
		"/tfr-project/terraform-aws-tfr",
		"6.6.6",
	)
	require.NoError(t, err)
	assert.Equal(t,
		"https://gruntwork.io/registry/modules/v1/tfr-project/terraform-aws-tfr/6.6.6/download",
		requestURL.String(),
	)
}

func TestBuildRequestURLRelativePath(t *testing.T) {
	t.Parallel()

	requestURL, err := getter.BuildRequestURL(
		"gruntwork.io",
		"/registry/modules/v1",
		"/tfr-project/terraform-aws-tfr",
		"6.6.6",
	)
	require.NoError(t, err)
	assert.Equal(t,
		"https://gruntwork.io/registry/modules/v1/tfr-project/terraform-aws-tfr/6.6.6/download",
		requestURL.String(),
	)
}

// newRegistryTestServer stands up an httptest TLS server that speaks enough of
// the OpenTofu/Terraform module-registry protocol to satisfy the
// RegistryGetter: the service-discovery document, a module download endpoint
// that returns an X-Terraform-Get header, and the zip archive the header
// points at.
func newRegistryTestServer(t *testing.T) *httptest.Server {
	t.Helper()

	zipBody := buildModuleZip(t)

	mux := http.NewServeMux()

	mux.HandleFunc("/.well-known/terraform.json", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, err := w.Write([]byte(`{"modules.v1":"/v1/modules/"}`))
		assert.NoError(t, err)
	})

	mux.HandleFunc("/v1/modules/terraform-aws-modules/vpc/aws/3.3.0/download", func(w http.ResponseWriter, r *http.Request) {
		// Resolve against the request host so the downloader hits the same
		// test server we are about to shut down at end-of-test.
		w.Header().Set("X-Terraform-Get", "https://"+r.Host+"/download/terraform-aws-vpc.zip")
		w.WriteHeader(http.StatusNoContent)
	})

	mux.HandleFunc("/download/terraform-aws-vpc.zip", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/zip")

		_, err := w.Write(zipBody)
		assert.NoError(t, err)
	})

	server := httptest.NewTLSServer(mux)
	t.Cleanup(server.Close)

	return server
}
