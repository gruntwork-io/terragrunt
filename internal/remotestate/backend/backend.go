// Package backend represents a backend for interacting with remote state.
package backend

import (
	"context"

	"github.com/gruntwork-io/terragrunt/options"
)

type Backends []Backend

func (backends Backends) Get(name string) Backend {
	for _, backend := range backends {
		if backend.Name() == name {
			return backend
		}
	}

	return nil
}

type Backend interface {
	// Names returns the backend name.
	Name() string

	// NeedsInit returns true if remote state needs to be initialized.
	NeedsInit(ctx context.Context, config Config, existingConfig Config, opts *options.TerragruntOptions) (bool, error)

	// Init initializes the remote state.
	Init(ctx context.Context, config Config, opts *options.TerragruntOptions) error

	// DeleteBucket deletes the remote state.
	DeleteBucket(ctx context.Context, config Config, opts *options.TerragruntOptions) error

	// GetTerraformInitArgs returns the config that should be passed on to terraform via -backend-config cmd line param
	// Allows the Backends to filter and/or modify the configuration given from the user.
	GetTerraformInitArgs(config Config) map[string]any
}
