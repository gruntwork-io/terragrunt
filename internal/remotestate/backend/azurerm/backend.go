// Package azurerm implements the Azure Storage (azurerm) remote-state
// backend. Bootstrap, migration and teardown lifecycle operations are
// gated behind the azure-backend experiment; when the experiment is
// disabled only Name() and GetTFInitArgs() are functional so that
// terragrunt init can still pass an azurerm backend block through to
// terraform unchanged.
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

// GetTFInitArgs returns the config filtered to the keys the terraform
// azurerm backend understands (terragrunt-only keys removed).
func (b *Backend) GetTFInitArgs(config backend.Config) map[string]any {
	return Config(config).GetTFInitArgs()
}
