//go:build azure

package azurehelper_test

import (
	"errors"
	"net/http"
	"testing"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"

	"github.com/gruntwork-io/terragrunt/internal/azurehelper"
)

func respErr(status int, code string) error {
	return &azcore.ResponseError{StatusCode: status, ErrorCode: code}
}

func TestClassifyError(t *testing.T) {
	t.Parallel()

	tests := []struct {
		err  error
		name string
		want azurehelper.ErrorClass
	}{
		{nil, "nil", azurehelper.ErrorClassUnknown},
		{errors.New("boom"), "non-azure", azurehelper.ErrorClassUnknown},
		{respErr(http.StatusUnauthorized, ""), "401", azurehelper.ErrorClassAuthentication},
		{respErr(http.StatusForbidden, ""), "403", azurehelper.ErrorClassPermission},
		{respErr(http.StatusNotFound, ""), "404", azurehelper.ErrorClassNotFound},
		{respErr(http.StatusConflict, ""), "409", azurehelper.ErrorClassConflict},
		{respErr(http.StatusTooManyRequests, ""), "429", azurehelper.ErrorClassThrottling},
		{respErr(http.StatusBadRequest, ""), "400", azurehelper.ErrorClassInvalidRequest},
		{respErr(http.StatusInternalServerError, ""), "500", azurehelper.ErrorClassTransient},
		{respErr(http.StatusServiceUnavailable, ""), "503", azurehelper.ErrorClassTransient},
		{respErr(http.StatusTeapot, ""), "418 unknown", azurehelper.ErrorClassUnknown},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			if got := azurehelper.ClassifyError(tc.err); got != tc.want {
				t.Errorf("ClassifyError = %q, want %q", got, tc.want)
			}
		})
	}
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

			if got := azurehelper.IsRetryable(tc.err); got != tc.want {
				t.Errorf("IsRetryable = %v, want %v", got, tc.want)
			}
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
		{respErr(http.StatusOK, "BlobNotFound"), "BlobNotFound code", true},
		{respErr(http.StatusOK, "ContainerNotFound"), "ContainerNotFound code", true},
		{respErr(http.StatusForbidden, ""), "403", false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			if got := azurehelper.IsNotFound(tc.err); got != tc.want {
				t.Errorf("IsNotFound = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestWrapError(t *testing.T) {
	t.Parallel()

	if got := azurehelper.WrapError(nil, "op"); got != nil {
		t.Errorf("WrapError(nil) = %v, want nil", got)
	}

	base := errors.New("inner")

	wrapped := azurehelper.WrapError(base, "creating container")
	if wrapped == nil {
		t.Fatal("WrapError returned nil for non-nil input")
	}

	if !errors.Is(wrapped, base) {
		t.Errorf("errors.Is should return true for wrapped error")
	}

	if msg := wrapped.Error(); msg == "" {
		t.Error("wrapped error has empty message")
	}
}
