package getter_test

import (
	"context"
	"io"
	"sync"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/cas"
	"github.com/gruntwork-io/terragrunt/internal/getter"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/gruntwork-io/terragrunt/test/helpers/venvtest"
	"github.com/opencontainers/go-digest"
	ociv1 "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOCIResolverScheme(t *testing.T) {
	t.Parallel()

	assert.Equal(t, getter.SchemeOCI, getter.NewOCIResolver(nil).Scheme())
}

// TestOCIResolverProbeDigestPinSkipsResolve: a digest pin is the key, no registry roundtrip.
func TestOCIResolverProbeDigestPinSkipsResolve(t *testing.T) {
	t.Parallel()

	pinned := digest.FromString("pinned-manifest").String()
	r := getter.NewOCIResolver(failingNewStore())

	key, err := r.Probe(t.Context(), "oci://127.0.0.1:5000/terraform-modules/vpc?digest="+pinned)
	require.NoError(t, err, "a digest pin must not consult the store")
	assert.Equal(t, cas.ContentKey("oci-manifest", pinned), key)
}

// TestOCIResolverProbeTagResolvesEveryProbe: tags are mutable, so every probe re-resolves.
func TestOCIResolverProbeTagResolvesEveryProbe(t *testing.T) {
	t.Parallel()

	store := resolverFakeStore(t)
	r := getter.NewOCIResolver(staticStore(store))

	first, err := r.Probe(t.Context(), "oci://127.0.0.1:5000/terraform-modules/vpc?tag=1.0.0")
	require.NoError(t, err)

	second, err := r.Probe(t.Context(), "oci://127.0.0.1:5000/terraform-modules/vpc?tag=1.0.0")
	require.NoError(t, err)

	assert.Equal(t, first, second, "an unchanged tag must produce the same key")
	assert.Equal(t, []string{"1.0.0", "1.0.0"}, store.gotRefs, "every probe must re-resolve the tag")
}

// TestOCIResolverProbeMovedTagChangesKey: a re-pushed tag must miss the old cache entry.
func TestOCIResolverProbeMovedTagChangesKey(t *testing.T) {
	t.Parallel()

	store := resolverFakeStore(t)
	r := getter.NewOCIResolver(staticStore(store))

	before, err := r.Probe(t.Context(), "oci://127.0.0.1:5000/terraform-modules/vpc?tag=1.0.0")
	require.NoError(t, err)

	store.manifestDesc.Digest = digest.FromString("re-pushed-manifest")

	after, err := r.Probe(t.Context(), "oci://127.0.0.1:5000/terraform-modules/vpc?tag=1.0.0")
	require.NoError(t, err)

	assert.NotEqual(t, before, after, "a moved tag must produce a new key")
}

// TestOCIResolverProbeStripsSubdir: subdir selectors must not split the cache entry.
func TestOCIResolverProbeStripsSubdir(t *testing.T) {
	t.Parallel()

	store := resolverFakeStore(t)
	r := getter.NewOCIResolver(staticStore(store))

	whole, err := r.Probe(t.Context(), "oci://127.0.0.1:5000/terraform-modules/vpc?tag=1.0.0")
	require.NoError(t, err)

	subdir, err := r.Probe(t.Context(), "oci://127.0.0.1:5000/terraform-modules/vpc//subdir?tag=1.0.0")
	require.NoError(t, err)

	assert.Equal(t, whole, subdir)
}

// TestOCIResolverProbeTagAndDigestShareKey: a tag resolving to a digest shares its entry.
func TestOCIResolverProbeTagAndDigestShareKey(t *testing.T) {
	t.Parallel()

	store := resolverFakeStore(t)
	r := getter.NewOCIResolver(staticStore(store))

	viaTag, err := r.Probe(t.Context(), "oci://127.0.0.1:5000/terraform-modules/vpc?tag=1.0.0")
	require.NoError(t, err)

	viaDigest, err := r.Probe(
		t.Context(),
		"oci://127.0.0.1:5000/terraform-modules/vpc?digest="+store.manifestDesc.Digest.String(),
	)
	require.NoError(t, err)

	assert.Equal(t, viaTag, viaDigest)
}

func TestOCIResolverProbeErrors(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		newStore getter.OCINewStoreFunc
		name     string
		rawURL   string
	}{
		{name: "wrong scheme", rawURL: "tfr://registry.example.com/module?version=1.0.0"},
		{name: "unparseable url", rawURL: "oci://bad\x00url"},
		{name: "missing registry domain", rawURL: "oci:///terraform-modules/vpc?tag=1.0.0"},
		{name: "missing repository name", rawURL: "oci://127.0.0.1:5000?tag=1.0.0"},
		{name: "unsupported query parameter", rawURL: "oci://127.0.0.1:5000/vpc?tag=1.0.0&foo=bar"},
		{name: "invalid digest value", rawURL: "oci://127.0.0.1:5000/vpc?digest=not-a-digest"},
		{name: "nil store seam", rawURL: "oci://127.0.0.1:5000/vpc?tag=1.0.0"},
		{
			name:     "store construction failure",
			rawURL:   "oci://127.0.0.1:5000/vpc?tag=1.0.0",
			newStore: failingNewStore(),
		},
		{
			name:     "resolve failure",
			rawURL:   "oci://127.0.0.1:5000/vpc?tag=1.0.0",
			newStore: failingResolveStoreFunc(),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			r := getter.NewOCIResolver(tc.newStore)

			_, err := r.Probe(t.Context(), tc.rawURL)
			require.ErrorIs(t, err, cas.ErrNoVersionMetadata)
		})
	}
}

// TestOCIResolverProbeConcurrentWithRacing: concurrent probes of the same and
// different refs must be race-free and key-stable.
func TestOCIResolverProbeConcurrentWithRacing(t *testing.T) {
	t.Parallel()

	store := resolverFakeStore(t)
	r := getter.NewOCIResolver(staticStore(store))
	pinned := store.manifestDesc.Digest.String()

	const probesPerShape = 8

	keys := make(chan string, 3*probesPerShape)

	var wg sync.WaitGroup

	for range probesPerShape {
		for _, rawURL := range []string{
			"oci://127.0.0.1:5000/terraform-modules/vpc?tag=1.0.0",
			"oci://127.0.0.1:5000/terraform-modules/vpc//subdir?tag=1.0.0",
			"oci://127.0.0.1:5000/terraform-modules/vpc?digest=" + pinned,
		} {
			wg.Add(1)

			go func() {
				defer wg.Done()

				key, err := r.Probe(t.Context(), rawURL)
				assert.NoError(t, err)

				keys <- key
			}()
		}
	}

	wg.Wait()
	close(keys)

	want := cas.ContentKey("oci-manifest", pinned)
	for key := range keys {
		assert.Equal(t, want, key, "every probe shape must map to the manifest digest key")
	}

	assert.Len(t, store.gotRefs, 2*probesPerShape, "every tag probe must resolve, digest probes must not")
}

func TestDefaultSourceResolversOCIConfig(t *testing.T) {
	t.Parallel()

	_, found := getter.DefaultSourceResolvers()[getter.SchemeOCI]
	assert.False(t, found, "oci resolver must be absent without WithOCIConfig")

	resolvers := getter.DefaultSourceResolvers(getter.WithOCIConfig(logger.CreateLogger(), venvtest.New()))

	r, found := resolvers[getter.SchemeOCI]
	require.True(t, found, "oci resolver must be present with WithOCIConfig")
	assert.Equal(t, getter.SchemeOCI, r.Scheme())
}

// resolverFakeStore builds a fake store carrying only a resolvable manifest descriptor.
func resolverFakeStore(t *testing.T) *fakeStore {
	t.Helper()

	zipBytes := moduleZipBytes(t, map[string]string{"main.tf": `output "probe" {}`})
	layer := zipLayerDesc(zipBytes)
	manifestBytes, manifestDesc := manifestFor(t, getter.ArtifactTypeModulePkg, layer)

	return newFakeStore(manifestBytes, &manifestDesc, zipBytes, &layer)
}

// failingNewStore returns a seam whose construction always fails.
func failingNewStore() getter.OCINewStoreFunc {
	return func(context.Context, string, string) (getter.OCIRepositoryStore, error) {
		return nil, errUnknownBlob
	}
}

// failingResolveStoreFunc returns a seam whose store fails every Resolve.
func failingResolveStoreFunc() getter.OCINewStoreFunc {
	return func(context.Context, string, string) (getter.OCIRepositoryStore, error) {
		return failingResolveStore{}, nil
	}
}

// failingResolveStore fails Resolve and Fetch alike.
type failingResolveStore struct{}

func (failingResolveStore) Resolve(context.Context, string) (ociv1.Descriptor, error) {
	return ociv1.Descriptor{}, errUnknownBlob
}

func (failingResolveStore) Fetch(context.Context, *ociv1.Descriptor) (io.ReadCloser, error) {
	return nil, errUnknownBlob
}
