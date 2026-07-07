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
		{err: nil, name: "nil", want: false},
		{err: errors.New("dial tcp: timeout"), name: "non-azure (network)", want: true},
		{err: context.Canceled, name: "context.Canceled", want: false},
		{err: context.DeadlineExceeded, name: "context.DeadlineExceeded", want: false},
		{err: fmt.Errorf("wrap: %w", context.Canceled), name: "wrapped context.Canceled", want: false},
		{err: respErr(http.StatusUnauthorized, ""), name: "401", want: false},
		{err: respErr(http.StatusNotFound, ""), name: "404", want: false},
		{err: respErr(http.StatusTooManyRequests, ""), name: "429", want: true},
		{err: respErr(http.StatusInternalServerError, ""), name: "500", want: true},
		{err: respErr(http.StatusServiceUnavailable, ""), name: "503", want: true},
		{err: respErr(http.StatusBadRequest, ""), name: "400", want: false},
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
		{err: nil, name: "nil", want: false},
		{err: errors.New("nope"), name: "non-azure", want: false},
		{err: respErr(http.StatusNotFound, ""), name: "404", want: true},
		{err: respErr(http.StatusInternalServerError, "ResourceNotFound"), name: "ResourceNotFound code", want: true},
		{err: respErr(http.StatusOK, "BlobNotFound"), name: "BlobNotFound code", want: true},
		{err: respErr(http.StatusOK, "ContainerNotFound"), name: "ContainerNotFound code", want: true},
		{err: respErr(http.StatusForbidden, ""), name: "403", want: false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tc.want, azurehelper.IsNotFound(tc.err))
		})
	}
}
