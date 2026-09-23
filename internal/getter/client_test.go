package getter_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/gruntwork-io/terragrunt/internal/vhttp"

	"github.com/gruntwork-io/terragrunt/internal/getter"
	"github.com/gruntwork-io/terragrunt/test/helpers"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/gruntwork-io/terragrunt/test/helpers/venvtest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestGetFileConvenience exercises [getter.GetFile], the top-level
// convenience wrapper used by callers like [internal/github].
func TestGetFileConvenience(t *testing.T) {
	t.Parallel()

	const body = "hello, downloader\n"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, err := w.Write([]byte(body))
		assert.NoError(t, err)
	}))
	t.Cleanup(server.Close)

	dst := filepath.Join(helpers.TmpDirWOSymlinks(t), "out.txt")
	v := venvtest.NewWithOSFS().WithHTTP(vhttp.NewOSClient())

	res, err := getter.GetFile(t.Context(), logger.CreateLogger(), v, dst, server.URL+"/blob")
	require.NoError(t, err)
	assert.Equal(t, dst, res.Dst)

	got, err := os.ReadFile(dst)
	require.NoError(t, err)
	assert.Equal(t, body, string(got))
}

// TestNewClientFetchesThroughVenvHTTP pins that an http source goes through
// the venv's client with no option passed, so a call site that forgets one
// cannot reach the network outside the venv.
func TestNewClientFetchesThroughVenvHTTP(t *testing.T) {
	t.Parallel()

	const body = "through the venv\n"

	var calls atomic.Int64

	v := venvtest.NewWithOSFS().WithHTTP(vhttp.NewMemClient(
		func(_ context.Context, _ *http.Request) (*http.Response, error) {
			calls.Add(1)

			return vhttp.Respond(http.StatusOK, []byte(body), nil), nil
		}))

	dst := filepath.Join(helpers.TmpDirWOSymlinks(t), "out.txt")

	_, err := getter.GetFile(t.Context(), logger.CreateLogger(), v, dst, "http://getter.invalid/blob")
	require.NoError(t, err)

	got, err := os.ReadFile(dst)
	require.NoError(t, err)
	assert.Equal(t, body, string(got))
	assert.Positive(t, calls.Load())
}

// TestGetAnyConvenience pins [getter.GetAny] against a directory source so the
// wrapper's ModeAny + Client.Get path is exercised end-to-end.
func TestGetAnyConvenience(t *testing.T) {
	t.Parallel()

	src := helpers.TmpDirWOSymlinks(t)
	require.NoError(t, os.WriteFile(filepath.Join(src, "main.tf"), []byte("# fixture\n"), 0644))

	dst := filepath.Join(helpers.TmpDirWOSymlinks(t), "copy")
	_, err := getter.GetAny(
		t.Context(),
		logger.CreateLogger(),
		venvtest.NewWithOSFS(),
		dst,
		helpers.FileURL(src),
		getter.WithFileCopy(getter.NewFileCopyGetter(vfs.NewOSFS())),
	)
	require.NoError(t, err)

	got, err := os.ReadFile(filepath.Join(dst, "main.tf"))
	require.NoError(t, err)
	assert.Equal(t, "# fixture\n", string(got))
}

// TestGetConvenience pins [getter.Get] (ModeDir) against a local directory.
func TestGetConvenience(t *testing.T) {
	t.Parallel()

	src := helpers.TmpDirWOSymlinks(t)
	require.NoError(t, os.WriteFile(filepath.Join(src, "main.tf"), []byte("# fixture\n"), 0644))

	dst := filepath.Join(helpers.TmpDirWOSymlinks(t), "copy")
	_, err := getter.Get(
		t.Context(),
		logger.CreateLogger(),
		venvtest.NewWithOSFS(),
		dst,
		helpers.FileURL(src),
		getter.WithFileCopy(getter.NewFileCopyGetter(vfs.NewOSFS())),
	)
	require.NoError(t, err)

	assert.FileExists(t, filepath.Join(dst, "main.tf"))
}

// TestNewClientWithDecompressorsEmptyMap verifies that passing a non-nil empty
// map disables archive decompression while a nil map keeps the v2 defaults.
func TestNewClientWithDecompressorsEmptyMap(t *testing.T) {
	t.Parallel()

	disabled := getter.NewClient(logger.CreateLogger(), venvtest.NewWithOSFS(),
		getter.WithDecompressors(map[string]getter.Decompressor{}),
	)
	require.NotNil(t, disabled.Decompressors)
	assert.Empty(t, disabled.Decompressors)

	def := getter.NewClient(logger.CreateLogger(), venvtest.NewWithOSFS(),
		getter.WithDecompressors(nil))
	assert.Nil(t, def.Decompressors, "nil map must leave the v2 default decompressors untouched")
}
