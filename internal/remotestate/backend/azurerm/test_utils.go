package azurerm

import (
	"os"
	"testing"
)

// checkAzureTestCredentials checks if the required Azure test credentials are available
// and skips the test if they are not. Returns the credentials if available.
func checkAzureTestCredentials(t *testing.T) (storageAccount, accessKey string) {
	storageAccount = os.Getenv("TERRAGRUNT_AZURE_TEST_STORAGE_ACCOUNT")
	accessKey = os.Getenv("TERRAGRUNT_AZURE_TEST_ACCESS_KEY")

	if storageAccount == "" || accessKey == "" {
		t.Skip("Skipping Azure test: TERRAGRUNT_AZURE_TEST_STORAGE_ACCOUNT or TERRAGRUNT_AZURE_TEST_ACCESS_KEY not set")
	}
	return storageAccount, accessKey
}
