// Package azurerm represents the Azure Storage (azurerm) backend for interacting
// with remote state.
//
// This is a stub registration: it makes Terragrunt recognize backend = "azurerm"
// and routes configuration through the common backend abstraction. Bootstrap,
// delete, migrate and other lifecycle operations fall through to CommonBackend
// defaults (no-op) and are gated behind the `azure-backend` experiment, which
// will deliver functional behavior in subsequent PRs.
package azurerm

import (
	"github.com/gruntwork-io/terragrunt/internal/remotestate/backend"
)

const BackendName = "azurerm"

var _ backend.Backend = new(Backend)

type Backend struct {
	*backend.CommonBackend
}

func NewBackend() *Backend {
	return &Backend{
		CommonBackend: backend.NewCommonBackend(BackendName),
	}
}
