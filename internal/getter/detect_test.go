package getter_test

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/getter"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestDetectCanonicalizesShorthand pins the host-shorthand canonicalization
// from each upstream detector so a v1->v2 protocol drift would surface here.
//
// The s3, gcs, and absolute-path rows specifically exercise the prefixedDetector
// and fileSchemeDetector wrappers in defaultDetectors(): v2 dropped v1's
// inline "s3::"/"gcs::" forced-getter prefix and "file://" scheme on raw
// detector output, so the wrappers reattach them to preserve v1 textual
// conventions that downstream parseSourceURL / IsLocalSource depend on.
func TestDetectCanonicalizesShorthand(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		src    string
		expect string
	}{
		{
			name:   "github shorthand",
			src:    "github.com/gruntwork-io/terragrunt",
			expect: "git::https://github.com/gruntwork-io/terragrunt.git",
		},
		{
			name:   "explicit forced getter passes through",
			src:    "git::https://github.com/foo/bar.git",
			expect: "git::https://github.com/foo/bar.git",
		},
		{
			name:   "https url with valid scheme returns unchanged",
			src:    "https://example.com/foo.zip",
			expect: "https://example.com/foo.zip",
		},
		{
			name:   "tfr scheme returns unchanged",
			src:    "tfr:///foo/bar/baz?version=1.0.0",
			expect: "tfr:///foo/bar/baz?version=1.0.0",
		},
		{
			name:   "s3 vhost shorthand reattaches s3:: prefix",
			src:    "bucket.s3.amazonaws.com/key/path",
			expect: "s3::https://s3.amazonaws.com/bucket/key/path",
		},
		{
			name:   "gcs shorthand reattaches gcs:: prefix",
			src:    "www.googleapis.com/storage/v1/bucket/object",
			expect: "gcs::https://www.googleapis.com/storage/v1/bucket/object",
		},
		{
			name:   "absolute path gets file:// scheme reattached",
			src:    "/abs/path/to/module",
			expect: "file:///abs/path/to/module",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := getter.Detect(tt.src, "/tmp")
			require.NoError(t, err)
			assert.Equal(t, tt.expect, got)
		})
	}
}

// TestDetectWithSubdirReattaches pins the //subdir-preservation behavior on
// the detector chain. tf.normalizeSourceURL relies on this so a github
// shorthand with a //subdir keeps that subdir after canonicalization.
func TestDetectWithSubdirReattaches(t *testing.T) {
	t.Parallel()

	got, err := getter.Detect("github.com/gruntwork-io/terragrunt//modules/foo", "/tmp")
	require.NoError(t, err)
	assert.Equal(t, "git::https://github.com/gruntwork-io/terragrunt.git//modules/foo", got)
}

// TestDetectWithErrorPropagates ensures errors from a detector terminate
// the chain rather than silently falling through.
func TestDetectWithErrorPropagates(t *testing.T) {
	t.Parallel()

	want := errors.New("detector boom")
	det := []getter.Detector{erroringDetector{err: want}}

	_, err := getter.DetectWith("foo", "/tmp", det)
	require.Error(t, err)
	assert.ErrorIs(t, err, want)
}

// TestDetectWithNoMatchReturnsOriginal is the contract when no detector in
// the chain matches: the caller's source is returned unchanged.
func TestDetectWithNoMatchReturnsOriginal(t *testing.T) {
	t.Parallel()

	got, err := getter.DetectWith("does-not-look-like-anything", "/tmp", []getter.Detector{})
	require.NoError(t, err)
	assert.Equal(t, "does-not-look-like-anything", got)
}

// TestSubdirGlob exercises the wrapper around go-getter's SubdirGlob helper
// so a future v2 upgrade can't silently change the contract on it.
func TestSubdirGlob(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, "modules", "foo"), 0755))

	got, err := getter.SubdirGlob(root, "modules/foo")
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(root, "modules", "foo"), got)
}

// TestURLParseHandlesEmptyScheme pins URLParse against the helper-url
// behavior tf/source.go relies on for pre-canonicalized strings.
func TestURLParseHandlesEmptyScheme(t *testing.T) {
	t.Parallel()

	u, err := getter.URLParse("github.com/foo/bar")
	require.NoError(t, err)
	assert.Empty(t, u.Scheme)
}

// erroringDetector is a Detector that always returns an error. Used to pin
// the error-propagation behavior of DetectWith.
type erroringDetector struct{ err error }

func (d erroringDetector) Detect(_, _ string) (string, bool, error) {
	return "", false, d.err
}
