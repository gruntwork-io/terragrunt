package getter_test

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/getter"
	"github.com/gruntwork-io/terragrunt/test/helpers"
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
	res, err := getter.GetFile(t.Context(), dst, server.URL+"/blob")
	require.NoError(t, err)
	assert.Equal(t, dst, res.Dst)

	got, err := os.ReadFile(dst)
	require.NoError(t, err)
	assert.Equal(t, body, string(got))
}

// TestGetAnyConvenience pins [getter.GetAny] against a directory source so the
// wrapper's ModeAny + Client.Get path is exercised end-to-end.
func TestGetAnyConvenience(t *testing.T) {
	t.Parallel()

	src := helpers.TmpDirWOSymlinks(t)
	require.NoError(t, os.WriteFile(filepath.Join(src, "main.tf"), []byte("# fixture\n"), 0644))

	dst := filepath.Join(helpers.TmpDirWOSymlinks(t), "copy")
	_, err := getter.GetAny(t.Context(), dst, "file://"+src,
		getter.WithFileCopy(getter.NewFileCopyGetter()),
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
	_, err := getter.Get(t.Context(), dst, "file://"+src,
		getter.WithFileCopy(getter.NewFileCopyGetter()),
	)
	require.NoError(t, err)

	assert.FileExists(t, filepath.Join(dst, "main.tf"))
}

// TestNewClientWithDecompressorsEmptyMap verifies that passing a non-nil empty
// map disables archive decompression while a nil map keeps the v2 defaults.
func TestNewClientWithDecompressorsEmptyMap(t *testing.T) {
	t.Parallel()

	disabled := getter.NewClient(getter.WithDecompressors(map[string]getter.Decompressor{}))
	require.NotNil(t, disabled.Decompressors)
	assert.Empty(t, disabled.Decompressors)

	def := getter.NewClient(getter.WithDecompressors(nil))
	assert.Nil(t, def.Decompressors, "nil map must leave the v2 default decompressors untouched")
}
