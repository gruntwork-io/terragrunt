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
		name string
		err  error
		want azurehelper.ErrorClass
	}{
		{"nil", nil, azurehelper.ErrorClassUnknown},
		{"non-azure", errors.New("boom"), azurehelper.ErrorClassUnknown},
		{"401", respErr(http.StatusUnauthorized, ""), azurehelper.ErrorClassAuthentication},
		{"403", respErr(http.StatusForbidden, ""), azurehelper.ErrorClassPermission},
		{"404", respErr(http.StatusNotFound, ""), azurehelper.ErrorClassNotFound},
		{"409", respErr(http.StatusConflict, ""), azurehelper.ErrorClassConflict},
		{"429", respErr(http.StatusTooManyRequests, ""), azurehelper.ErrorClassThrottling},
		{"400", respErr(http.StatusBadRequest, ""), azurehelper.ErrorClassInvalidRequest},
		{"500", respErr(http.StatusInternalServerError, ""), azurehelper.ErrorClassTransient},
		{"503", respErr(http.StatusServiceUnavailable, ""), azurehelper.ErrorClassTransient},
		{"418 unknown", respErr(http.StatusTeapot, ""), azurehelper.ErrorClassUnknown},
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
		name string
		err  error
		want bool
	}{
		{"nil", nil, false},
		{"non-azure (network)", errors.New("dial tcp: timeout"), true},
		{"401", respErr(http.StatusUnauthorized, ""), false},
		{"404", respErr(http.StatusNotFound, ""), false},
		{"429", respErr(http.StatusTooManyRequests, ""), true},
		{"500", respErr(http.StatusInternalServerError, ""), true},
		{"503", respErr(http.StatusServiceUnavailable, ""), true},
		{"400", respErr(http.StatusBadRequest, ""), false},
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
		name string
		err  error
		want bool
	}{
		{"nil", nil, false},
		{"non-azure", errors.New("nope"), false},
		{"404", respErr(http.StatusNotFound, ""), true},
		{"ResourceNotFound code", respErr(http.StatusInternalServerError, "ResourceNotFound"), true},
		{"BlobNotFound code", respErr(http.StatusOK, "BlobNotFound"), true},
		{"ContainerNotFound code", respErr(http.StatusOK, "ContainerNotFound"), true},
		{"403", respErr(http.StatusForbidden, ""), false},
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
