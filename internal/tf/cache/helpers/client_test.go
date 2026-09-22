package helpers_test

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/tf/cache/helpers"
	"github.com/gruntwork-io/terragrunt/internal/vhttp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const versionsListing = `{"versions":[{"version":"5.0.0"},{"version":"5.1.0"}]}`

// testLimit stands in for the real body limit, which a test would otherwise
// have to allocate megabytes to reach.
const testLimit = 64

// TestReadBodyIgnoresTheHint pins that the body decides what comes back. A
// Content-Length is the hint's usual source and a registry picks that freely,
// so reading only as far as the hint would let one truncate a listing into a
// shorter listing that still parses.
func TestReadBodyIgnoresTheHint(t *testing.T) {
	t.Parallel()

	body := versionsListing

	for name, hint := range map[string]int64{
		"unknown length": -1,
		"declared empty": 0,
		"understated":    int64(len(body) / 2),
		"exact":          int64(len(body)),
		"overstated":     int64(len(body) * 2),
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			got, err := helpers.ReadBody(strings.NewReader(body), hint, testLimit)
			require.NoError(t, err)
			assert.Equal(t, body, string(got))
		})
	}
}

// TestReadBodyRefusesABodyPastTheLimit pins that the limit bounds the read
// itself. A registry declaring nothing, or gzipping its response, arrives with
// no usable hint and would otherwise be read without a bound.
func TestReadBodyRefusesABodyPastTheLimit(t *testing.T) {
	t.Parallel()

	body := bytes.Repeat([]byte("x"), testLimit+1)

	_, err := helpers.ReadBody(bytes.NewReader(body), -1, testLimit)

	var tooLarge helpers.ResponseTooLargeError

	require.ErrorAs(t, err, &tooLarge)
	assert.Equal(t, int64(testLimit), tooLarge.Limit)
}

// TestReadBodyKeepsABodyOnTheLimit pins the boundary, so the limit is the
// largest body accepted rather than one byte below it.
func TestReadBodyKeepsABodyOnTheLimit(t *testing.T) {
	t.Parallel()

	body := bytes.Repeat([]byte("x"), testLimit)

	got, err := helpers.ReadBody(bytes.NewReader(body), testLimit, testLimit)
	require.NoError(t, err)
	assert.Len(t, got, testLimit)
}

// TestReadBodySurfacesATruncatedTransfer pins that a read ending early fails
// the call. The bytes that did arrive can parse as a shorter document, which
// would answer with versions the registry never published.
func TestReadBodySurfacesATruncatedTransfer(t *testing.T) {
	t.Parallel()

	cut := errors.New("connection reset")

	_, err := helpers.ReadBody(errAfter(versionsListing[:10], cut), -1, testLimit)
	require.ErrorIs(t, err, cut)
}

// TestClientDoDecodesAResponseItsContentLengthUnderstates pins the whole path
// for a header a registry can understate at will.
func TestClientDoDecodesAResponseItsContentLengthUnderstates(t *testing.T) {
	t.Parallel()

	body := []byte(versionsListing)

	c := vhttp.NewMemClient(func(_ context.Context, _ *http.Request) (*http.Response, error) {
		resp := vhttp.Respond(http.StatusOK, body, nil)
		resp.ContentLength = int64(len(body) / 2)

		return resp, nil
	})

	var value struct {
		Versions []struct {
			Version string `json:"version"`
		} `json:"versions"`
	}

	client := helpers.NewClient(c, nil)
	require.NoError(t, client.Do(
		t.Context(),
		http.MethodGet,
		"https://registry.test/v1/providers/hashicorp/aws/versions",
		&value,
	))

	assert.Len(t, value.Versions, 2)
}

// errAfter returns a reader that yields prefix and then fails, standing in for
// a transfer the transport reports as cut short.
func errAfter(prefix string, err error) *truncatedReader {
	return &truncatedReader{prefix: strings.NewReader(prefix), err: err}
}

type truncatedReader struct {
	prefix *strings.Reader
	err    error
}

func (r *truncatedReader) Read(p []byte) (int, error) {
	if r.prefix.Len() > 0 {
		return r.prefix.Read(p)
	}

	return 0, r.err
}
