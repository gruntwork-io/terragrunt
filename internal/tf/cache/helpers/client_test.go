package helpers

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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
