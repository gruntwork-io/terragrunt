package helpers_test

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/tf/cache/helpers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestDecodeResponse_429ReturnsError verifies that decodeResponse returns an error with rate-limiting details
// when the HTTP response status is 429 Too Many Requests.
func TestDecodeResponse_429ReturnsError(t *testing.T) {
	t.Parallel()

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, "https://registry.terraform.io/v1/providers/hashicorp/aws/versions", nil)
	require.NoError(t, err)

	resp := &http.Response{
		StatusCode: http.StatusTooManyRequests,
		Request:    req,
	}

	data, err := helpers.DecodeResponse(resp)
	require.Error(t, err)
	assert.Nil(t, data)
	assert.Contains(t, err.Error(), "429")
	assert.Contains(t, err.Error(), "Too Many Requests")
	assert.Contains(t, err.Error(), "rate limited")
}

// TestDecodeResponse_NonOKReturnsError verifies that decodeResponse returns an error with the HTTP status code
// and status text when the response has a non-200 status code.
func TestDecodeResponse_NonOKReturnsError(t *testing.T) {
	t.Parallel()

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, "https://registry.terraform.io/v1/providers/hashicorp/aws/versions", nil)
	require.NoError(t, err)

	resp := &http.Response{
		StatusCode: http.StatusInternalServerError,
		Request:    req,
	}

	data, err := helpers.DecodeResponse(resp)
	require.Error(t, err)
	assert.Nil(t, data)
	assert.Contains(t, err.Error(), "500")
	assert.Contains(t, err.Error(), "Internal Server Error")
}

// TestDecodeResponse_OKReturnsBody verifies that decodeResponse returns the response body
// when the HTTP response status is 200 OK.
func TestDecodeResponse_OKReturnsBody(t *testing.T) {
	t.Parallel()

	body := `{"versions":["1.0.0"]}`

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, "https://registry.terraform.io/v1/providers/hashicorp/aws/versions", nil)
	require.NoError(t, err)

	resp := &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader(body)),
		Request:    req,
	}

	data, err := helpers.DecodeResponse(resp)
	require.NoError(t, err)
	assert.Equal(t, body, string(data))
}
