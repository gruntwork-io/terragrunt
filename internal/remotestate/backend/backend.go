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

	// Delete deletes the remote state.
	Delete(ctx context.Context, config Config, opts *options.TerragruntOptions) error

	// DeleteBucket deletes the entire bucket.
	DeleteBucket(ctx context.Context, config Config, opts *options.TerragruntOptions) error

	// GetTFInitArgs returns the config that should be passed on to `tofu -backend-config` cmd line param
	// Allows the Backends to filter and/or modify the configuration given from the user.
	GetTFInitArgs(config Config) map[string]any
}
