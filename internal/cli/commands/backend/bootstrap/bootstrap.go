// Package bootstrap provides the ability to initialize remote state backend.
package bootstrap

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/gruntwork-io/terragrunt/internal/component"
	"github.com/gruntwork-io/terragrunt/internal/configbridge"
	"github.com/gruntwork-io/terragrunt/internal/discovery"
	"github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/gruntwork-io/terragrunt/internal/telemetry"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/pkg/config"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/pkg/options"
)

func Run(ctx context.Context, l log.Logger, v venv.Venv, opts *options.TerragruntOptions) error {
	if opts.RunAll {
		return runAll(ctx, l, v, opts)
	}

	return runBootstrap(ctx, l, v, opts)
}

func runBootstrap(ctx context.Context, l log.Logger, v venv.Venv, opts *options.TerragruntOptions) error {
	return telemetry.TelemeterFromContext(ctx).Collect(ctx, "backend_bootstrap", map[string]any{
		"working_dir":            opts.WorkingDir,
		"terragrunt_config_path": opts.TerragruntConfigPath,
	}, func(ctx context.Context) error {
		_, pctx := configbridge.NewParsingContext(ctx, l, opts)

		remoteState, err := config.ParseRemoteState(ctx, l, pctx)
		if err != nil || remoteState == nil {
			return err
		}

		return remoteState.Bootstrap(ctx, l, configbridge.RemoteStateOptsFromOpts(v, opts))
	})
}

func runAll(ctx context.Context, l log.Logger, v venv.Venv, opts *options.TerragruntOptions) error {
	d := discovery.NewDiscovery(opts.WorkingDir)

	components, err := d.Discover(ctx, l, v, opts)
	if err != nil {
		return err
	}

	units := components.Filter(component.UnitKind).Sort()

	return telemetry.TelemeterFromContext(ctx).Collect(ctx, "backend_bootstrap_all", map[string]any{
		"working_dir": opts.WorkingDir,
		"unit_count":  len(units),
		"fail_fast":   opts.FailFast,
	}, func(ctx context.Context) error {
		var errs []error

		for _, unit := range units {
			unitOpts := opts.Clone()
			unitOpts.WorkingDir = unit.Path()

			configFilename := config.DefaultTerragruntConfigPath
			if len(opts.TerragruntConfigPath) > 0 {
				configFilename = filepath.Base(opts.TerragruntConfigPath)
			}

			unitOpts.TerragruntConfigPath = filepath.Join(unit.Path(), configFilename)
			unitOpts.OriginalTerragruntConfigPath = unitOpts.TerragruntConfigPath

			if err := runBootstrap(ctx, l, v, unitOpts); err != nil {
				if opts.FailFast {
					return err
				}

				errs = append(
					errs,
					fmt.Errorf(
						"backend bootstrap for unit %s failed: %w",
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
	})
}
