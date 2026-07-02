//go:build azure

package azurehelper_test

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"testing"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/stretchr/testify/assert"

	"github.com/gruntwork-io/terragrunt/internal/azurehelper"
)

func respErr(status int, code string) error {
	return &azcore.ResponseError{StatusCode: status, ErrorCode: code}
}

func TestIsRetryable(t *testing.T) {
	t.Parallel()

	tests := []struct {
		err  error
		name string
		want bool
	}{
		{nil, "nil", false},
		{errors.New("dial tcp: timeout"), "non-azure (network)", true},
		{context.Canceled, "context.Canceled", false},
		{context.DeadlineExceeded, "context.DeadlineExceeded", false},
		{fmt.Errorf("wrap: %w", context.Canceled), "wrapped context.Canceled", false},
		{respErr(http.StatusUnauthorized, ""), "401", false},
		{respErr(http.StatusNotFound, ""), "404", false},
		{respErr(http.StatusTooManyRequests, ""), "429", true},
		{respErr(http.StatusInternalServerError, ""), "500", true},
		{respErr(http.StatusServiceUnavailable, ""), "503", true},
		{respErr(http.StatusBadRequest, ""), "400", false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tc.want, azurehelper.IsRetryable(tc.err))
		})
	}
}

func TestIsNotFound(t *testing.T) {
	t.Parallel()

	tests := []struct {
		err  error
		name string
		want bool
	}{
		{nil, "nil", false},
		{errors.New("nope"), "non-azure", false},
		{respErr(http.StatusNotFound, ""), "404", true},
		{respErr(http.StatusInternalServerError, "ResourceNotFound"), "ResourceNotFound code", true},
		{respErr(http.StatusOK, "resourcenotfound"), "lower-case ResourceNotFound code", true},
		{respErr(http.StatusOK, "BlobNotFound"), "BlobNotFound code", true},
		{respErr(http.StatusOK, "ContainerNotFound"), "ContainerNotFound code", true},
		{respErr(http.StatusForbidden, ""), "403", false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tc.want, azurehelper.IsNotFound(tc.err))
		})
	}
}
