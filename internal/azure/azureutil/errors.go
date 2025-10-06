package azureutil

import (
	"fmt"

	"github.com/gruntwork-io/terragrunt/internal/azure/errorutil"
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
	return errorutil.IsPermissionError(err)
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
			"Permission denied while managing resource group "+resourceGroupName,
			WithError(err),
			WithSuggestion("Check that you have the Contributor or Owner role on the resource group or subscription."),
			WithClassification(ErrorClassPermission),
			WithResourceType(ResourceTypeResourceGroup),
			WithResourceName(resourceGroupName),
		)
	}

	return newGenericError(
		"Error managing resource group "+resourceGroupName,
		WithError(err),
		WithClassification(ClassifyError(err)),
		WithResourceType(ResourceTypeResourceGroup),
		WithResourceName(resourceGroupName),
	)
}
