package helpers_test

import (
	"compress/gzip"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/gruntwork-io/terragrunt/test/helpers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGzipHandler(t *testing.T) {
	t.Parallel()

	const body = "hello provider mirror"

	inner := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, body)
	})
	handler := helpers.GzipHandler(inner)

	t.Run("compresses when the client accepts gzip", func(t *testing.T) {
		t.Parallel()

		req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/aws.zip", nil)
		req.Header.Set("Accept-Encoding", "gzip")

		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		res := rec.Result()
		defer res.Body.Close()

		assert.Equal(t, "gzip", res.Header.Get("Content-Encoding"))
		assert.Empty(t, res.Header.Get("Content-Length"), "length is dropped once the body is compressed")

		gz, err := gzip.NewReader(res.Body)
		require.NoError(t, err)

		got, err := io.ReadAll(gz)
		require.NoError(t, err)
		assert.Equal(t, body, string(got))
	})

	t.Run("passes through when the client does not accept gzip", func(t *testing.T) {
		t.Parallel()

		req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/aws.zip", nil)

		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		res := rec.Result()
		defer res.Body.Close()

		assert.Empty(t, res.Header.Get("Content-Encoding"))

		got, err := io.ReadAll(res.Body)
		require.NoError(t, err)
		assert.Equal(t, body, string(got))
	})
}

func TestMustAbs(t *testing.T) {
	t.Parallel()

	got := helpers.MustAbs(t, "some/rel/path")
	assert.True(t, filepath.IsAbs(got), "MustAbs returns an absolute path")
	assert.True(t, filepath.IsAbs(helpers.MustAbs(t, ".")))
}

func TestGetPathRelativeTo(t *testing.T) {
	t.Parallel()

	assert.Equal(t, filepath.Join("b", "c"),
		helpers.GetPathRelativeTo(t, filepath.Join("/a", "b", "c"), "/a"))
}

func TestGetPathsRelativeTo(t *testing.T) {
	t.Parallel()

	paths := []string{filepath.Join("/a", "b"), filepath.Join("/a", "c", "d")}

	got := helpers.GetPathsRelativeTo(t, "/a", paths)
	assert.Equal(t, []string{"b", filepath.Join("c", "d")}, got)
}
