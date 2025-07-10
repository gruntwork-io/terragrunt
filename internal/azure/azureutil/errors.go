package azureutil

import (
	"fmt"
	"strings"
)

// Resource types for error handling
const (
	ResourceTypeBlob          = "blob"
	ResourceTypeContainer     = "container"
	ResourceTypeResourceGroup = "resource_group"
	ResourceTypeStorage       = "storage_account"
)

// IsPermissionError checks if the given error indicates a permission issue
func IsPermissionError(err error) bool {
	// Check common Azure permissions error patterns
	if err == nil {
		return false
	}

	// Convert to string for pattern matching
	errStr := err.Error()

	return strings.Contains(strings.ToLower(errStr), "unauthorized") ||
		strings.Contains(strings.ToLower(errStr), "forbidden") ||
		strings.Contains(strings.ToLower(errStr), "permission") ||
		strings.Contains(strings.ToLower(errStr), "access denied") ||
		strings.Contains(strings.ToLower(errStr), "authentication failed") ||
		strings.Contains(strings.ToLower(errStr), "insufficient privileges")
}

// WrapBlobError wraps a blob-related error with context
func WrapBlobError(err error, container, key string) error {
	if err == nil {
		return nil
	}

	if IsPermissionError(err) {
		return newPermissionError(
			fmt.Sprintf("Permission denied while accessing blob %s in container %s", key, container),
			WithError(err),
			WithSuggestion("Check that you have the Storage Blob Data Reader/Contributor role."),
			WithClassification(ErrorClassPermission),
			WithResourceType(ResourceTypeBlob),
			WithResourceName(key),
		)
	}

	return newGenericError(
		fmt.Sprintf("Error operating on blob %s in container %s", key, container),
		WithError(err),
		WithClassification(ClassifyError(err)),
		WithResourceType(ResourceTypeBlob),
		WithResourceName(key),
	)
}

// WrapResourceGroupError wraps a resource group-related error with context
func WrapResourceGroupError(err error, resourceGroupName string) error {
	if err == nil {
		return nil
	}

	if IsPermissionError(err) {
		return newPermissionError(
			fmt.Sprintf("Permission denied while managing resource group %s", resourceGroupName),
			WithError(err),
			WithSuggestion("Check that you have the Contributor or Owner role on the resource group or subscription."),
			WithClassification(ErrorClassPermission),
			WithResourceType(ResourceTypeResourceGroup),
			WithResourceName(resourceGroupName),
		)
	}

	return newGenericError(
		fmt.Sprintf("Error managing resource group %s", resourceGroupName),
		WithError(err),
		WithClassification(ClassifyError(err)),
		WithResourceType(ResourceTypeResourceGroup),
		WithResourceName(resourceGroupName),
	)
}
