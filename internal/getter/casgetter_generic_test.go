package getter_test

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync/atomic"
	"testing"

	tgcas "github.com/gruntwork-io/terragrunt/internal/cas"
	"github.com/gruntwork-io/terragrunt/internal/getter"
	"github.com/gruntwork-io/terragrunt/test/helpers"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	gogetter "github.com/hashicorp/go-getter/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
	c, err := tgcas.New(tgcas.WithStorePath(storePath))
	require.NoError(t, err)

	v, err := tgcas.OSVenv()
	require.NoError(t, err)

	l := logger.CreateLogger()

	g := getter.NewCASGetter(l, c, v, &tgcas.CloneOptions{}, getter.WithDefaultGenericDispatch())

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
	assert.GreaterOrEqual(t, firstHeads, int32(1), "first run must probe via HEAD before downloading")

	runOnce(t)

	assert.Equal(t, firstGets, h.gets.Load(),
		"second run must hit the CAS via the probe and skip the archive GET")
	assert.Greater(t, h.heads.Load(), firstHeads,
		"second run must still probe to confirm the cached version is current")
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
	c, err := tgcas.New(tgcas.WithStorePath(storePath))
	require.NoError(t, err)

	v, err := tgcas.OSVenv()
	require.NoError(t, err)

	l := logger.CreateLogger()

	g := getter.NewCASGetter(l, c, v, &tgcas.CloneOptions{}, getter.WithDefaultGenericDispatch())

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
