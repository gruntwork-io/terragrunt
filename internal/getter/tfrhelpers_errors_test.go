package getter_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/getter"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestRegistryErrorTypes pins the user-facing strings on the typed errors
// the registry getter returns. Callers don't switch on these via errors.As,
// but the strings appear in CLI output, so they're part of the contract.
func TestRegistryErrorTypes(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name   string
		err    error
		expect string
	}{
		{
			name:   "MalformedRegistryURLErr",
			err:    getter.MalformedRegistryURLErr{},
			expect: "tfr getter URL is malformed: ",
		},
		{
			name:   "ServiceDiscoveryErr",
			err:    getter.ServiceDiscoveryErr{},
			expect: "Error identifying module registry API location: ",
		},
		{
			name:   "ModuleDownloadErr",
			err:    getter.ModuleDownloadErr{},
			expect: "Error downloading module from : ",
		},
		{
			name:   "RegistryAPIErr",
			err:    getter.RegistryAPIErr{},
			expect: "Failed to fetch url : status code 0",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tc.expect, tc.err.Error())
		})
	}
}

// TestGetTerraformGetHeaderFallsBackToBodyLocation pins the fallback path
// where the registry omits the X-Terraform-Get header and embeds the
// download URL in a JSON body location field instead.
func TestGetTerraformGetHeaderFallsBackToBodyLocation(t *testing.T) {
	t.Parallel()

	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, err := w.Write([]byte(`{"location":"https://example.com/foo.zip"}`))
		assert.NoError(t, err)
	}))
	t.Cleanup(server.Close)

	got, err := getter.GetTerraformGetHeader(t.Context(), logger.CreateLogger(), server.Client(), map[string]string{}, parseURL(t, server.URL))
	require.NoError(t, err)
	assert.Equal(t, "https://example.com/foo.zip", got)
}

// TestGetTerraformGetHeaderMissing pins the typed error returned when neither
// the header nor the body carry the download URL.
func TestGetTerraformGetHeaderMissing(t *testing.T) {
	t.Parallel()

	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, err := w.Write([]byte(`{}`))
		assert.NoError(t, err)
	}))
	t.Cleanup(server.Close)

	_, err := getter.GetTerraformGetHeader(t.Context(), logger.CreateLogger(), server.Client(), map[string]string{}, parseURL(t, server.URL))
	require.Error(t, err)

	var typed getter.ModuleDownloadErr

	require.ErrorAs(t, err, &typed)
}

// TestGetModuleRegistryURLBasePathMissingModulesV1 pins the typed error
// returned when service discovery omits modules.v1.
func TestGetModuleRegistryURLBasePathMissingModulesV1(t *testing.T) {
	t.Parallel()

	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, err := w.Write([]byte(`{}`))
		assert.NoError(t, err)
	}))
	t.Cleanup(server.Close)

	_, err := getter.GetModuleRegistryURLBasePath(t.Context(), logger.CreateLogger(), server.Client(), map[string]string{}, addrFromURL(t, server.URL))
	require.Error(t, err)

	var typed getter.ServiceDiscoveryErr

	require.ErrorAs(t, err, &typed)
}

// TestHTTPGETAndGetResponseNonOK pins the typed RegistryAPIErr that all
// non-2xx responses bubble up as.
func TestHTTPGETAndGetResponseNonOK(t *testing.T) {
	t.Parallel()

	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	t.Cleanup(server.Close)

	_, err := getter.GetModuleRegistryURLBasePath(t.Context(), logger.CreateLogger(), server.Client(), map[string]string{}, addrFromURL(t, server.URL))
	require.Error(t, err)

	var typed getter.RegistryAPIErr

	require.ErrorAs(t, err, &typed)
}

// TestApplyHostTokenViaEnv pins the env-var fallback path for registry auth.
// When the OpenTofu/Terraform CLI config doesn't carry credentials for the
// host, TG_TF_REGISTRY_TOKEN supplied via the venv-mediated env map is sent
// as a bearer token.
func TestApplyHostTokenViaEnv(t *testing.T) {
	t.Parallel()

	const want = "Bearer my-test-token"

	env := map[string]string{"TG_TF_REGISTRY_TOKEN": "my-test-token"}

	var got string

	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.Header.Get("Authorization")

		w.Header().Set("Content-Type", "application/json")

		_, err := w.Write([]byte(`{"modules.v1":"/v1/modules/"}`))
		assert.NoError(t, err)
	}))
	t.Cleanup(server.Close)

	_, err := getter.GetModuleRegistryURLBasePath(t.Context(), logger.CreateLogger(), server.Client(), env, addrFromURL(t, server.URL))
	require.NoError(t, err)
	assert.Equal(t, want, got)
}

// TestHTTPGETAndGetResponseRespectsContextCancellation guards the request
// against a cancelled context so callers can abort long registry calls.
func TestHTTPGETAndGetResponseRespectsContextCancellation(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(server.Close)

	_, err := getter.GetModuleRegistryURLBasePath(ctx, logger.CreateLogger(), server.Client(), map[string]string{}, addrFromURL(t, server.URL))
	require.Error(t, err)
	assert.ErrorIs(t, err, context.Canceled)
}

func parseURL(t *testing.T, raw string) *url.URL {
	t.Helper()

	u, err := url.Parse(raw)
	require.NoError(t, err)

	return u
}

func addrFromURL(t *testing.T, raw string) string {
	t.Helper()

	u, err := url.Parse(raw)
	require.NoError(t, err)

	return u.Host
}
