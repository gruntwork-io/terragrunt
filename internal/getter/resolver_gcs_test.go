package getter_test

import (
	"context"
	"encoding/hex"
	"errors"
	"testing"

	"cloud.google.com/go/storage"

	"github.com/gruntwork-io/terragrunt/internal/cas"
	"github.com/gruntwork-io/terragrunt/internal/getter"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGCSResolver_PrefersMD5(t *testing.T) {
	t.Parallel()

	md5 := []byte{0xde, 0xad, 0xbe, 0xef, 0x00, 0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08, 0x09, 0x0a, 0x0b}
	client := &fakeGCSClient{objects: map[string]*fakeGCSObject{
		"path/to/key.tgz": {attrs: &storage.ObjectAttrs{
			MD5:    md5,
			CRC32C: 0xdeadbeef,
			Etag:   `"some-etag"`,
		}},
	}}

	r := newGCSResolverWith(client)

	got, err := r.Probe(t.Context(), "gs://bucket/path/to/key.tgz")
	require.NoError(t, err)
	assert.Equal(t, cas.ContentKey("md5", hex.EncodeToString(md5)), got)
}

func TestGCSResolver_FallsThroughToCRC32CWhenMD5Absent(t *testing.T) {
	t.Parallel()

	client := &fakeGCSClient{objects: map[string]*fakeGCSObject{
		"composite.tgz": {attrs: &storage.ObjectAttrs{
			// MD5 nil, common on composite objects.
			CRC32C: 0xdeadbeef,
			Etag:   `"some-etag"`,
		}},
	}}

	r := newGCSResolverWith(client)

	got, err := r.Probe(t.Context(), "gs://bucket/composite.tgz")
	require.NoError(t, err)
	assert.Equal(t, cas.ContentKey("crc32c", "deadbeef"), got)
}

// TestGCSResolver_PrefersCRC32CEvenWhenZero pins that CRC32C
// participates in the cascade even when the checksum value is
// literally 0. Some legitimate content (the empty object is the
// canonical example, but other byte sequences also hash to 0)
// produces a zero CRC32C, and treating that as "absent" silently
// downgrades the cache key to the opaque ETag fallback, losing
// cross-URL dedupe for content-addressable objects.
func TestGCSResolver_PrefersCRC32CEvenWhenZero(t *testing.T) {
	t.Parallel()

	client := &fakeGCSClient{objects: map[string]*fakeGCSObject{
		"zero-crc.tgz": {attrs: &storage.ObjectAttrs{
			// MD5 absent so the cascade falls to CRC32C; the legal
			// value 0 must not be treated as "no signal".
			CRC32C: 0,
			Etag:   `"some-etag"`,
		}},
	}}

	r := newGCSResolverWith(client)

	got, err := r.Probe(t.Context(), "gs://bucket/zero-crc.tgz")
	require.NoError(t, err)
	assert.Equal(t, cas.ContentKey("crc32c", "0"), got,
		"CRC32C=0 is a real checksum and must be preferred over the opaque ETag")
}

func TestGCSResolver_AttrsFailureSurfacesErrNoVersionMetadata(t *testing.T) {
	t.Parallel()

	client := &fakeGCSClient{objects: map[string]*fakeGCSObject{
		"err.tgz": {err: errors.New("transient GCS error")},
	}}

	r := newGCSResolverWith(client)

	_, err := r.Probe(t.Context(), "gs://bucket/err.tgz")
	require.ErrorIs(t, err, cas.ErrNoVersionMetadata)
}

// TestGCSResolver_NilAttrsReturnsErrNoVersionMetadata covers the
// only path to ErrNoVersionMetadata after the cascade lost its
// ETag/empty-attrs fallback: an SDK that returns (nil, nil) from
// Attrs. The fake client below stands in for that regression mode;
// real GCS always populates CRC32C, so the empty-attrs path is
// unreachable from production.
func TestGCSResolver_NilAttrsReturnsErrNoVersionMetadata(t *testing.T) {
	t.Parallel()

	client := &fakeGCSClient{objects: map[string]*fakeGCSObject{
		"empty.tgz": {attrs: nil},
	}}

	r := newGCSResolverWith(client)

	_, err := r.Probe(t.Context(), "gs://bucket/empty.tgz")
	require.ErrorIs(t, err, cas.ErrNoVersionMetadata)
}

func TestGCSResolver_AcceptsCanonicalAndShortURLs(t *testing.T) {
	t.Parallel()

	md5 := []byte{0xde, 0xad, 0xbe, 0xef, 0x00, 0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08, 0x09, 0x0a, 0x0b}
	client := &fakeGCSClient{objects: map[string]*fakeGCSObject{
		"path/to/key.tgz": {attrs: &storage.ObjectAttrs{MD5: md5}},
	}}

	r := newGCSResolverWith(client)

	short, err := r.Probe(t.Context(), "gs://bucket/path/to/key.tgz")
	require.NoError(t, err)

	canonical, err := r.Probe(t.Context(), "https://www.googleapis.com/storage/v1/bucket/path/to/key.tgz")
	require.NoError(t, err)

	// Both forms resolve to the same object metadata, so the
	// content-addressed cache key is identical.
	assert.Equal(t, short, canonical)
}

func TestGCSResolver_RejectsUnknownURLShape(t *testing.T) {
	t.Parallel()

	r := newGCSResolverWith(&fakeGCSClient{})

	_, err := r.Probe(t.Context(), "ftp://bucket/key.tgz")
	require.ErrorIs(t, err, getter.ErrGCSUnsupportedScheme)
}

// TestGCSResolver_RejectsEmptyObject pins that parseGCSURL rejects
// URLs that name a bucket but no object. Without this guard the
// resolver passes object="" to ObjectHandle.Attrs, which fails
// downstream with an SDK-shaped error that surfaces as
// ErrNoVersionMetadata after a wasted round-trip. Fail at parse time
// instead so the caller sees a meaningful URL-shape error.
func TestGCSResolver_RejectsEmptyObject(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		url  string
	}{
		{name: "gs bucket with no path", url: "gs://bucket"},
		{name: "gs bucket with trailing slash only", url: "gs://bucket/"},
		{name: "canonical with no object segment", url: "https://www.googleapis.com/storage/v1/bucket/"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			r := newGCSResolverWith(&fakeGCSClient{objects: map[string]*fakeGCSObject{
				"": {attrs: nil, err: errors.New("Attrs must not be called for an empty-object URL")},
			}})

			_, err := r.Probe(t.Context(), tt.url)
			require.ErrorIs(t, err, getter.ErrGCSMissingObject,
				"parseGCSURL must reject %q with no object", tt.url)
			require.NotErrorIs(t, err, cas.ErrNoVersionMetadata,
				"rejection must come from parseGCSURL, not from an Attrs call on an empty object name")
		})
	}
}

// TestGCSResolver_RejectsEmptyBucket pins that parseGCSURL rejects a
// gs:// URL with no host. Without this guard the resolver passes
// bucket="" downstream and pays a doomed Attrs round trip.
func TestGCSResolver_RejectsEmptyBucket(t *testing.T) {
	t.Parallel()

	r := newGCSResolverWith(&fakeGCSClient{})

	_, err := r.Probe(t.Context(), "gs:///key.tgz")
	require.ErrorIs(t, err, getter.ErrGCSMissingBucket)
	require.NotErrorIs(t, err, cas.ErrNoVersionMetadata)
}

// TestGCSResolver_RejectsUnrecognizedCanonicalPath pins that an
// http(s) URL outside the canonical `/storage/<version>/<bucket>/<object>`
// shape is rejected at parse time. Mirrors the S3 resolver's
// parse-time rejection of unsupported URL forms.
func TestGCSResolver_RejectsUnrecognizedCanonicalPath(t *testing.T) {
	t.Parallel()

	r := newGCSResolverWith(&fakeGCSClient{})

	_, err := r.Probe(t.Context(), "https://www.googleapis.com/not-storage/v1/bucket/key.tgz")
	require.ErrorIs(t, err, getter.ErrGCSUnrecognizedURL)
	require.NotErrorIs(t, err, cas.ErrNoVersionMetadata)
}

// fakeGCSObject returns canned object attributes.
type fakeGCSObject struct {
	attrs *storage.ObjectAttrs
	err   error
}

func (o *fakeGCSObject) Attrs(_ context.Context) (*storage.ObjectAttrs, error) {
	if o.err != nil {
		return nil, o.err
	}

	return o.attrs, nil
}

// fakeGCSClient routes Object(bucket, name) to a per-name fake.
type fakeGCSClient struct {
	objects map[string]*fakeGCSObject
}

func (c *fakeGCSClient) Object(_, name string) getter.GCSObject {
	if o, ok := c.objects[name]; ok {
		return o
	}

	return &fakeGCSObject{err: errors.New("no such object")}
}

func (c *fakeGCSClient) Close() error { return nil }

func newGCSResolverWith(client *fakeGCSClient) *getter.GCSResolver {
	r := getter.NewGCSResolver()
	r.NewClient = func(_ context.Context) (getter.GCSClient, error) {
		return client, nil
	}

	return r
}
