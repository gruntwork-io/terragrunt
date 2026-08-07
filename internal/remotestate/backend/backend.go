// Package backend represents a backend for interacting with remote state.
package backend

import (
	"context"

	"github.com/gruntwork-io/terragrunt/internal/experiment"
	"github.com/gruntwork-io/terragrunt/internal/iam"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/pkg/log"
)

// Options bundles the configuration the Backend interface needs at each call
// site.
type Options struct {
	Experiments                  experiment.Experiments
	IAMRoleOptions               iam.RoleOptions
	NonInteractive               bool
	FailIfBucketCreationRequired bool
}

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
	IsVersionControlEnabled(
		ctx context.Context,
		l log.Logger,
		v *venv.Venv,
		config Config,
		opts *Options,
	) (bool, error)

	// NeedsBootstrap returns true if remote state needs to be bootstrapped.
	NeedsBootstrap(
		ctx context.Context,
		l log.Logger,
		v *venv.Venv,
		config Config,
		opts *Options,
	) (bool, error)

	// Bootstrap bootstraps the remote state.
	Bootstrap(ctx context.Context, l log.Logger, v *venv.Venv, config Config, opts *Options) error

	// Migrate determines where the remote state resources exist for source backend config and migrate them to dest backend config.
	//
	// srcV and dstV are the source and destination environments: the same
	// variable (AWS_PROFILE, ARM_ENVIRONMENT, ...) can hold a different value on
	// each side, so destination SETTINGS must be resolved from dstV, never from
	// srcV. Backends currently use dstV to validate that the destination is
	// reachable by the migration they implement (for example azurerm refuses a
	// destination in another cloud). Performing destination WRITES under
	// destination credentials is not implemented yet: the s3, gcs, and azurerm
	// backends still write through the source client, so a migration whose
	// destination needs different credentials must be rejected rather than
	// attempted.
	Migrate(
		ctx context.Context,
		l log.Logger,
		srcV, dstV *venv.Venv,
		srcConfig, dstConfig Config,
		opts *Options,
	) error

	// Delete deletes the remote state.
	Delete(ctx context.Context, l log.Logger, v *venv.Venv, config Config, opts *Options) error

	// DeleteBucket deletes the entire bucket.
	DeleteBucket(
		ctx context.Context,
		l log.Logger,
		v *venv.Venv,
		config Config,
		opts *Options,
	) error

	// GetTFInitArgs returns the config that should be passed on to `tofu -backend-config` cmd line param
	// Allows the Backends to filter and/or modify the configuration given from the user.
	GetTFInitArgs(config Config) map[string]any
}
