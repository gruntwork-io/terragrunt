package getter_test

import (
	"net/url"
	"path/filepath"
	"testing"

	tgcas "github.com/gruntwork-io/terragrunt/internal/cas"
	"github.com/gruntwork-io/terragrunt/internal/getter"
	"github.com/gruntwork-io/terragrunt/test/helpers"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	gogetter "github.com/hashicorp/go-getter/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCASGetterDetect_GitForcedPrefix(t *testing.T) {
	t.Parallel()

	g := newCASGetterForDetect(t)

	tests := []struct {
		name      string
		src       string
		reqForced string
		want      string
	}{
		{
			name: "git:: forced prefix is claimed and Src is stripped",
			src:  "git::https://example.com/repo.git",
			want: getter.SchemeGit,
		},
		{
			name:      "Forced field set to git is claimed without inspecting Src",
			src:       "https://example.com/repo.git",
			reqForced: getter.SchemeGit,
			want:      getter.SchemeGit,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			req := &gogetter.Request{Src: tt.src, Forced: tt.reqForced}

			ok, err := g.Detect(req)
			require.NoError(t, err)
			require.True(t, ok)
			assert.Equal(t, tt.want, req.Forced)
		})
	}
}

func TestCASGetterDetect_GenericForcedPrefixes(t *testing.T) {
	t.Parallel()

	g := newCASGetterForDetect(t)

	tests := []struct {
		forced string
		src    string
	}{
		{forced: getter.SchemeS3, src: "s3.amazonaws.com/bucket/key.tgz"},
		{forced: getter.SchemeGCS, src: "www.googleapis.com/storage/v1/bucket/key.tgz"},
		{forced: getter.SchemeHTTP, src: "example.com/mod.tar.gz"},
		{forced: getter.SchemeHTTPS, src: "example.com/mod.tar.gz"},
		{forced: getter.SchemeHg, src: "example.com/repo"},
		{forced: getter.SchemeSMB, src: "example.com/share/path"},
	}

	for _, tt := range tests {
		t.Run("forced "+tt.forced, func(t *testing.T) {
			t.Parallel()

			req := &gogetter.Request{Src: tt.src, Forced: tt.forced}

			ok, err := g.Detect(req)
			require.NoError(t, err)
			require.True(t, ok)
			assert.Equal(t, tt.forced, req.Forced)
			// Detect appends archive=false so the outer v2 client
			// skips its pre-decompression step.
			u, parseErr := url.Parse(req.Src)
			require.NoError(t, parseErr)
			assert.Equal(t, "false", u.Query().Get("archive"))
		})
	}
}

// TestCASGetterDetect_SchemeDetectionByURL pins URL-scheme claiming.
// Only http and https URLs are claimed by URL scheme alone; s3, gcs,
// hg, and smb sources route through the bare go-getter v2 protocol
// getters and reject the `<scheme>://...` form, so claiming those
// schemes by URL would set up a doomed inner fetch on every cache
// miss. Routing those sources through CAS requires the explicit
// forced-prefix form (`s3::https://...`, `gcs::https://...`).
func TestCASGetterDetect_SchemeDetectionByURL(t *testing.T) {
	t.Parallel()

	g := newCASGetterForDetect(t)

	claimed := []struct {
		name   string
		src    string
		forced string
	}{
		{name: "http URL", src: "http://example.com/mod.tar.gz", forced: getter.SchemeHTTP},
		{name: "https URL", src: "https://example.com/mod.tar.gz", forced: getter.SchemeHTTPS},
	}

	for _, tt := range claimed {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			req := &gogetter.Request{Src: tt.src}

			ok, err := g.Detect(req)
			require.NoError(t, err)
			require.True(t, ok, "detector should claim %s", tt.src)
			assert.Equal(t, tt.forced, req.Forced)
		})
	}

	unclaimed := []struct {
		name string
		src  string
	}{
		{name: "s3 URL", src: "s3://bucket/key.tgz"},
		{name: "gs URL", src: "gs://bucket/key.tgz"},
	}

	for _, tt := range unclaimed {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			req := &gogetter.Request{Src: tt.src}

			ok, _ := g.Detect(req)
			assert.False(t, ok, "URL-scheme claim must not match %s; require <scheme>:: forced prefix instead", tt.src)
		})
	}
}

// TestCASGetterDetect_ForcedPrefixNormalizesAlias pins that `gs::`
// forced inputs route through the gcs fetcher entry. Without this,
// `gs::` users would silently miss the GCS dispatch path.
func TestCASGetterDetect_ForcedPrefixNormalizesAlias(t *testing.T) {
	t.Parallel()

	g := newCASGetterForDetect(t)

	req := &gogetter.Request{
		Src:    "https://www.googleapis.com/storage/v1/bucket/mod.tgz",
		Forced: "gs",
	}

	ok, err := g.Detect(req)
	require.NoError(t, err)
	require.True(t, ok, "gs:: forced prefix must claim through the gcs fetcher")
	assert.Equal(t, getter.SchemeGCS, req.Forced, "Forced must be normalized to the registry key")
}

func TestCASGetterDetect_SchemeNotInRegistryFallsThrough(t *testing.T) {
	t.Parallel()

	// A scheme that CASGetter does not handle, with no fetcher
	// registered for it, must not be claimed by the generic-scheme
	// matcher. (A higher-priority getter, TFR for instance, wins the
	// outer registry race in this case.)
	storePath := filepath.Join(helpers.TmpDirWOSymlinks(t), "store")
	c, err := tgcas.New(tgcas.WithStorePath(storePath))
	require.NoError(t, err)

	v, err := tgcas.OSVenv()
	require.NoError(t, err)

	// No generic dispatch wired: only the git+file paths are active.
	g := getter.NewCASGetter(logger.CreateLogger(), c, v, &tgcas.CloneOptions{})

	// An s3:// URL would be claimed if generic dispatch were on;
	// without it, the generic-scheme matcher in Detect must return
	// false (the FileDetector then runs and reports a stat error,
	// but the assertion here only cares that the generic-scheme
	// path did not silently match).
	req := &gogetter.Request{Src: "s3://bucket/key.tgz"}

	ok, _ := g.Detect(req)
	assert.False(t, ok, "without WithGenericFetchers, s3:// must not be claimed")
}

func TestCASGetterDetect_PreservesExistingArchiveQueryValue(t *testing.T) {
	t.Parallel()

	g := newCASGetterForDetect(t)

	// If the URL already carries archive=true, Detect must not
	// overwrite it. Same for archive=false (which is what Detect
	// would have added anyway).
	tests := []struct {
		name string
		src  string
		want string
	}{
		{name: "preserve archive=true", src: "https://example.com/mod.tar.gz?archive=true", want: "true"},
		{name: "preserve archive=false", src: "https://example.com/mod.tar.gz?archive=false", want: "false"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			req := &gogetter.Request{Src: tt.src}

			ok, err := g.Detect(req)
			require.NoError(t, err)
			require.True(t, ok)

			u, parseErr := url.Parse(req.Src)
			require.NoError(t, parseErr)
			assert.Equal(t, tt.want, u.Query().Get("archive"))
		})
	}
}

// newCASGetterForDetect returns a CASGetter with the default generic
// dispatch wiring so Detect's scheme-matching path is fully exercised.
func newCASGetterForDetect(t *testing.T) *getter.CASGetter {
	t.Helper()

	storePath := filepath.Join(helpers.TmpDirWOSymlinks(t), "store")
	c, err := tgcas.New(tgcas.WithStorePath(storePath))
	require.NoError(t, err)

	v, err := tgcas.OSVenv()
	require.NoError(t, err)

	return getter.NewCASGetter(logger.CreateLogger(), c, v, &tgcas.CloneOptions{}, getter.WithDefaultGenericDispatch())
}
