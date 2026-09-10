package helpers_test

import (
	"bytes"
	"compress/gzip"
	"context"
	"io"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/gruntwork-io/terragrunt/internal/tf/cache/helpers"
	"github.com/gruntwork-io/terragrunt/internal/vhttp"
)

// gzipped returns size bytes of compressible filler, gzip-encoded. Whatever
// size the caller asks for arrives as a few hundred bytes on the wire.
func gzipped(t *testing.T, size int) []byte {
	t.Helper()

	var buf bytes.Buffer

	w := gzip.NewWriter(&buf)

	_, err := w.Write(bytes.Repeat([]byte("a"), size))
	require.NoError(t, err)
	require.NoError(t, w.Close())

	return buf.Bytes()
}

func gzipClient(t *testing.T, decodedSize int) vhttp.Client {
	t.Helper()

	body := gzipped(t, decodedSize)

	return vhttp.NewMemClient(func(_ context.Context, _ *http.Request) (*http.Response, error) {
		return vhttp.Respond(http.StatusOK, body, http.Header{"Content-Encoding": []string{"gzip"}}), nil
	})
}

func TestFetchRefusesBodyPastLimit(t *testing.T) {
	t.Parallel()

	const limit = 1 << 10

	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, "https://registry.test/sums", nil)
	require.NoError(t, err)

	var dst bytes.Buffer

	err = helpers.Fetch(t.Context(), gzipClient(t, limit*64), req, &dst, limit)

	var tooLarge helpers.ResponseTooLargeError

	require.ErrorAs(t, err, &tooLarge)
	require.Equal(t, int64(limit), tooLarge.Limit)
}

func TestFetchAcceptsBodyAtLimit(t *testing.T) {
	t.Parallel()

	const limit = 1 << 10

	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, "https://registry.test/sums", nil)
	require.NoError(t, err)

	var dst bytes.Buffer

	require.NoError(t, helpers.Fetch(t.Context(), gzipClient(t, limit), req, &dst, limit))
	require.Equal(t, limit, dst.Len())
}

func TestResponseBufferRefusesBodyPastLimit(t *testing.T) {
	t.Parallel()

	resp := vhttp.Respond(
		http.StatusOK,
		gzipped(t, helpers.MaxJSONResponseBytes+1),
		http.Header{"Content-Encoding": []string{"gzip"}},
	)
	defer func() { require.NoError(t, resp.Body.Close()) }()

	buffer, err := helpers.ResponseBuffer(resp)

	var tooLarge helpers.ResponseTooLargeError

	require.ErrorAs(t, err, &tooLarge)
	require.Nil(t, buffer)
}

func TestResponseBufferReadsBodyWithinLimit(t *testing.T) {
	t.Parallel()

	resp := vhttp.Respond(http.StatusOK, gzipped(t, 64), http.Header{"Content-Encoding": []string{"gzip"}})
	defer func() { require.NoError(t, resp.Body.Close()) }()

	buffer, err := helpers.ResponseBuffer(resp)
	require.NoError(t, err)

	body, err := io.ReadAll(buffer)
	require.NoError(t, err)
	require.Len(t, body, 64)
}
