package azurerm_test

import (
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/remotestate/backend/azurerm"
)

func TestBackendName(t *testing.T) {
	t.Parallel()

	if got := azurerm.NewBackend().Name(); got != azurerm.BackendName {
		t.Fatalf("Name() = %q, want %q", got, azurerm.BackendName)
	}
}
