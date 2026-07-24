package getter_test

import (
	"net/url"
	"path/filepath"
	"testing"

	gogetter "github.com/hashicorp/go-getter/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	tgcas "github.com/gruntwork-io/terragrunt/internal/cas"
	"github.com/gruntwork-io/terragrunt/internal/getter"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/gruntwork-io/terragrunt/internal/vhttp"
	"github.com/gruntwork-io/terragrunt/test/helpers"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
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
			assert.False(
				t,
				ok,
				"URL-scheme claim must not match %s; require <scheme>:: forced prefix instead",
				tt.src,
			)
		})
	}
}

// TestCASGetterDetect_AWSS3HTTPSRoutesToS3Fetcher pins that an https URL
// against an AWS S3 endpoint claims the s3 scheme (so the inner fetch
// uses S3 auth) and is rewritten to the path-style form the bare s3
// getter accepts. Without this, virtual-host URLs would route through
// the plain HTTPS fetcher and silently fail for private buckets.
func TestCASGetterDetect_AWSS3HTTPSRoutesToS3Fetcher(t *testing.T) {
	t.Parallel()

	g := newCASGetterForDetect(t)

	tests := []struct {
		name    string
		src     string
		wantSrc string
	}{
		{
			name:    "global virtual-host rewritten to global path-style",
			src:     "https://my-bucket.s3.amazonaws.com/path.zip",
			wantSrc: "https://s3.amazonaws.com/my-bucket/path.zip",
		},
		{
			name:    "regional virtual-host rewritten to regional path-style",
			src:     "https://my-bucket.s3-us-west-2.amazonaws.com/path.zip",
			wantSrc: "https://s3-us-west-2.amazonaws.com/my-bucket/path.zip",
		},
		{
			name:    "modern virtual-host rewritten to regional path-style",
			src:     "https://my-bucket.s3.us-west-2.amazonaws.com/path.zip",
			wantSrc: "https://s3-us-west-2.amazonaws.com/my-bucket/path.zip",
		},
		{
			name:    "modern us-east-1 virtual-host rewritten to global path-style",
			src:     "https://my-bucket.s3.us-east-1.amazonaws.com/path.zip",
			wantSrc: "https://s3.amazonaws.com/my-bucket/path.zip",
		},
		{
			name:    "modern path-style rewritten to legacy regional path-style",
			src:     "https://s3.us-west-2.amazonaws.com/my-bucket/path.zip",
			wantSrc: "https://s3-us-west-2.amazonaws.com/my-bucket/path.zip",
		},
		{
			name:    "global path-style claimed unchanged",
			src:     "https://s3.amazonaws.com/my-bucket/path.zip",
			wantSrc: "https://s3.amazonaws.com/my-bucket/path.zip",
		},
		{
			name:    "regional path-style claimed unchanged",
			src:     "https://s3-us-west-2.amazonaws.com/my-bucket/path.zip",
			wantSrc: "https://s3-us-west-2.amazonaws.com/my-bucket/path.zip",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			req := &gogetter.Request{Src: tt.src}

			ok, err := g.Detect(req)
			require.NoError(t, err)
			require.True(t, ok, "detector should claim %s", tt.src)
			assert.Equal(
				t,
				getter.SchemeS3,
				req.Forced,
				"AWS S3 host must route through the s3 fetcher",
			)

			u, parseErr := url.Parse(req.Src)
			require.NoError(t, parseErr)

			q := u.Query()
			assert.Equal(t, "false", q.Get("archive"), "Detect must append archive=false")

			q.Del("archive")
			u.RawQuery = q.Encode()

			assert.Equal(
				t,
				tt.wantSrc,
				u.String(),
				"Src must be canonicalized to the bare s3 getter's accepted form",
			)
		})
	}
}

// TestCASGetterDetect_AWSS3HTTPSPreservesQueryAndVersion pins that the
// ?version selector and other query parameters survive the rewrite, so
// versioned S3 objects keep resolving to the same VersionId after
// canonicalization.
func TestCASGetterDetect_AWSS3HTTPSPreservesQueryAndVersion(t *testing.T) {
	t.Parallel()

	g := newCASGetterForDetect(t)

	req := &gogetter.Request{Src: "https://my-bucket.s3.amazonaws.com/path.zip?version=abc123"}

	ok, err := g.Detect(req)
	require.NoError(t, err)
	require.True(t, ok)
	assert.Equal(t, getter.SchemeS3, req.Forced)

	u, parseErr := url.Parse(req.Src)
	require.NoError(t, parseErr)
	assert.Equal(t, "abc123", u.Query().Get("version"))
	assert.Equal(t, "/my-bucket/path.zip", u.Path)
	assert.Equal(t, "s3.amazonaws.com", u.Host)
}

// TestCASGetterDetect_NonS3AmazonAWSHostFallsThroughToHTTPS pins that
// non-S3 amazonaws.com hosts (iam, sts, ec2, ...) stay on the HTTPS
// fetcher rather than being misrouted through s3. canonicalAWSS3HTTPSURL
// is the gate.
func TestCASGetterDetect_NonS3AmazonAWSHostFallsThroughToHTTPS(t *testing.T) {
	t.Parallel()

	g := newCASGetterForDetect(t)

	tests := []string{
		"https://iam.amazonaws.com/bucket/key.tgz",
		"https://sts.amazonaws.com/bucket/key.tgz",
		"https://ec2.amazonaws.com/bucket/key.tgz",
	}

	for _, src := range tests {
		t.Run(src, func(t *testing.T) {
			t.Parallel()

			req := &gogetter.Request{Src: src}

			ok, err := g.Detect(req)
			require.NoError(t, err)
			require.True(t, ok)
			assert.Equal(
				t,
				getter.SchemeHTTPS,
				req.Forced,
				"non-S3 amazonaws.com hosts must route through HTTPS, not s3",
			)
		})
	}
}

// TestCASGetterDetect_S3ForcedPrefixCanonicalizesVHost pins that
// `s3::https://<bucket>.s3.amazonaws.com/<key>` is rewritten to the
// path-style form before being handed to the bare s3 getter, which
// rejects virtual-host hosts. Without this rewrite, the forced-prefix
// form would set up a doomed inner fetch on every cache miss.
func TestCASGetterDetect_S3ForcedPrefixCanonicalizesVHost(t *testing.T) {
	t.Parallel()

	g := newCASGetterForDetect(t)

	req := &gogetter.Request{
		Src:    "https://my-bucket.s3.amazonaws.com/path.zip",
		Forced: getter.SchemeS3,
	}

	ok, err := g.Detect(req)
	require.NoError(t, err)
	require.True(t, ok)
	assert.Equal(t, getter.SchemeS3, req.Forced)

	u, parseErr := url.Parse(req.Src)
	require.NoError(t, parseErr)
	assert.Equal(t, "/my-bucket/path.zip", u.Path)
	assert.Equal(t, "s3.amazonaws.com", u.Host)
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

	v := venv.OSVenv()

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

// TestNewCASGetter_PanicsOnNilVenvFS pins the constructor-time rejection
// of a Venv missing FS. The misconfiguration surfaces at the offending
// NewCASGetter call rather than at first Detect.
func TestNewCASGetter_PanicsOnNilVenvFS(t *testing.T) {
	t.Parallel()

	storePath := filepath.Join(helpers.TmpDirWOSymlinks(t), "store")
	c, err := tgcas.New(tgcas.WithStorePath(storePath))
	require.NoError(t, err)

	require.PanicsWithValue(t, venv.ErrVenvFSUnset, func() {
		getter.NewCASGetter(logger.CreateLogger(), c, &venv.Venv{}, &tgcas.CloneOptions{})
	})
}

// TestNewCASGetter_PanicsOnNilVenvExec pins the constructor-time
// rejection of a Venv with FS set but Exec missing. CASGetter derives
// a git runner from Exec for any git source, so a missing executor
// would otherwise nil-deref deep inside the clone path.
func TestNewCASGetter_PanicsOnNilVenvExec(t *testing.T) {
	t.Parallel()

	storePath := filepath.Join(helpers.TmpDirWOSymlinks(t), "store")
	c, err := tgcas.New(tgcas.WithStorePath(storePath))
	require.NoError(t, err)

	v := &venv.Venv{FS: vfs.NewOSFS()}

	require.PanicsWithValue(t, venv.ErrVenvExecUnset, func() {
		getter.NewCASGetter(logger.CreateLogger(), c, v, &tgcas.CloneOptions{})
	})
}

// TestNewCASGetter_PanicsOnNilVenvHTTPWithDispatch pins the
// construction-time rejection of a Venv missing HTTP when
// WithDefaultGenericDispatch needs it for resolver probes. Without an
// explicit WithHTTPClient override, the venv's client is the only
// source, so its absence surfaces at the offending NewCASGetter call.
func TestNewCASGetter_PanicsOnNilVenvHTTPWithDispatch(t *testing.T) {
	t.Parallel()

	storePath := filepath.Join(helpers.TmpDirWOSymlinks(t), "store")
	c, err := tgcas.New(tgcas.WithStorePath(storePath))
	require.NoError(t, err)

	v := venv.OSVenv()
	v.HTTP = nil

	require.PanicsWithValue(t, venv.ErrVenvHTTPUnset, func() {
		getter.NewCASGetter(logger.CreateLogger(), c, v, &tgcas.CloneOptions{},
			getter.WithDefaultGenericDispatch())
	})
}

// TestCASGetterDetect_PanicsOnNilVenvFS pins the in-Detect repeat of
// the constructor check. Only reachable when a caller hand-assembles
// CASGetter and skips NewCASGetter.
func TestCASGetterDetect_PanicsOnNilVenvFS(t *testing.T) {
	t.Parallel()

	storePath := filepath.Join(helpers.TmpDirWOSymlinks(t), "store")
	c, err := tgcas.New(tgcas.WithStorePath(storePath))
	require.NoError(t, err)

	g := &getter.CASGetter{
		CAS:       c,
		Logger:    logger.CreateLogger(),
		Opts:      &tgcas.CloneOptions{},
		Venv:      &venv.Venv{},
		Detectors: []getter.Detector{new(getter.FileDetector)},
	}

	require.PanicsWithValue(t, venv.ErrVenvFSUnset, func() {
		_, _ = g.Detect(&gogetter.Request{Src: "./some/local/path", Pwd: t.TempDir()})
	})
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
		{
			name: "preserve archive=true",
			src:  "https://example.com/mod.tar.gz?archive=true",
			want: "true",
		},
		{
			name: "preserve archive=false",
			src:  "https://example.com/mod.tar.gz?archive=false",
			want: "false",
		},
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

	v := venv.OSVenv()

	return getter.NewCASGetter(logger.CreateLogger(), c, v, &tgcas.CloneOptions{},
		getter.WithDefaultGenericDispatch(getter.WithHTTPClient(vhttp.NewNoNetworkClient())))
}
