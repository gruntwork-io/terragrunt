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

// TestPinModuleVersionBuildMetadata pins how a resolved version carrying
// semver build metadata is written back into the source. The `+` is
// percent-encoded so that re-parsing the pinned source yields the version the
// registry published, rather than the `+`-as-space decoding a query string
// would otherwise apply.
func TestPinModuleVersionBuildMetadata(t *testing.T) {
	t.Parallel()

	server := newVersionsTestServer(t, buildMetadataVersionsBody)
	source := "tfr://" + server.Listener.Addr().String() + "/foo/bar/baz"

	pinned, err := getter.PinModuleVersion(
		t.Context(), logger.CreateLogger(), server.Client(), tfimpl.OpenTofu, source, "~> 1.8.24",
	)
	require.NoError(t, err)
	assert.Equal(t, source+"?version=1.8.26%2Bcss9.10.001", pinned)

	pinnedURL, err := url.Parse(pinned)
	require.NoError(t, err)
	assert.Equal(t, "1.8.26+css9.10.001", pinnedURL.Query().Get("version"))
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
		{
			name:   "exact pin with build metadata",
			source: "tfr://registry.opentofu.org/cloudstoragesec/cloud-storage-security/aws?version=1.8.26%2Bcss9.10.001",
			want:   false,
		},
		{
			// A literal `+` in a query decodes to a space, so this hand-written
			// pin reaches the parser as "1.8.26 css9.10.001" and fails to parse
			// as a version. Terragrunt rejects the source rather than resolving
			// the wrong module version. Encode the `+` as %2B, or express the
			// version through the terraform block's version attribute.
			name:   "unencoded build metadata pin",
			source: "tfr://registry.opentofu.org/cloudstoragesec/cloud-storage-security/aws?version=1.8.26+css9.10.001",
			want:   true,
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

// TestBuildRequestURLBuildMetadata pins that a version carrying build metadata
// keeps its `+` in the download path. A `+` is a literal in a path segment, and
// the OpenTofu registry serves the endpoint either way.
func TestBuildRequestURLBuildMetadata(t *testing.T) {
	t.Parallel()

	requestURL, err := getter.BuildRequestURL(
		"registry.opentofu.org",
		"/v1/modules/",
		"/cloudstoragesec/cloud-storage-security/aws",
		"1.8.26+css9.10.001",
	)
	require.NoError(t, err)
	assert.Equal(t,
		"https://registry.opentofu.org/v1/modules/cloudstoragesec/cloud-storage-security/aws/1.8.26+css9.10.001/download",
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

// buildMetadataVersionsBody mirrors the list-versions response of
// cloudstoragesec/cloud-storage-security/aws, a module whose every published
// version carries semver build metadata. Requesting such a module without the
// metadata is not an option: the registry serves 1.8.26+css9.10.001 and 404s
// on 1.8.26.
const buildMetadataVersionsBody = `{"modules":[{"versions":[` +
	`{"version":"1.8.26+css9.10.001"},{"version":"1.8.25+css9.10.000"},` +
	`{"version":"1.8.24+css9.09.002"}]}]}`

// TestGetLatestModuleVersionBuildMetadata pins that the latest-version path
// preserves build metadata, so a bare tfr:// source pointed at such a module
// requests a version the registry actually publishes.
func TestGetLatestModuleVersionBuildMetadata(t *testing.T) {
	t.Parallel()

	server := newVersionsTestServer(t, buildMetadataVersionsBody)

	latest, err := getter.GetLatestModuleVersion(
		t.Context(), logger.CreateLogger(), server.Client(),
		server.Listener.Addr().String(), "/v1/modules/", "foo/bar/baz",
	)
	require.NoError(t, err)
	assert.Equal(t, "1.8.26+css9.10.001", latest)
}

// TestGetMatchingModuleVersionBuildMetadata pins constraint resolution against
// versions carrying build metadata. Metadata does not affect precedence, so a
// constraint written without it still selects the published version, and the
// metadata survives into the result.
func TestGetMatchingModuleVersionBuildMetadata(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name       string
		constraint string
		want       string
	}{
		{name: "exact without metadata", constraint: "1.8.25", want: "1.8.25+css9.10.000"},
		{
			name:       "exact with metadata",
			constraint: "1.8.25+css9.10.000",
			want:       "1.8.25+css9.10.000",
		},
		{name: "pessimistic patch", constraint: "~> 1.8.24", want: "1.8.26+css9.10.001"},
		{name: "range", constraint: ">= 1.8.24, < 1.8.26", want: "1.8.25+css9.10.000"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			server := newVersionsTestServer(t, buildMetadataVersionsBody)

			got, err := getter.GetMatchingModuleVersion(
				t.Context(), logger.CreateLogger(), server.Client(),
				server.Listener.Addr().String(), "/v1/modules/", "foo/bar/baz", tc.constraint,
			)
			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
}

// TestGetMatchingModuleVersionPrereleaseBuildMetadata pins the combination of
// both suffixes, modeled on waveaccounting/chatbot-slack-configuration/aws,
// which publishes prereleases tagged with a commit SHA as build metadata. The
// prerelease opt-in rule reads the constraint's prerelease and ignores its
// metadata, so an alpha pinned without the SHA resolves to the published tag.
func TestGetMatchingModuleVersionPrereleaseBuildMetadata(t *testing.T) {
	t.Parallel()

	server := newVersionsTestServer(
		t,
		`{"modules":[{"versions":[{"version":"1.1.0"},{"version":"1.1.0-alpha.3"},`+
			`{"version":"1.1.0-alpha.2+a88b844"},{"version":"1.0.0"},`+
			`{"version":"1.0.0-alpha.3+46e44c9"}]}]}`,
	)

	got, err := getter.GetMatchingModuleVersion(
		t.Context(), logger.CreateLogger(), server.Client(),
		server.Listener.Addr().String(), "/v1/modules/", "foo/bar/baz", "1.0.0-alpha.3",
	)
	require.NoError(t, err)
	assert.Equal(t, "1.0.0-alpha.3+46e44c9", got)
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
// module list-versions endpoint with the supplied JSON body, plus the service
// discovery endpoint so callers that resolve a source URL end to end can use it
// too. Used by the GetLatestModuleVersion tests that exercise prerelease
// filtering and the unparsable-version skip path.
func newVersionsTestServer(t *testing.T, body string) *httptest.Server {
	t.Helper()

	mux := http.NewServeMux()
	mux.HandleFunc("/.well-known/terraform.json", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, err := w.Write([]byte(`{"modules.v1":"/v1/modules/"}`))
		assert.NoError(t, err)
	})
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
