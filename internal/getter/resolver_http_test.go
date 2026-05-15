package getter_test

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/cas"
	"github.com/gruntwork-io/terragrunt/internal/getter"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHTTPResolver_PrefersStrongETag(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodHead, r.Method)
		w.Header().Set("ETag", `"abc123"`)
		w.Header().Set("Last-Modified", "Mon, 01 Jan 2024 00:00:00 GMT")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	r := getter.NewHTTPResolver()
	url := srv.URL + "/mod.tgz"

	key, err := r.Probe(t.Context(), url)
	require.NoError(t, err)
	assert.Equal(t, cas.OpaqueKey("http", url, "abc123"), key)
}

func TestHTTPResolver_StripsWeakETagPrefix(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("ETag", `W/"weak-tag"`)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	r := getter.NewHTTPResolver()
	url := srv.URL + "/mod.tgz"

	key, err := r.Probe(t.Context(), url)
	require.NoError(t, err)
	assert.Equal(t, cas.OpaqueKey("http", url, "weak-tag"), key)
}

func TestHTTPResolver_FallsBackToLastModified(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Last-Modified", "Mon, 01 Jan 2024 00:00:00 GMT")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	r := getter.NewHTTPResolver()
	url := srv.URL + "/mod.tgz"

	key, err := r.Probe(t.Context(), url)
	require.NoError(t, err)
	assert.Equal(t, cas.OpaqueKey("http", url, "Mon, 01 Jan 2024 00:00:00 GMT"), key)
}

func TestHTTPResolver_ReturnsErrNoVersionMetadata(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	r := getter.NewHTTPResolver()

	_, err := r.Probe(t.Context(), srv.URL+"/mod.tgz")
	require.ErrorIs(t, err, cas.ErrNoVersionMetadata)
}

func TestHTTPResolver_ReturnsErrOnNon2xx(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	r := getter.NewHTTPResolver()

	_, err := r.Probe(t.Context(), srv.URL+"/mod.tgz")
	require.ErrorIs(t, err, cas.ErrNoVersionMetadata)
}

// TestHTTPResolver_LowercaseWeakETag pins that both `W/` and `w/`
// weak-validator prefixes normalize to the same key, since some
// servers emit the lowercase form despite the RFC specifying upper.
func TestHTTPResolver_LowercaseWeakETag(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("ETag", `w/"weak-tag"`)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	r := getter.NewHTTPResolver()
	url := srv.URL + "/mod.tgz"

	key, err := r.Probe(t.Context(), url)
	require.NoError(t, err)
	assert.Equal(t, cas.OpaqueKey("http", url, "weak-tag"), key)
}

// TestHTTPResolver_SchemeReportsRegisteredScheme pins the
// SourceResolver contract: the resolver registered under "https" in
// DefaultSourceResolvers must report "https" from Scheme(), not "http".
func TestHTTPResolver_SchemeReportsRegisteredScheme(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "http", getter.NewHTTPResolver().Scheme())
	assert.Equal(t, "https", getter.NewHTTPSResolver().Scheme())
}

// TestHTTPResolver_StripsOuterClientMagicParams pins that probes for
// the same URL with and without the outer-client magic params share
// a cache key, and that the params do not reach the wire on HEAD.
func TestHTTPResolver_StripsOuterClientMagicParams(t *testing.T) {
	t.Parallel()

	var seenQueries []string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seenQueries = append(seenQueries, r.URL.RawQuery)

		w.Header().Set("ETag", `"abc123"`)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	r := getter.NewHTTPResolver()

	plain := srv.URL + "/mod.tgz"
	plainKey, err := r.Probe(t.Context(), plain)
	require.NoError(t, err)

	withMagic := srv.URL + "/mod.tgz?archive=zip&checksum=sha256:deadbeef&filename=override.tgz"
	magicKey, err := r.Probe(t.Context(), withMagic)
	require.NoError(t, err)

	assert.Equal(t, plainKey, magicKey,
		"magic params must not split the cache key; the outer client strips them before fetch")

	require.Len(t, seenQueries, 2)
	assert.Empty(t, seenQueries[0], "first probe has no query")
	assert.Empty(t, seenQueries[1], "magic params must be stripped before the HEAD request")
}

// TestHTTPResolver_PreservesNonMagicQueryParams pins that stripping
// is scoped to the magic-param allowlist; caller-supplied params
// (auth tokens, server-honored selectors) must reach HEAD.
func TestHTTPResolver_PreservesNonMagicQueryParams(t *testing.T) {
	t.Parallel()

	var seenQueries []string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seenQueries = append(seenQueries, r.URL.RawQuery)

		w.Header().Set("ETag", `"abc123"`)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	r := getter.NewHTTPResolver()

	withCustom := srv.URL + "/mod.tgz?token=secret&v=2"
	_, err := r.Probe(t.Context(), withCustom)
	require.NoError(t, err)

	require.Len(t, seenQueries, 1)
	q, parseErr := url.ParseQuery(seenQueries[0])
	require.NoError(t, parseErr)
	assert.Equal(t, "secret", q.Get("token"))
	assert.Equal(t, "2", q.Get("v"))
}
