package getter_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/cas"
	"github.com/gruntwork-io/terragrunt/internal/getter"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOCIResolver_SchemeReturnsOCI(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "oci", getter.NewOCIResolver().Scheme())
}

// TestOCIResolver_ProbeReturnsDigestKey pins that Probe resolves a tag
// reference to an immutable OCI digest and wraps it as a CAS content key.
func TestOCIResolver_ProbeReturnsDigestKey(t *testing.T) {
	t.Parallel()

	srv, state := newOCITestServer(t, map[string]string{testMainTF: "ok"})

	ociRef := srv.Listener.Addr().String() + "/org/module:v1.0.0"
	key, err := getter.NewOCIResolver().Probe(t.Context(), "oci://"+ociRef)
	require.NoError(t, err)

	// cas.ContentKey hashes its inputs — verify the key matches the expected hash
	// of ref@digest so that a re-push with a different digest busts the cache.
	expectedKey := cas.ContentKey("oci", ociRef+"@"+state.manifestDigest)
	assert.Equal(t, expectedKey, key,
		"CAS key must be ContentKey(\"oci\", ref@manifestDigest)")
}

// TestOCIResolver_ProbeIsStable pins that two Probe calls for the same tag
// return the same key, enabling a deterministic CAS hit on the second run.
func TestOCIResolver_ProbeIsStable(t *testing.T) {
	t.Parallel()

	srv, _ := newOCITestServer(t, map[string]string{testMainTF: "ok"})

	ref := "oci://" + srv.Listener.Addr().String() + "/org/module:v1.0.0"
	r := getter.NewOCIResolver()

	first, err := r.Probe(t.Context(), ref)
	require.NoError(t, err)

	second, err := r.Probe(t.Context(), ref)
	require.NoError(t, err)

	assert.Equal(t, first, second)
}

// TestOCIResolver_SubdirCollapsesToSameKey pins that two oci:// URLs that
// differ only in their //subdir selector produce the same probe key: the
// underlying artifact is identical regardless of which subdir is extracted.
func TestOCIResolver_SubdirCollapsesToSameKey(t *testing.T) {
	t.Parallel()

	srv, _ := newOCITestServer(t, map[string]string{
		"modules/vpc/main.tf":   "# vpc",
		"modules/other/main.tf": "# other",
	})

	base := "oci://" + srv.Listener.Addr().String() + "/org/modules:v1.0.0"
	r := getter.NewOCIResolver()

	bareKey, err := r.Probe(t.Context(), base)
	require.NoError(t, err)

	subdirKey, err := r.Probe(t.Context(), base+"//modules/vpc")
	require.NoError(t, err)

	assert.Equal(t, bareKey, subdirKey,
		"URLs that differ only in //subdir must share one CAS entry")
}

// TestOCIResolver_RegistryErrorReturnsErrNoVersionMetadata pins that a
// registry failure degrades gracefully: CAS falls back to download+hash.
func TestOCIResolver_RegistryErrorReturnsErrNoVersionMetadata(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(srv.Close)

	ref := "oci://" + srv.Listener.Addr().String() + "/org/module:v1.0.0"

	_, err := getter.NewOCIResolver().Probe(t.Context(), ref)
	require.ErrorIs(t, err, cas.ErrNoVersionMetadata)
}

// TestOCIResolver_BadURLReturnsErrNoVersionMetadata verifies that an
// unparseable source URL does not panic and returns ErrNoVersionMetadata.
func TestOCIResolver_BadURLReturnsErrNoVersionMetadata(t *testing.T) {
	t.Parallel()

	_, err := getter.NewOCIResolver().Probe(t.Context(), "://not a url")
	require.ErrorIs(t, err, cas.ErrNoVersionMetadata)
}

// TestOCIResolver_WrongSchemeReturnsErrNoVersionMetadata verifies that the
// resolver declines non-oci:// URLs.
func TestOCIResolver_WrongSchemeReturnsErrNoVersionMetadata(t *testing.T) {
	t.Parallel()

	cases := []string{
		"https://ghcr.io/org/module:v1.0.0",
		"tfr://registry.terraform.io/org/module/aws?version=1.0.0",
		"git::https://github.com/org/repo.git",
	}

	r := getter.NewOCIResolver()

	for _, src := range cases {
		t.Run(src, func(t *testing.T) {
			t.Parallel()

			_, err := r.Probe(t.Context(), src)
			require.ErrorIs(t, err, cas.ErrNoVersionMetadata)
		})
	}
}
