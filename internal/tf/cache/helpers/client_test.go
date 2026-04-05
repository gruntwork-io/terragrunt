package helpers

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestDecodeResponse_429ReturnsError verifies that decodeResponse returns an error with rate-limiting details
// when the HTTP response status is 429 Too Many Requests.
func TestDecodeResponse_429ReturnsError(t *testing.T) {
	t.Parallel()

	req := httptest.NewRequest(http.MethodGet, "https://registry.terraform.io/v1/providers/hashicorp/aws/versions", nil)
	resp := &http.Response{
		StatusCode: http.StatusTooManyRequests,
		Request:    req,
	}

	data, err := decodeResponse(resp)
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

	req := httptest.NewRequest(http.MethodGet, "https://registry.terraform.io/v1/providers/hashicorp/aws/versions", nil)
	resp := &http.Response{
		StatusCode: http.StatusInternalServerError,
		Request:    req,
	}

	data, err := decodeResponse(resp)
	require.Error(t, err)
	assert.Nil(t, data)
	assert.Contains(t, err.Error(), "500")
	assert.Contains(t, err.Error(), "Internal Server Error")
}
