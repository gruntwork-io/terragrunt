package getter_test

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	gogetter "github.com/hashicorp/go-getter/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	tgcas "github.com/gruntwork-io/terragrunt/internal/cas"
	"github.com/gruntwork-io/terragrunt/internal/getter"
	"github.com/gruntwork-io/terragrunt/internal/vhttp"
	"github.com/gruntwork-io/terragrunt/test/helpers"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/gruntwork-io/terragrunt/test/helpers/venvtest"
)

// tarballHandler serves the supplied tar.gz bytes with a stable ETag and
// counts GET vs HEAD requests so a test can assert the second run skips
// the GET.
type tarballHandler struct {
	etag  string
	body  []byte
	heads atomic.Int32
	gets  atomic.Int32
}

func (h *tarballHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("ETag", `"`+h.etag+`"`)
	w.Header().Set("Content-Type", "application/gzip")

	switch r.Method {
	case http.MethodHead:
		h.heads.Add(1)
		w.WriteHeader(http.StatusOK)
	case http.MethodGet:
		h.gets.Add(1)
		w.WriteHeader(http.StatusOK)

		if _, err := w.Write(h.body); err != nil {
			panic(err)
		}
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

// makeTarGz packs files into an in-memory tar.gz tree.
func makeTarGz(t *testing.T, files map[string]string) []byte {
	t.Helper()

	var buf bytes.Buffer

	gzw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gzw)

	for name, body := range files {
		hdr := &tar.Header{
			Name: name,
			Mode: 0o644,
			Size: int64(len(body)),
		}
		require.NoError(t, tw.WriteHeader(hdr))

		_, err := tw.Write([]byte(body))
		require.NoError(t, err)
	}

	require.NoError(t, tw.Close())
	require.NoError(t, gzw.Close())

	return buf.Bytes()
}

func TestCASGetter_HTTPArchiveCachesSecondRun(t *testing.T) {
	t.Parallel()

	body := makeTarGz(t, map[string]string{
		"main.tf":   `resource "null_resource" "a" {}`,
		"sub/x.tf":  "variable \"x\" {}\n",
		"README.md": "hello",
	})

	h := &tarballHandler{body: body, etag: "stable-etag"}

	srv := httptest.NewServer(h)
	defer srv.Close()

	storePath := filepath.Join(helpers.TmpDirWOSymlinks(t), "store")
	c, err := tgcas.New(venvtest.NewWithOSFS(), tgcas.WithStorePath(storePath))
	require.NoError(t, err)

	v := venvtest.NewOSWithEmptyEnv()

	l := logger.CreateLogger()

	g := getter.NewCASGetter(l, c, v, &tgcas.CloneOptions{},
		getter.WithDefaultGenericDispatch(getter.WithHTTPClient(vhttp.NewOSClient())))

	client := &gogetter.Client{Getters: []gogetter.Getter{g}}

	src := srv.URL + "/mod.tar.gz"

	runOnce := func(t *testing.T) string {
		t.Helper()

		dst := filepath.Join(t.TempDir(), "out")

		_, err := client.Get(t.Context(), &gogetter.Request{
			Src:     src,
			Dst:     dst,
			GetMode: gogetter.ModeAny,
		})
		require.NoError(t, err)
		require.FileExists(t, filepath.Join(dst, "main.tf"))
		require.FileExists(t, filepath.Join(dst, "sub", "x.tf"))

		return dst
	}

	runOnce(t)

	firstGets := h.gets.Load()
	firstHeads := h.heads.Load()

	assert.Equal(t, int32(1), firstGets, "first run must download the archive once")
	assert.GreaterOrEqual(
		t,
		firstHeads,
		int32(1),
		"first run must probe via HEAD before downloading",
	)

	runOnce(t)

	assert.Equal(t, firstGets, h.gets.Load(),
		"second run must hit the CAS via the probe and skip the archive GET")
	assert.Greater(t, h.heads.Load(), firstHeads,
		"second run must still probe to confirm the cached version is current")
}

// TestCASGetter_HTTPArchiveIgnoresIncludedGitFiles pins that the git-only
// IncludedGitFiles option production callers set on every CloneOptions
// neither writes a .git directory for a non-git source nor turns the
// probe hit into a re-download.
func TestCASGetter_HTTPArchiveIgnoresIncludedGitFiles(t *testing.T) {
	t.Parallel()

	body := makeTarGz(t, map[string]string{"main.tf": "ok"})

	h := &tarballHandler{body: body, etag: "stable-etag"}

	srv := httptest.NewServer(h)
	defer srv.Close()

	storePath := filepath.Join(helpers.TmpDirWOSymlinks(t), "store")
	c, err := tgcas.New(venvtest.NewWithOSFS(), tgcas.WithStorePath(storePath))
	require.NoError(t, err)

	v := venvtest.NewOSWithEmptyEnv()

	l := logger.CreateLogger()

	g := getter.NewCASGetter(l, c, v, &tgcas.CloneOptions{IncludedGitFiles: []string{"HEAD", "config"}},
		getter.WithDefaultGenericDispatch(getter.WithHTTPClient(vhttp.NewOSClient())))

	client := &gogetter.Client{Getters: []gogetter.Getter{g}}

	src := srv.URL + "/mod.tar.gz"

	for range 2 {
		dst := filepath.Join(t.TempDir(), "out")

		_, err := client.Get(t.Context(), &gogetter.Request{
			Src:     src,
			Dst:     dst,
			GetMode: gogetter.ModeAny,
		})
		require.NoError(t, err)
		require.FileExists(t, filepath.Join(dst, "main.tf"))
		require.NoDirExists(t, filepath.Join(dst, ".git"))
	}

	assert.Equal(t, int32(1), h.gets.Load(), "second run must be served from the store")
}

// TestCASGetter_HTTPArchiveFalseSkipsExtraction pins that a user-supplied
// archive=false reaches the inner fetch, so CAS ingests the raw archive
// instead of its extracted contents. CAS injects archive=false itself to
// stop the outer client pre-decompressing, so the parameter cannot carry
// the user's intent on its own; Detect records it separately for Get.
func TestCASGetter_HTTPArchiveFalseSkipsExtraction(t *testing.T) {
	t.Parallel()

	body := makeTarGz(t, map[string]string{"main.tf": "ok"})

	h := &tarballHandler{body: body, etag: "stable-etag"}

	srv := httptest.NewServer(h)
	defer srv.Close()

	storePath := filepath.Join(helpers.TmpDirWOSymlinks(t), "store")
	c, err := tgcas.New(venvtest.NewWithOSFS(), tgcas.WithStorePath(storePath))
	require.NoError(t, err)

	v := venvtest.NewOSWithEmptyEnv()

	l := logger.CreateLogger()

	g := getter.NewCASGetter(l, c, v, &tgcas.CloneOptions{},
		getter.WithDefaultGenericDispatch(getter.WithHTTPClient(vhttp.NewOSClient())))

	client := &gogetter.Client{Getters: []gogetter.Getter{g}}

	dst := filepath.Join(t.TempDir(), "out")

	_, err = client.Get(t.Context(), &gogetter.Request{
		Src:     srv.URL + "/mod.tar.gz?archive=false",
		Dst:     dst,
		GetMode: gogetter.ModeAny,
	})
	require.NoError(t, err)

	require.FileExists(t, filepath.Join(dst, "mod.tar.gz"),
		"archive=false must leave the downloaded tarball unextracted")
	require.NoFileExists(t, filepath.Join(dst, "main.tf"),
		"archive=false must stop the inner client from extracting")
}

// TestCASGetter_HTTPChecksumAndArchiveFormatReachInnerFetch: the inner fetch applies checksum and archive format.
func TestCASGetter_HTTPChecksumAndArchiveFormatReachInnerFetch(t *testing.T) {
	t.Parallel()

	body := makeTarGz(t, map[string]string{"main.tf": "ok"})
	sum := sha256.Sum256(body)
	checksum := "sha256:" + hex.EncodeToString(sum[:])

	testCases := []struct {
		name       string
		path       string
		wantMainTF bool
	}{
		{name: "matching checksum", path: "/mod.tar.gz?checksum=" + checksum, wantMainTF: true},
		{name: "archive format without extension", path: "/mod?archive=tar.gz", wantMainTF: true},
		{name: "archive=true extracts by extension", path: "/mod.tar.gz?archive=true", wantMainTF: true},
		{name: "mismatched checksum", path: "/mod.tar.gz?checksum=sha256:" + strings.Repeat("0", 64)},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			srv := httptest.NewServer(&tarballHandler{body: body, etag: "stable-etag"})
			t.Cleanup(srv.Close)

			storePath := filepath.Join(helpers.TmpDirWOSymlinks(t), "store")
			c, err := tgcas.New(venvtest.NewWithOSFS(), tgcas.WithStorePath(storePath))
			require.NoError(t, err)

			v := venvtest.NewOSWithEmptyEnv()
			g := getter.NewCASGetter(logger.CreateLogger(), c, v, &tgcas.CloneOptions{},
				getter.WithDefaultGenericDispatch(getter.WithHTTPClient(vhttp.NewOSClient())))
			client := &gogetter.Client{Getters: []gogetter.Getter{g}}
			dst := filepath.Join(t.TempDir(), "out")

			_, err = client.Get(t.Context(), &gogetter.Request{
				Src:     srv.URL + tc.path,
				Dst:     dst,
				GetMode: gogetter.ModeAny,
			})

			if !tc.wantMainTF {
				var checksumErr *gogetter.ChecksumError
				require.ErrorAs(t, err, &checksumErr, "a checksum mismatch must fail the download")
				require.NoFileExists(t, filepath.Join(dst, "main.tf"))

				return
			}

			require.NoError(t, err)
			require.FileExists(t, filepath.Join(dst, "main.tf"), "the inner fetch must verify and extract the archive")
		})
	}
}

// TestCASGetter_HTTPChecksumIsVerifiedOnStoreHit: a stored tree must not satisfy a request whose checksum disagrees.
func TestCASGetter_HTTPChecksumIsVerifiedOnStoreHit(t *testing.T) {
	t.Parallel()

	body := makeTarGz(t, map[string]string{"main.tf": "ok"})

	srv := httptest.NewServer(&tarballHandler{body: body, etag: "stable-etag"})
	t.Cleanup(srv.Close)

	storePath := filepath.Join(helpers.TmpDirWOSymlinks(t), "store")
	c, err := tgcas.New(venvtest.NewWithOSFS(), tgcas.WithStorePath(storePath))
	require.NoError(t, err)

	v := venvtest.NewOSWithEmptyEnv()

	fetch := func(path, dst string) error {
		g := getter.NewCASGetter(logger.CreateLogger(), c, v, &tgcas.CloneOptions{},
			getter.WithDefaultGenericDispatch(getter.WithHTTPClient(vhttp.NewOSClient())))
		client := &gogetter.Client{Getters: []gogetter.Getter{g}}

		_, err := client.Get(t.Context(), &gogetter.Request{Src: srv.URL + path, Dst: dst, GetMode: gogetter.ModeAny})

		return err
	}

	require.NoError(t, fetch("/mod.tar.gz", filepath.Join(t.TempDir(), "first")),
		"the first fetch stores the archive under its ETag key")

	second := filepath.Join(t.TempDir(), "second")
	err = fetch("/mod.tar.gz?checksum=sha256:"+strings.Repeat("0", 64), second)

	var checksumErr *gogetter.ChecksumError
	require.ErrorAs(t, err, &checksumErr, "a wrong checksum must fail even though the ETag key is already stored")
	require.NoFileExists(t, filepath.Join(second, "main.tf"))
}

// TestCASGetter_HTTPMissingETagFallsBackToContentHash exercises the no-probe
// path: a server without ETag/Last-Modified causes CAS to download every
// run, but blob storage still dedupes across runs.
func TestCASGetter_HTTPMissingETagFallsBackToContentHash(t *testing.T) {
	t.Parallel()

	body := makeTarGz(t, map[string]string{"main.tf": "ok"})

	var gets atomic.Int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/gzip")

		if r.Method == http.MethodGet {
			gets.Add(1)
		}

		w.WriteHeader(http.StatusOK)

		if r.Method == http.MethodGet {
			if _, err := w.Write(body); err != nil {
				panic(err)
			}
		}
	}))
	defer srv.Close()

	storePath := filepath.Join(helpers.TmpDirWOSymlinks(t), "store")
	c, err := tgcas.New(venvtest.NewWithOSFS(), tgcas.WithStorePath(storePath))
	require.NoError(t, err)

	v := venvtest.NewOSWithEmptyEnv()

	l := logger.CreateLogger()

	g := getter.NewCASGetter(l, c, v, &tgcas.CloneOptions{},
		getter.WithDefaultGenericDispatch(getter.WithHTTPClient(vhttp.NewOSClient())))

	client := &gogetter.Client{Getters: []gogetter.Getter{g}}

	src := srv.URL + "/mod.tar.gz"

	for range 2 {
		dst := t.TempDir()

		_, err := client.Get(t.Context(), &gogetter.Request{
			Src:     src,
			Dst:     filepath.Join(dst, "out"),
			GetMode: gogetter.ModeAny,
		})
		require.NoError(t, err)
		require.FileExists(t, filepath.Join(dst, "out", "main.tf"))
	}

	assert.Equal(t, int32(2), gets.Load(),
		"with no ETag, every run downloads; cache deduplication happens only at the blob level")
}

// offlinePrime names what an online run leaves behind before the offline run.
type offlinePrime int

const (
	primeNothing offlinePrime = iota
	primeStore
	primeProbe
)

// TestCASGetter_HTTPChecksumHonorsOffline: offline, a pinned archive is linked from the store and never fetched.
func TestCASGetter_HTTPChecksumHonorsOffline(t *testing.T) {
	t.Parallel()

	body := makeTarGz(t, map[string]string{"main.tf": "ok"})
	sum := sha256.Sum256(body)
	literal := "/mod.tar.gz?checksum=sha256:" + hex.EncodeToString(sum[:])
	wrong := "/mod.tar.gz?checksum=sha256:" + strings.Repeat("0", 64)

	testCases := []struct {
		name  string
		path  string
		prime offlinePrime
	}{
		{name: "literal checksum on an empty store", path: literal},
		{name: "literal checksum on a warm store", path: literal, prime: primeStore},
		{name: "literal checksum recorded by a failed download", path: wrong, prime: primeProbe},
		{name: "checksum file on an empty store", path: "/mod.tar.gz?checksum=file:/nowhere/mod.sha256"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			h := &tarballHandler{body: body, etag: "stable-etag"}
			srv := httptest.NewServer(h)
			t.Cleanup(srv.Close)

			storePath := filepath.Join(helpers.TmpDirWOSymlinks(t), "store")
			v := venvtest.NewOSWithEmptyEnv()

			fetch := func(c *tgcas.CAS, dst string) error {
				g := getter.NewCASGetter(logger.CreateLogger(), c, v, &tgcas.CloneOptions{},
					getter.WithDefaultGenericDispatch(getter.WithHTTPClient(vhttp.NewOSClient())))
				client := &gogetter.Client{Getters: []gogetter.Getter{g}}

				_, err := client.Get(t.Context(), &gogetter.Request{
					Src:     srv.URL + tc.path,
					Dst:     dst,
					GetMode: gogetter.ModeAny,
				})

				return err
			}

			online := func() error {
				c, err := tgcas.New(venvtest.NewWithOSFS(), tgcas.WithStorePath(storePath), tgcas.WithProbeCache())
				require.NoError(t, err)

				return fetch(c, filepath.Join(t.TempDir(), "online"))
			}

			if tc.prime == primeStore {
				require.NoError(t, online())
			}

			if tc.prime == primeProbe {
				var checksumErr *gogetter.ChecksumError

				require.ErrorAs(t, online(), &checksumErr)
			}

			offline, err := tgcas.New(venvtest.NewWithOSFS(), tgcas.WithStorePath(storePath), tgcas.WithOffline())
			require.NoError(t, err)

			gets, heads := h.gets.Load(), h.heads.Load()
			dst := filepath.Join(t.TempDir(), "offline")
			err = fetch(offline, dst)

			assert.Equal(t, gets, h.gets.Load(), "the offline run must not download")
			assert.Equal(t, heads, h.heads.Load(), "the offline run must not probe either")

			if tc.prime != primeStore {
				require.ErrorIs(t, err, tgcas.ErrCASOffline)
				require.NoFileExists(t, filepath.Join(dst, "main.tf"))

				return
			}

			require.NoError(t, err)
			require.FileExists(t, filepath.Join(dst, "main.tf"), "the verified copy must be linked from the store")
		})
	}
}

// A failed inner download is reported once, by the bare getter alone.
func TestCASGetter_HTTPInnerFetchFailsOnce(t *testing.T) {
	t.Parallel()

	var gets atomic.Int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			gets.Add(1)
		}

		http.Error(w, "missing", http.StatusNotFound)
	}))
	t.Cleanup(srv.Close)

	storePath := filepath.Join(helpers.TmpDirWOSymlinks(t), "store")
	c, err := tgcas.New(venvtest.NewWithOSFS(), tgcas.WithStorePath(storePath))
	require.NoError(t, err)

	v := venvtest.NewOSWithEmptyEnv()
	g := getter.NewCASGetter(logger.CreateLogger(), c, v, &tgcas.CloneOptions{},
		getter.WithDefaultGenericDispatch(getter.WithHTTPClient(vhttp.NewOSClient())))
	client := &gogetter.Client{Getters: []gogetter.Getter{g}}

	source := "http://user:secret@" + strings.TrimPrefix(srv.URL, "http://") + "/mod.tar.gz"

	_, err = client.Get(t.Context(), &gogetter.Request{
		Src:     source,
		Dst:     filepath.Join(t.TempDir(), "dst"),
		GetMode: gogetter.ModeAny,
	})
	require.Error(t, err)

	assert.Equal(t, int32(1), gets.Load(), "the stock http getter must not retry the bare download")
	assert.NotContains(t, err.Error(), "secret", "the error must not carry the URL credentials")
}

// A fallback getter sees the source as written after the CAS getter fails.
func TestCASGetter_HTTPFailureRestoresSourceForFallbackGetters(t *testing.T) {
	t.Parallel()

	body := makeTarGz(t, map[string]string{"main.tf": "ok"})
	srv := httptest.NewServer(&tarballHandler{body: body, etag: "stable-etag"})
	t.Cleanup(srv.Close)

	storePath := filepath.Join(helpers.TmpDirWOSymlinks(t), "store")
	c, err := tgcas.New(venvtest.NewWithOSFS(), tgcas.WithStorePath(storePath))
	require.NoError(t, err)

	v := venvtest.NewOSWithEmptyEnv()
	g := getter.NewCASGetter(logger.CreateLogger(), c, v, &tgcas.CloneOptions{},
		getter.WithDefaultGenericDispatch(getter.WithHTTPClient(vhttp.NewOSClient())))
	fallback := &forcedRequiredGetter{scheme: getter.SchemeHTTP}
	client := &gogetter.Client{Getters: []gogetter.Getter{g, fallback}}

	source := srv.URL + "/mod.tar.gz?checksum=sha256:" + strings.Repeat("0", 64)

	_, err = client.Get(t.Context(), &gogetter.Request{
		Src:     source,
		Dst:     filepath.Join(t.TempDir(), "dst"),
		GetMode: gogetter.ModeAny,
	})
	require.Error(t, err)

	seen := fallback.lastSrc.Load()
	require.NotNil(t, seen, "the fallback getter must run after the CAS failure")
	assert.Equal(t, source, *seen, "the fallback getter must see the checksum the user wrote")
}

// TestCASGetter_HTTPChecksumFileFetchesThroughOtherSchemes: a checksum=file: reference on another scheme is read.
func TestCASGetter_HTTPChecksumFileFetchesThroughOtherSchemes(t *testing.T) {
	t.Parallel()

	body := makeTarGz(t, map[string]string{"main.tf": "ok"})
	sum := sha256.Sum256(body)

	testCases := []struct {
		name    string
		sums    string
		wantErr bool
	}{
		{name: "matching checksum file", sums: hex.EncodeToString(sum[:]) + "  mod.tar.gz\n"},
		{name: "mismatched checksum file", sums: strings.Repeat("0", 64) + "  mod.tar.gz\n", wantErr: true},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			srv := httptest.NewServer(&tarballHandler{body: body, etag: "stable-etag"})
			t.Cleanup(srv.Close)

			sumsPath := filepath.Join(helpers.TmpDirWOSymlinks(t), "mod.sha256")
			require.NoError(t, os.WriteFile(sumsPath, []byte(tc.sums), 0o644))

			storePath := filepath.Join(helpers.TmpDirWOSymlinks(t), "store")
			c, err := tgcas.New(venvtest.NewWithOSFS(), tgcas.WithStorePath(storePath))
			require.NoError(t, err)

			v := venvtest.NewOSWithEmptyEnv()
			g := getter.NewCASGetter(logger.CreateLogger(), c, v, &tgcas.CloneOptions{},
				getter.WithDefaultGenericDispatch(getter.WithHTTPClient(vhttp.NewOSClient())))
			client := &gogetter.Client{Getters: []gogetter.Getter{g}}
			dst := filepath.Join(t.TempDir(), "out")

			_, err = client.Get(t.Context(), &gogetter.Request{
				Src:     srv.URL + "/mod.tar.gz?checksum=file:" + getter.FileURL(sumsPath).String(),
				Dst:     dst,
				GetMode: gogetter.ModeAny,
			})

			if tc.wantErr {
				var checksumErr *gogetter.ChecksumError
				require.ErrorAs(t, err, &checksumErr, "a checksum file that disagrees must fail the download")
				require.NoFileExists(t, filepath.Join(dst, "main.tf"))

				return
			}

			require.NoError(t, err)
			require.FileExists(t, filepath.Join(dst, "main.tf"), "the inner client must read the checksum file over file://")
		})
	}
}

// TestCASGetter_HTTPChecksumFileUsesConfiguredClient: an https checksum file is read with the configured client.
func TestCASGetter_HTTPChecksumFileUsesConfiguredClient(t *testing.T) {
	t.Parallel()

	body := makeTarGz(t, map[string]string{"main.tf": "ok"})
	sum := sha256.Sum256(body)

	srv := httptest.NewServer(&tarballHandler{body: body, etag: "stable-etag"})
	t.Cleanup(srv.Close)

	sums := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if _, err := w.Write([]byte(hex.EncodeToString(sum[:]) + "  mod.tar.gz\n")); err != nil {
			panic(err)
		}
	}))
	t.Cleanup(sums.Close)

	storePath := filepath.Join(helpers.TmpDirWOSymlinks(t), "store")
	c, err := tgcas.New(venvtest.NewWithOSFS(), tgcas.WithStorePath(storePath))
	require.NoError(t, err)

	v := venvtest.NewOSWithEmptyEnv()
	g := getter.NewCASGetter(logger.CreateLogger(), c, v, &tgcas.CloneOptions{},
		getter.WithDefaultGenericDispatch(getter.WithHTTPClient(sums.Client())))
	client := &gogetter.Client{Getters: []gogetter.Getter{g}}
	dst := filepath.Join(t.TempDir(), "out")

	_, err = client.Get(t.Context(), &gogetter.Request{
		Src:     srv.URL + "/mod.tar.gz?checksum=file:" + sums.URL + "/mod.sha256",
		Dst:     dst,
		GetMode: gogetter.ModeAny,
	})
	require.NoError(t, err)
	require.FileExists(t, filepath.Join(dst, "main.tf"))
}

// TestCASGetter_CustomFetcherNeedsNoHTTP: a registered fetcher runs on a venv without an HTTP handle.
func TestCASGetter_CustomFetcherNeedsNoHTTP(t *testing.T) {
	t.Parallel()

	const scheme = "fakescheme"

	stub := &forcedRequiredGetter{scheme: scheme}

	storePath := filepath.Join(helpers.TmpDirWOSymlinks(t), "store")
	c, err := tgcas.New(venvtest.NewWithOSFS(), tgcas.WithStorePath(storePath))
	require.NoError(t, err)

	v := venvtest.NewOSWithEmptyEnv()
	v.HTTP = nil

	g := getter.NewCASGetter(logger.CreateLogger(), c, v, &tgcas.CloneOptions{},
		getter.WithGenericFetchers(map[string]gogetter.Getter{scheme: stub}))
	client := &gogetter.Client{Getters: []gogetter.Getter{g}}
	dst := filepath.Join(t.TempDir(), "out")

	_, err = client.Get(t.Context(), &gogetter.Request{
		Src:     scheme + "::http://example.com/catalog",
		Dst:     dst,
		GetMode: gogetter.ModeAny,
	})
	require.NoError(t, err)
	require.FileExists(t, filepath.Join(dst, "main.tf"))
	assert.Positive(t, stub.forcedCalls.Load())
}
