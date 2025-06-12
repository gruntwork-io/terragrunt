package azurerm

import (
	"os"
	"testing"
)

// CheckAzureTestCredentials checks if the required Azure test credentials are available
// and skips the test if they are not. Returns the credentials if available.
func CheckAzureTestCredentials(t *testing.T) (storageAccount, accessKey string) {
	t.Helper() // Mark this as a test helper function

	storageAccount = os.Getenv("TERRAGRUNT_AZURE_TEST_STORAGE_ACCOUNT")
	accessKey = os.Getenv("TERRAGRUNT_AZURE_TEST_ACCESS_KEY")

	if storageAccount == "" || accessKey == "" {
		t.Skip("Skipping Azure test: TERRAGRUNT_AZURE_TEST_STORAGE_ACCOUNT or TERRAGRUNT_AZURE_TEST_ACCESS_KEY not set")
	}

	return storageAccount, accessKey
}
