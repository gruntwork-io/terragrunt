package errors

import (
	"fmt"
)

// WrapBlobError wraps a blob-related error with context
func WrapBlobError(err error, container, key string) error {
	if err == nil {
		return nil
	}

	if IsPermissionError(err) {
		return NewPermissionError(
			fmt.Sprintf("Permission denied while accessing blob %s in container %s", key, container),
			WithError(err),
			WithSuggestion("Check that you have the Storage Blob Data Reader/Contributor role."),
			WithClassification(ErrorClassPermission),
			WithResourceType(ResourceTypeBlob),
			WithResourceName(key),
		)
	}

	return NewGenericError(
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
		return NewPermissionError(
			fmt.Sprintf("Permission denied while managing resource group %s", resourceGroupName),
			WithError(err),
			WithSuggestion("Check that you have the Contributor or Owner role on the resource group or subscription."),
			WithClassification(ErrorClassPermission),
			WithResourceType(ResourceTypeResourceGroup),
			WithResourceName(resourceGroupName),
		)
	}

	return NewGenericError(
		fmt.Sprintf("Error managing resource group %s", resourceGroupName),
		WithError(err),
		WithClassification(ClassifyError(err)),
		WithResourceType(ResourceTypeResourceGroup),
		WithResourceName(resourceGroupName),
	)
}

// WrapStorageAccountError wraps a storage account-related error with context
func WrapStorageAccountError(err error, accountName string) error {
	if err == nil {
		return nil
	}

	if IsPermissionError(err) {
		return NewPermissionError(
			fmt.Sprintf("Permission denied while managing storage account %s", accountName),
			WithError(err),
			WithSuggestion("Check that you have the Storage Account Contributor role and Storage Blob Data Owner role."),
			WithClassification(ErrorClassPermission),
			WithResourceType(ResourceTypeStorage),
			WithResourceName(accountName),
		)
	}

	return NewGenericError(
		fmt.Sprintf("Error managing storage account %s", accountName),
		WithError(err),
		WithClassification(ClassifyError(err)),
		WithResourceType(ResourceTypeStorage),
		WithResourceName(accountName),
	)
}

// WrapContainerError wraps a container-related error with context
func WrapContainerError(err error, containerName string) error {
	if err == nil {
		return nil
	}

	if IsPermissionError(err) {
		return NewPermissionError(
			fmt.Sprintf("Permission denied while managing container %s", containerName),
			WithError(err),
			WithSuggestion("Check that you have the Storage Blob Data Reader/Contributor role."),
			WithClassification(ErrorClassPermission),
			WithResourceType(ResourceTypeContainer),
			WithResourceName(containerName),
		)
	}

	return NewGenericError(
		fmt.Sprintf("Error managing container %s", containerName),
		WithError(err),
		WithClassification(ClassifyError(err)),
		WithResourceType(ResourceTypeContainer),
		WithResourceName(containerName),
	)
}

// WrapContainerDoesNotExistError wraps a container not found error with context
func WrapContainerDoesNotExistError(err error, containerName string) error {
	return NewGenericError(
		fmt.Sprintf("Container %s does not exist", containerName),
		WithError(err),
		WithClassification(ErrorClassNotFound),
		WithResourceType(ResourceTypeContainer),
		WithResourceName(containerName),
	)
}

// WrapAuthenticationError wraps an authentication error with context
func WrapAuthenticationError(err error, method string) error {
	return NewGenericError(
		fmt.Sprintf("Failed to authenticate using %s", method),
		WithError(err),
		WithClassification(ErrorClassAuthentication),
		WithSuggestion("Check your credentials and ensure you have the necessary permissions."),
	)
}
