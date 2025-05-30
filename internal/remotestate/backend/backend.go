// Package backend represents a backend for interacting with remote state.
package backend

import (
	"context"

	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/pkg/log"
)

type Backends []Backend

// Get returns the backend by the given name.
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

	// IsVersionControlEnabled returns true if the version control is enabled.
	IsVersionControlEnabled(ctx context.Context, l log.Logger, config Config, opts *options.TerragruntOptions) (bool, error)

	// NeedsBootstrap returns true if remote state needs to be bootstrapped.
	NeedsBootstrap(ctx context.Context, l log.Logger, config Config, opts *options.TerragruntOptions) (bool, error)

	// Bootstrap bootstraps the remote state.
	Bootstrap(ctx context.Context, l log.Logger, config Config, opts *options.TerragruntOptions) error

	// Migrate determines where the remote state resources exist for source backend config and migrate them to dest backend config.
	Migrate(ctx context.Context, l log.Logger, srcConfig, dstConfig Config, opts *options.TerragruntOptions) error

	// Delete deletes the remote state.
	Delete(ctx context.Context, l log.Logger, config Config, opts *options.TerragruntOptions) error

	// DeleteBucket deletes the entire bucket.
	DeleteBucket(ctx context.Context, l log.Logger, config Config, opts *options.TerragruntOptions) error

	// GetTFInitArgs returns the config that should be passed on to `tofu -backend-config` cmd line param
	// Allows the Backends to filter and/or modify the configuration given from the user.
	GetTFInitArgs(config Config) map[string]any
}
