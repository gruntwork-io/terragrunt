// Package bootstrap provides the ability to initialize remote state backend.
package bootstrap

import (
	"context"
	"path/filepath"

	"github.com/gruntwork-io/terragrunt/config"
	"github.com/gruntwork-io/terragrunt/internal/component"
	"github.com/gruntwork-io/terragrunt/internal/discovery"
	"github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/pkg/log"
)

func Run(ctx context.Context, l log.Logger, opts *options.TerragruntOptions) error {
	if opts.RunAll {
		return runAll(ctx, l, opts)
	}

	return runBootstrap(ctx, l, opts)
}

func runBootstrap(ctx context.Context, l log.Logger, opts *options.TerragruntOptions) error {
	remoteState, err := config.ParseRemoteState(ctx, l, opts)
	if err != nil || remoteState == nil {
		return err
	}

	return remoteState.Bootstrap(ctx, l, opts)
}

func runAll(ctx context.Context, l log.Logger, opts *options.TerragruntOptions) error {
	d := discovery.NewDiscovery(opts.WorkingDir)

	components, err := d.Discover(ctx, l, opts)
	if err != nil {
		return err
	}

	// Filter to only units (not stacks)
	units := components.Filter(component.UnitKind)

	var errs []error

	for _, unit := range units {
		unitOpts := opts.Clone()
		unitOpts.WorkingDir = unit.Path()

		configFilename := config.DefaultTerragruntConfigPath
		if len(opts.TerragruntConfigPath) > 0 {
			configFilename = filepath.Base(opts.TerragruntConfigPath)
		}

		unitOpts.TerragruntConfigPath = filepath.Join(unit.Path(), configFilename)

		if err := runBootstrap(ctx, l, unitOpts); err != nil {
			errs = append(errs, err)
		}
	}

	if len(errs) > 0 {
		return errors.Join(errs...)
	}

	return nil
}
