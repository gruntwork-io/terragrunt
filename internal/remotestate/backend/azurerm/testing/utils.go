// Package testing provides utilities for testing Azure remote state backends.
package testing

import (
	"os"
	"testing"
)

// CheckAzureTestCredentials checks if the required Azure test credentials are available and
// skips the current test if they are not present in the environment.
//
// This function looks for the following environment variables:
//   - TERRAGRUNT_AZURE_TEST_STORAGE_ACCOUNT: The Azure storage account name
//   - TERRAGRUNT_AZURE_TEST_ACCESS_KEY: The access key for the storage account
//
// Parameters:
//   - t *testing.T: The testing context
//
// Returns:
//   - storageAccount: The Azure storage account name from environment
//   - accessKey: The Azure storage account access key from environment
//
// If either of the required environment variables is not set, the test will be skipped
// with an appropriate message.
func CheckAzureTestCredentials(t *testing.T) (storageAccount, accessKey string) {
	t.Helper() // Mark this as a test helper function

	storageAccount = os.Getenv("TERRAGRUNT_AZURE_TEST_STORAGE_ACCOUNT")
	accessKey = os.Getenv("TERRAGRUNT_AZURE_TEST_ACCESS_KEY")

	if storageAccount == "" || accessKey == "" {
		t.Skip("Skipping Azure test: TERRAGRUNT_AZURE_TEST_STORAGE_ACCOUNT or TERRAGRUNT_AZURE_TEST_ACCESS_KEY not set")
	}

	return storageAccount, accessKey
}
