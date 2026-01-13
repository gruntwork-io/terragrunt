// Package delete provides the ability to remove remote state files/buckets.
package delete

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/gruntwork-io/terragrunt/config"
	"github.com/gruntwork-io/terragrunt/internal/component"
	"github.com/gruntwork-io/terragrunt/internal/discovery"
	"github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/gruntwork-io/terragrunt/internal/remotestate/backend"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/pkg/log"
)

func Run(ctx context.Context, l log.Logger, opts *options.TerragruntOptions) error {
	if opts.RunAll {
		return runAll(ctx, l, opts)
	}

	return runDelete(ctx, l, opts)
}

func runDelete(ctx context.Context, l log.Logger, opts *options.TerragruntOptions) error {
	remoteState, err := config.ParseRemoteState(ctx, l, opts)
	if err != nil || remoteState == nil {
		return err
	}

	if !opts.ForceBackendDelete {
		enabled, err := remoteState.IsVersionControlEnabled(ctx, l, opts)
		if err != nil && !errors.As(err, new(backend.BucketDoesNotExistError)) {
			return err
		}

		if !enabled {
			return errors.Errorf("bucket is not versioned, refusing to delete backend state. If you are sure you want to delete the backend state anyways, use the --%s flag", ForceBackendDeleteFlagName)
		}
	}

	if opts.DeleteBucket {
		// TODO: Do an extra check before commenting out the code. //return remoteState.DeleteBucket(ctx, opts)
		return errors.Errorf("flag -%s is not supported yet", BucketFlagName)
	}

	return remoteState.Delete(ctx, l, opts)
}

func runAll(ctx context.Context, l log.Logger, opts *options.TerragruntOptions) error {
	d := discovery.NewDiscovery(opts.WorkingDir)

	components, err := d.Discover(ctx, l, opts)
	if err != nil {
		return err
	}

	units := components.Filter(component.UnitKind).Sort()

	var errs []error

	for _, unit := range units {
		unitOpts := opts.Clone()
		unitOpts.WorkingDir = unit.Path()

		configFilename := config.DefaultTerragruntConfigPath
		if len(opts.TerragruntConfigPath) > 0 {
			configFilename = filepath.Base(opts.TerragruntConfigPath)
		}

		unitOpts.TerragruntConfigPath = filepath.Join(unit.Path(), configFilename)

		if err := runDelete(ctx, l, unitOpts); err != nil {
			if opts.FailFast {
				return err
			}

			errs = append(
				errs,
				fmt.Errorf(
					"backend delete for unit %s failed: %w",
					unit.Path(),
					err,
				),
			)
		}
	}

	if len(errs) > 0 {
		return errors.Join(errs...)
	}

	return nil
}
