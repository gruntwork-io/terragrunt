package azurehelper

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
)

// Sentinel errors for input validation. Match with errors.Is.
var (
	ErrAzureConfigRequired          = errors.New("azure config is required")
	ErrStorageAccountRequired       = errors.New("storage account name is required")
	ErrContainerNameRequired        = errors.New("container name is required")
	ErrBlobKeyRequired              = errors.New("container name and blob key are required")
	ErrNoContainerBound             = errors.New("BlobClient has no container bound; call BindContainer first or use GetBlob")
	ErrCopyBlobArgsRequired         = errors.New("source and destination container/key are required")
	ErrSubscriptionIDRequired       = errors.New("subscription_id is required")
	ErrResourceGroupNameRequired    = errors.New("resource group name is required")
	ErrStorageAccountConfigRequired = errors.New("storage account config is required")
	ErrLocationRequiredForRG        = errors.New("location is required to create resource group")
	ErrNoAccessKeysReturned         = errors.New("no access keys returned for storage account")
	ErrAllAccessKeysEmpty           = errors.New("storage account returned keys but all values were empty")
)

// CredentialMissingError is returned when a token-credential auth method
// is requested but cfg.Credential is nil. Match with errors.As.
type CredentialMissingError struct {
	Method AuthMethod
}

func (e *CredentialMissingError) Error() string {
	return fmt.Sprintf("azure config has no credential for method %q", e.Method)
}

// UnsupportedAuthMethodError is returned when cfg.Method is not one of the
// supported AuthMethod constants. Match with errors.As.
type UnsupportedAuthMethodError struct {
	Method AuthMethod
}

func (e *UnsupportedAuthMethodError) Error() string {
	return fmt.Sprintf("unsupported azure auth method %q", e.Method)
}

// UnsupportedAuthForOpError is returned when an auth method is unsuitable
// for a given operation (e.g. SAS-token auth attempting RBAC operations).
type UnsupportedAuthForOpError struct {
	Method    AuthMethod
	Operation string
}

func (e *UnsupportedAuthForOpError) Error() string {
	return fmt.Sprintf("%s require a token credential (auth method %q is not supported)", e.Operation, e.Method)
}

// UnknownCloudEnvironmentError is returned for an unrecognised CloudEnvironment string.
type UnknownCloudEnvironmentError struct {
	Name string
}

func (e *UnknownCloudEnvironmentError) Error() string {
	return fmt.Sprintf("unknown cloud environment %q (want one of: public, government, china)", e.Name)
}

// UnknownAccessTierError is returned for a StorageAccountConfig.AccessTier
// outside the supported set.
type UnknownAccessTierError struct {
	Tier string
}

func (e *UnknownAccessTierError) Error() string {
	return fmt.Sprintf("unknown access tier %q (want Hot, Cool, Cold, or Premium)", e.Tier)
}

// IsRetryable reports whether the error is one a caller may retry. This covers
// throttling (429), transient 5xx, and network-style errors that did not yield
// an azcore.ResponseError at all (treated as transient). Context cancellation
// and deadline-exceeded are caller-driven and are never retryable.
func IsRetryable(err error) bool {
	if err == nil {
		return false
	}

	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}

	respErr, ok := errors.AsType[*azcore.ResponseError](err)
	if !ok {
		return true
	}

	if respErr.StatusCode == http.StatusTooManyRequests {
		return true
	}

	if respErr.StatusCode >= http.StatusInternalServerError {
		return true
	}

	return false
}

// IsNotFound reports whether the error represents a 404 / ResourceNotFound /
// ContainerNotFound / BlobNotFound from an Azure API. Use this for idempotent
// "already-gone" paths where the specific kind of resource is not significant
// (e.g. role-assignment cleanup). For container- or blob-specific code paths,
// match the response code directly with errors.As + respErr.ErrorCode so that
// a "blob not found" error isn't silently treated as "container not found".
func IsNotFound(err error) bool {
	if err == nil {
		return false
	}

	respErr, ok := errors.AsType[*azcore.ResponseError](err)
	if !ok {
		return false
	}

	if respErr.StatusCode == http.StatusNotFound {
		return true
	}

	switch {
	case strings.EqualFold(respErr.ErrorCode, "ResourceNotFound"),
		strings.EqualFold(respErr.ErrorCode, "ContainerNotFound"),
		strings.EqualFold(respErr.ErrorCode, "BlobNotFound"):
		return true
	}

	return false
}

// isErrorCode reports whether err is an azcore.ResponseError with the given
// ErrorCode (case-insensitive). Use for narrow checks at call sites that
// distinguish e.g. "ContainerNotFound" from "BlobNotFound".
func isErrorCode(err error, code string) bool {
	respErr, ok := errors.AsType[*azcore.ResponseError](err)
	return ok && strings.EqualFold(respErr.ErrorCode, code)
}
