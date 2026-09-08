package getter_test

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/cas"
	"github.com/gruntwork-io/terragrunt/internal/getter"
	"github.com/gruntwork-io/terragrunt/internal/tfimpl"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/gruntwork-io/terragrunt/test/helpers/venvtest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTFRResolver_ProbeReturnsContentKey(t *testing.T) {
	t.Parallel()

	server := newRegistryTestServer(t)

	r := getter.NewTFRResolver().
		WithHTTPClient(server.Client()).
		WithLogger(logger.CreateLogger())

	src := "tfr://" + server.Listener.Addr().String() +
		"/terraform-aws-modules/vpc/aws?version=3.3.0"

	key, err := r.Probe(t.Context(), src)
	require.NoError(t, err)

	expected := cas.ContentKey(
		"tfr-xtg",
		"https://"+server.Listener.Addr().String()+"/download/terraform-aws-vpc.zip",
	)
	assert.Equal(t, expected, key)
}

// TestTFRResolver_ProbeIsStable pins that two probes of the same URL
// produce the same key, so the CAS hit path on a repeated run is
// deterministic.
func TestTFRResolver_ProbeIsStable(t *testing.T) {
	t.Parallel()

	server := newRegistryTestServer(t)

	r := getter.NewTFRResolver().
		WithHTTPClient(server.Client()).
		WithLogger(logger.CreateLogger())

	src := "tfr://" + server.Listener.Addr().String() +
		"/terraform-aws-modules/vpc/aws?version=3.3.0"

	first, err := r.Probe(t.Context(), src)
	require.NoError(t, err)

	second, err := r.Probe(t.Context(), src)
	require.NoError(t, err)

	assert.Equal(t, first, second)
}

// TestTFRResolver_SubdirCollapsesToSameKey pins that two tfr:// URLs
// that only differ in their //subdir selector share one CAS entry: the
// fetcher copies out the subdir, but the upstream archive is identical.
func TestTFRResolver_SubdirCollapsesToSameKey(t *testing.T) {
	t.Parallel()

	server := newRegistryTestServer(t)

	r := getter.NewTFRResolver().
		WithHTTPClient(server.Client()).
		WithLogger(logger.CreateLogger())

	base := "tfr://" + server.Listener.Addr().String() +
		"/terraform-aws-modules/vpc/aws"

	bare, err := r.Probe(t.Context(), base+"?version=3.3.0")
	require.NoError(t, err)

	withSubdir, err := r.Probe(t.Context(), base+"//modules/public?version=3.3.0")
	require.NoError(t, err)

	assert.Equal(t, bare, withSubdir)
}

func TestTFRResolver_MissingVersionReturnsErrNoVersionMetadata(t *testing.T) {
	t.Parallel()

	r := getter.NewTFRResolver().WithLogger(logger.CreateLogger())

	_, err := r.Probe(t.Context(), "tfr://registry.terraform.io/terraform-aws-modules/vpc/aws")
	require.ErrorIs(t, err, cas.ErrNoVersionMetadata)
}

func TestTFRResolver_EmptyVersionReturnsErrNoVersionMetadata(t *testing.T) {
	t.Parallel()

	r := getter.NewTFRResolver().WithLogger(logger.CreateLogger())

	_, err := r.Probe(
		t.Context(),
		"tfr://registry.terraform.io/terraform-aws-modules/vpc/aws?version=",
	)
	require.ErrorIs(t, err, cas.ErrNoVersionMetadata)
}

func TestTFRResolver_DiscoveryFailureReturnsErrNoVersionMetadata(t *testing.T) {
	t.Parallel()

	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(server.Close)

	r := getter.NewTFRResolver().
		WithHTTPClient(server.Client()).
		WithLogger(logger.CreateLogger())

	src := "tfr://" + server.Listener.Addr().String() +
		"/terraform-aws-modules/vpc/aws?version=3.3.0"

	_, err := r.Probe(t.Context(), src)
	require.ErrorIs(t, err, cas.ErrNoVersionMetadata)
}

func TestTFRResolver_SchemeReportsTFR(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "tfr", getter.NewTFRResolver().Scheme())
}

// TestTFRResolver_ProbeCredentialsFollowImplementation pins that the resolver's registry
// requests authenticate with the CLI config files of the implementation it was given, even
// though its RegistryAuth was constructed with the default OpenTofu implementation.
func TestTFRResolver_ProbeCredentialsFollowImplementation(t *testing.T) {
	t.Parallel()

	const home = "/virtual/home"

	// Every registry request must carry the ~/.terraformrc token; discovery and the
	// download-header request both go through the changed resolverAuth calls.
	server := newRegistryTestServerWithRequestHook(t, func(r *http.Request) {
		assert.Equal(t, "Bearer terraform-token", r.Header.Get("Authorization"))
	})

	serverURL, err := url.Parse(server.URL)
	require.NoError(t, err)

	v := venvtest.New().
		WithGOOS("linux").
		WithHTTP(server.Client()).
		WithUserHomeDir(func() (string, error) { return home, nil })

	require.NoError(t, v.FS.MkdirAll(home, 0o755))
	// A credential-less ~/.tofurc shadows ~/.terraformrc under OpenTofu's search order.
	require.NoError(t, vfs.WriteFile(v.FS, filepath.Join(home, ".tofurc"), []byte("\n"), 0o600))
	require.NoError(t, vfs.WriteFile(v.FS, filepath.Join(home, ".terraformrc"), []byte(`
credentials "`+serverURL.Hostname()+`" {
  token = "terraform-token"
}
`), 0o600))

	r := getter.NewTFRResolver().
		WithHTTPClient(server.Client()).
		WithLogger(logger.CreateLogger()).
		WithAuth(getter.NewRegistryAuth(v)).
		WithTofuImplementation(tfimpl.Terraform)

	src := "tfr://" + server.Listener.Addr().String() +
		"/terraform-aws-modules/vpc/aws?version=~> 3.0"

	_, err = r.Probe(t.Context(), src)
	require.NoError(t, err)
}
