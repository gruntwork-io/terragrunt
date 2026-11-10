// Package delete provides the ability to remove remote state files/buckets.
package delete

import (
	"context"
	"fmt"
	"path/filepath"

	"errors"

	"github.com/gruntwork-io/terragrunt/internal/cli/commands/discoverysetup"
	rewrite "github.com/gruntwork-io/terragrunt/internal/config/rewrite"
	"github.com/gruntwork-io/terragrunt/internal/configbridge"
	"github.com/gruntwork-io/terragrunt/internal/remotestate/backend"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/pkg/config"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/pkg/options"
)

// Run deletes the state that the remote_state block of the unit at
// opts.TerragruntConfigPath points to, or of every discovered unit when
// opts.RunAll is set. A unit without a remote_state block is skipped.
//
// Returns an error when the bucket has versioning disabled and
// opts.ForceBackendDelete is unset, or when opts.DeleteBucket is set.
func Run(ctx context.Context, l log.Logger, v *venv.Venv, s *rewrite.Store, opts *options.TerragruntOptions) error {
	if opts.RunAll {
		return runAll(ctx, l, v, s, opts)
	}

	return runDelete(ctx, l, v, opts)
}

func runDelete(
	ctx context.Context,
	l log.Logger,
	v *venv.Venv,
	opts *options.TerragruntOptions,
) error {
	pctx := configbridge.NewParsingContext(opts)

	remoteState, err := config.ParseRemoteState(ctx, l, v, pctx)
	if err != nil || remoteState == nil {
		return err
	}

	if !opts.ForceBackendDelete {
		enabled, err := remoteState.IsVersionControlEnabled(
			ctx,
			l,
			v,
			configbridge.RemoteStateOptsFromOpts(v.Env, opts),
		)
		if err != nil && !errors.As(err, new(backend.BucketDoesNotExistError)) {
			return err
		}

		if !enabled {
			return fmt.Errorf(
				"bucket is not versioned, refusing to delete backend state."+
					" If you are sure you want to delete the backend state anyways, use the --%s flag",
				ForceBackendDeleteFlagName)
		}
	}

	if opts.DeleteBucket {
		// TODO: Do an extra check before commenting out the code. //return remoteState.DeleteBucket(ctx, opts)
		return fmt.Errorf("flag -%s is not supported yet", BucketFlagName)
	}

	return remoteState.Delete(ctx, l, v, configbridge.RemoteStateOptsFromOpts(v.Env, opts))
}

func runAll(
	ctx context.Context,
	l log.Logger,
	v *venv.Venv,
	s *rewrite.Store,
	opts *options.TerragruntOptions,
) error {
	units, cleanupWorktrees, err := discoverysetup.Units(ctx, l, v, s, opts)

	defer cleanupWorktrees(ctx)

	if err != nil {
		return err
	}

	var errs []error

	for _, unit := range units {
		unitOpts := opts.Clone()
		unitOpts.WorkingDir = unit.Path()

		configFilename := config.DefaultTerragruntConfigPath
		if len(opts.TerragruntConfigPath) > 0 {
			configFilename = filepath.Base(opts.TerragruntConfigPath)
		}

		unitOpts.TerragruntConfigPath = filepath.Join(unit.Path(), configFilename)

		// Parsing can write obtained credentials into the env, so each
		// unit gets its own clone to keep them from leaking to siblings.
		unitV := v.WithEnvCloned()
		if err := runDelete(ctx, l, unitV, unitOpts); err != nil {
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
