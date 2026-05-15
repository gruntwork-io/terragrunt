// Package azurerm represents the Azure Storage (azurerm) backend for interacting
// with remote state.
//
// This is a stub registration: it makes Terragrunt recognize backend = "azurerm"
// and routes configuration through the common backend abstraction. Bootstrap,
// delete, migrate and other lifecycle operations currently fall through to
// CommonBackend defaults (no-op). Experiment gating and functional lifecycle
// behavior for the Azure backend will be added in follow-up PRs.
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
