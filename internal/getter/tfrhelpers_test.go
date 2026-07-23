package getter_test

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/getter"
	"github.com/gruntwork-io/terragrunt/internal/tfimpl"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestVersionResolverMemoizesWithRacing pins that concurrent and repeated
// resolutions for the same module and constraint query the registry's
// list-versions endpoint exactly once.
func TestVersionResolverMemoizesWithRacing(t *testing.T) {
	t.Parallel()

	var versionsHits atomic.Int64

	mux := http.NewServeMux()
	mux.HandleFunc("/.well-known/terraform.json", func(w http.ResponseWriter, _ *http.Request) {
		_, err := w.Write([]byte(`{"modules.v1":"/v1/modules/"}`))
		assert.NoError(t, err)
	})
	mux.HandleFunc(
		"/v1/modules/foo/bar/baz/versions",
		func(w http.ResponseWriter, _ *http.Request) {
			versionsHits.Add(1)

			_, err := w.Write(
				[]byte(`{"modules":[{"versions":[{"version":"3.3.0"},{"version":"2.0.0"}]}]}`),
			)
			assert.NoError(t, err)
		},
	)

	server := httptest.NewTLSServer(mux)
	t.Cleanup(server.Close)

	resolver := getter.NewVersionResolver().WithHTTPClient(server.Client())
	source := "tfr://" + server.Listener.Addr().String() + "/foo/bar/baz"

	var wg sync.WaitGroup

	for range 10 {
		wg.Add(1)

		go func() {
			defer wg.Done()

			pinned, err := resolver.Pin(
				t.Context(), logger.CreateLogger(), tfimpl.OpenTofu, source, "~> 3.0",
			)
			assert.NoError(t, err)
			assert.Equal(t, source+"?version=3.3.0", pinned)
		}()
	}

	wg.Wait()

	assert.Equal(t, int64(1), versionsHits.Load())
}

func TestPinModuleVersion(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name       string
		constraint string
		want       string
	}{
		{name: "pessimistic major", constraint: "~> 3.0", want: "3.4.0"},
		{name: "pessimistic minor", constraint: "~> 3.3", want: "3.4.0"},
		{name: "pessimistic patch", constraint: "~> 3.3.0", want: "3.3.1"},
		{name: "range", constraint: ">= 3.2.0, < 3.4.0", want: "3.3.1"},
		{name: "exact as constraint", constraint: "3.2.0", want: "3.2.0"},
		{name: "prerelease excluded", constraint: ">= 4.0.0", want: "4.0.0"},
		{name: "prerelease opt-in", constraint: ">= 4.1.0-rc1", want: "4.1.0-rc1"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			server := newRegistryTestServer(t)
			source := "tfr://" + server.Listener.Addr().String() + "/terraform-aws-modules/vpc/aws"

			pinned, err := getter.PinModuleVersion(
				t.Context(), logger.CreateLogger(), server.Client(), tfimpl.OpenTofu, source, tc.constraint,
			)
			require.NoError(t, err)
			assert.Equal(t, source+"?version="+tc.want, pinned)
		})
	}
}

func TestSourceHasVersionConstraint(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name   string
		source string
		want   bool
	}{
		{
			name:   "exact pin",
			source: "tfr://registry.opentofu.org/terraform-aws-modules/vpc/aws?version=3.3.0",
			want:   false,
		},
		{
			name:   "constraint",
			source: "tfr://registry.opentofu.org/terraform-aws-modules/vpc/aws?version=~> 3.3",
			want:   true,
		},
		{
			name:   "no version query",
			source: "tfr://registry.opentofu.org/terraform-aws-modules/vpc/aws",
			want:   false,
		},
		{
			name:   "non-tfr source",
			source: "git::https://github.com/foo/bar.git?ref=v1.0.0",
			want:   false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tc.want, getter.SourceHasVersionConstraint(tc.source))
		})
	}
}

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

	header, err := getter.GetTerraformGetHeader(
		t.Context(),
		logger.CreateLogger(),
		server.Client(),
		&moduleURL,
	)
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

func TestGetLatestModuleVersion(t *testing.T) {
	t.Parallel()

	server := newRegistryTestServer(t)

	latestVersion, err := getter.GetLatestModuleVersion(
		t.Context(), logger.CreateLogger(), server.Client(),
		server.Listener.Addr().String(), "/v1/modules/", "terraform-aws-modules/vpc/aws",
	)
	require.NoError(t, err)
	assert.Equal(t, "4.0.0", latestVersion)
}

// TestGetLatestModuleVersionSkipsPrereleases pins the behavior of the
// resolver when a registry has prerelease versions that sort above the
// latest stable: prereleases are excluded so the resolver matches the
// default unconstrained-resolution semantics of OpenTofu and Terraform.
func TestGetLatestModuleVersionSkipsPrereleases(t *testing.T) {
	t.Parallel()

	server := newVersionsTestServer(
		t,
		`{"modules":[{"versions":[{"version":"3.3.0"},{"version":"4.0.0-rc1"},{"version":"2.0.0"}]}]}`,
	)

	latest, err := getter.GetLatestModuleVersion(
		t.Context(), logger.CreateLogger(), server.Client(),
		server.Listener.Addr().String(), "/v1/modules/", "foo/bar/baz",
	)
	require.NoError(t, err)
	assert.Equal(t, "3.3.0", latest)
}

// TestGetLatestModuleVersionAllPrereleases verifies that when only
// prerelease versions are published, the resolver errors out instead of
// silently returning a prerelease. Users are expected to pin a version
// explicitly in that case.
func TestGetLatestModuleVersionAllPrereleases(t *testing.T) {
	t.Parallel()

	server := newVersionsTestServer(
		t,
		`{"modules":[{"versions":[{"version":"1.0.0-alpha"},{"version":"2.0.0-rc1"}]}]}`,
	)

	_, err := getter.GetLatestModuleVersion(
		t.Context(), logger.CreateLogger(), server.Client(),
		server.Listener.Addr().String(), "/v1/modules/", "foo/bar/baz",
	)
	require.Error(t, err)
}

// TestGetLatestModuleVersionSkipsUnparsable pins the behavior that
// unparsable version entries are silently skipped (with a debug log) so
// a single bad row in the registry response cannot block resolution.
func TestGetLatestModuleVersionSkipsUnparsable(t *testing.T) {
	t.Parallel()

	server := newVersionsTestServer(
		t,
		`{"modules":[{"versions":[{"version":"not-a-version"},{"version":"1.0.0"}]}]}`,
	)

	latest, err := getter.GetLatestModuleVersion(
		t.Context(), logger.CreateLogger(), server.Client(),
		server.Listener.Addr().String(), "/v1/modules/", "foo/bar/baz",
	)
	require.NoError(t, err)
	assert.Equal(t, "1.0.0", latest)
}

// matchVersionsBody is a list-versions response spanning several minor and
// patch lines plus a prerelease, letting the constraint resolver tests exercise
// pessimistic, range, exact and prerelease-exclusion semantics off one fixture.
const matchVersionsBody = `{"modules":[{"versions":[` +
	`{"version":"1.0.0"},{"version":"1.5.0"},{"version":"1.9.0"},` +
	`{"version":"2.0.0"},{"version":"3.3.0"},{"version":"3.3.5"},` +
	`{"version":"3.4.0"},{"version":"4.0.0-rc1"}]}]}`

func TestGetMatchingModuleVersion(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name       string
		constraint string
		want       string
	}{
		{name: "pessimistic patch", constraint: "~> 3.3.0", want: "3.3.5"},
		{name: "pessimistic minor", constraint: "~> 3.3", want: "3.4.0"},
		{name: "range", constraint: ">= 1.0.0, < 2.0.0", want: "1.9.0"},
		{name: "exact as constraint", constraint: "2.0.0", want: "2.0.0"},
		{name: "prerelease excluded", constraint: ">= 3.0.0", want: "3.4.0"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			server := newVersionsTestServer(t, matchVersionsBody)

			got, err := getter.GetMatchingModuleVersion(
				t.Context(), logger.CreateLogger(), server.Client(),
				server.Listener.Addr().String(), "/v1/modules/", "foo/bar/baz", tc.constraint,
			)
			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
}

// TestGetMatchingModuleVersionPrereleaseOptIn pins the opt-in half of the
// prerelease policy: a constraint naming a prerelease admits prerelease
// versions sharing its base, so >= 4.0.0-rc1 resolves to 4.0.0-rc1 even though
// the only other version sorts lower.
func TestGetMatchingModuleVersionPrereleaseOptIn(t *testing.T) {
	t.Parallel()

	server := newVersionsTestServer(
		t,
		`{"modules":[{"versions":[{"version":"3.4.0"},{"version":"4.0.0-rc1"}]}]}`,
	)

	got, err := getter.GetMatchingModuleVersion(
		t.Context(), logger.CreateLogger(), server.Client(),
		server.Listener.Addr().String(), "/v1/modules/", "foo/bar/baz", ">= 4.0.0-rc1",
	)
	require.NoError(t, err)
	assert.Equal(t, "4.0.0-rc1", got)
}

// TestGetMatchingModuleVersionNoMatch pins the typed error returned when the
// registry publishes versions but none satisfy the constraint.
func TestGetMatchingModuleVersionNoMatch(t *testing.T) {
	t.Parallel()

	server := newVersionsTestServer(
		t,
		`{"modules":[{"versions":[{"version":"1.0.0"},{"version":"2.0.0"}]}]}`,
	)

	_, err := getter.GetMatchingModuleVersion(
		t.Context(), logger.CreateLogger(), server.Client(),
		server.Listener.Addr().String(), "/v1/modules/", "foo/bar/baz", ">= 9.0.0",
	)
	require.Error(t, err)

	var typed getter.NoMatchingVersionErr

	require.ErrorAs(t, err, &typed)
}

// TestGetMatchingModuleVersionUnparsableConstraint pins the typed error
// returned when the constraint itself does not parse, before any registry call.
func TestGetMatchingModuleVersionUnparsableConstraint(t *testing.T) {
	t.Parallel()

	server := newVersionsTestServer(t, `{"modules":[{"versions":[{"version":"1.0.0"}]}]}`)

	_, err := getter.GetMatchingModuleVersion(
		t.Context(), logger.CreateLogger(), server.Client(),
		server.Listener.Addr().String(), "/v1/modules/", "foo/bar/baz", "not a constraint",
	)
	require.Error(t, err)

	var typed getter.ConstraintParseErr

	require.ErrorAs(t, err, &typed)
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

	// Serve the list-versions endpoint with a realistic spread of minor and
	// patch lines plus a prerelease, so one server exercises latest-version
	// resolution (TestRegistryGetterWithoutVersion) and every constraint case
	// (TestPinModuleVersion).
	mux.HandleFunc(
		"/v1/modules/terraform-aws-modules/vpc/aws/versions",
		func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, err := w.Write(
				[]byte(
					`{"modules":[{"versions":[` +
						`{"version":"3.2.0"},{"version":"3.3.0"},{"version":"3.3.1"},` +
						`{"version":"3.4.0"},{"version":"4.0.0"},{"version":"4.1.0-rc1"}]}]}`,
				),
			)
			assert.NoError(t, err)
		},
	)

	mux.HandleFunc(
		"/v1/modules/terraform-aws-modules/vpc/aws/{version}/download",
		func(w http.ResponseWriter, r *http.Request) {
			// Resolve against the request host so the downloader hits the same
			// test server we are about to shut down at end-of-test.
			w.Header().Set("X-Terraform-Get", "https://"+r.Host+"/download/terraform-aws-vpc.zip")
			w.WriteHeader(http.StatusNoContent)
		},
	)

	mux.HandleFunc("/download/terraform-aws-vpc.zip", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/zip")

		_, err := w.Write(zipBody)
		assert.NoError(t, err)
	})

	server := httptest.NewTLSServer(mux)
	t.Cleanup(server.Close)

	return server
}

// newVersionsTestServer stands up a TLS test server that responds to the
// module list-versions endpoint with the supplied JSON body. Used by the
// GetLatestModuleVersion tests that exercise prerelease filtering and the
// unparsable-version skip path.
func newVersionsTestServer(t *testing.T, body string) *httptest.Server {
	t.Helper()

	mux := http.NewServeMux()
	mux.HandleFunc(
		"/v1/modules/foo/bar/baz/versions",
		func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, err := w.Write([]byte(body))
			assert.NoError(t, err)
		},
	)

	server := httptest.NewTLSServer(mux)
	t.Cleanup(server.Close)

	return server
}
