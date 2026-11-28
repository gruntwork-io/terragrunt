// Package bootstrap provides the ability to initialize remote state backend.
package bootstrap

import (
	"context"
	"fmt"

	"errors"

	"github.com/gruntwork-io/terragrunt/internal/cli/commands/discoverysetup"
	"github.com/gruntwork-io/terragrunt/internal/configbridge"
	"github.com/gruntwork-io/terragrunt/internal/telemetry"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/pkg/config"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/pkg/options"
)

// Run creates the backend resources declared in the remote_state block of the
// unit at opts.TerragruntConfigPath, or of every discovered unit when
// opts.RunAll is set. A unit without a remote_state block is skipped.
func Run(ctx context.Context, l log.Logger, v *venv.Venv, s *config.Store, opts *options.TerragruntOptions) error {
	if opts.RunAll {
		return runAll(ctx, l, v, s, opts)
	}

	return runBootstrap(ctx, l, v, s, opts)
}

func runBootstrap(
	ctx context.Context,
	l log.Logger,
	v *venv.Venv,
	s *config.Store,
	opts *options.TerragruntOptions,
) error {
	return telemetry.TelemeterFromContext(ctx).Collect(ctx, l, "backend_bootstrap", map[string]any{
		"working_dir":            opts.WorkingDir,
		"terragrunt_config_path": opts.TerragruntConfigPath,
	}, func(ctx context.Context, l log.Logger) error {
		pctx := configbridge.NewParsingContext(opts)

		remoteState, err := config.ParseRemoteState(ctx, l, v, s, pctx)
		if err != nil || remoteState == nil {
			return err
		}

		return remoteState.Bootstrap(ctx, l, v, configbridge.RemoteStateOptsFromOpts(v.Exec, v.Env, opts))
	})
}

func runAll(
	ctx context.Context,
	l log.Logger,
	v *venv.Venv,
	s *config.Store,
	opts *options.TerragruntOptions,
) error {
	units, cleanupWorktrees, err := discoverysetup.Units(ctx, l, v, s, opts)

	defer cleanupWorktrees(ctx)

	if err != nil {
		return err
	}

	return telemetry.TelemeterFromContext(ctx).
		Collect(ctx, l, "backend_bootstrap_all", map[string]any{
			"working_dir": opts.WorkingDir,
			"unit_count":  len(units),
			"fail_fast":   opts.FailFast,
		}, func(ctx context.Context, l log.Logger) error {
			var errs []error

			for _, unit := range units {
				unitOpts := opts.Clone()
				discoverysetup.ScopeToUnit(unitOpts, unit)

				// Parsing can write obtained credentials into the env, so each
				// unit gets its own clone to keep them from leaking to siblings.
				unitV := v.WithEnvCloned()
				if err := runBootstrap(ctx, l, unitV, s, unitOpts); err != nil {
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
