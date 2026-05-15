// Package migrate provides the ability to bootstrap remote state backend.
package migrate

import (
	"context"
	"fmt"

	"github.com/gruntwork-io/terragrunt/internal/configbridge"
	"github.com/gruntwork-io/terragrunt/internal/runner"
	"github.com/gruntwork-io/terragrunt/internal/venv"

	"errors"

	"github.com/gruntwork-io/terragrunt/internal/remotestate/backend"
	"github.com/gruntwork-io/terragrunt/internal/util"
	"github.com/gruntwork-io/terragrunt/pkg/config"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/pkg/options"
)

// Run migrates Terraform/OpenTofu state from srcPath to dstPath. v is the
// virtualized environment used to build the stack runner; the source and
// destination each parse and run under their own Env clone of it so a
// migration between two accounts of the same cloud can carry distinct
// credentials on each side.
func Run(
	ctx context.Context,
	l log.Logger,
	v *venv.Venv,
	srcPath, dstPath string,
	opts *options.TerragruntOptions,
) error {
	var err error

	srcPath, err = util.CanonicalPath(srcPath, opts.WorkingDir)
	if err != nil {
		return err
	}

	l.Debugf("Source unit path %s", srcPath)

	dstPath, err = util.CanonicalPath(dstPath, opts.WorkingDir)
	if err != nil {
		return err
	}

	l.Debugf("Destination unit path %s", dstPath)

	rnr, err := runner.NewStackRunner(ctx, l, v, opts)
	if err != nil {
		return err
	}

	srcUnit := rnr.GetStack().FindUnitByPath(srcPath)
	if srcUnit == nil {
		return fmt.Errorf("src unit not found at %s", srcPath)
	}

	dstUnit := rnr.GetStack().FindUnitByPath(dstPath)
	if dstUnit == nil {
		return fmt.Errorf("dst unit not found at %s", dstPath)
	}

	srcOpts, _, err := runner.BuildUnitOpts(l, opts, srcUnit)
	if err != nil {
		return fmt.Errorf("failed to build opts for src unit %s: %w", srcPath, err)
	}

	dstOpts, _, err := runner.BuildUnitOpts(l, opts, dstUnit)
	if err != nil {
		return fmt.Errorf("failed to build opts for dst unit %s: %w", dstPath, err)
	}

	// Source and destination each get an independent Env clone so per-unit
	// contributions during parsing (auth-provider-cmd credentials, TF_VAR_*)
	// stay on their own side instead of clobbering one another, and so the
	// pull and push below run under the correct environment.
	srcV := v.WithEnvCloned()
	dstV := v.WithEnvCloned()

	_, srcPctx := configbridge.NewParsingContext(ctx, l, srcOpts)
	srcPctx = srcPctx.WithVenv(&srcV)

	srcRemoteState, err := config.ParseRemoteState(ctx, l, srcPctx)
	if err != nil {
		return err
	}

	if srcRemoteState == nil {
		return fmt.Errorf("missing remote state configuration for source module: %s", srcPath)
	}

	// ParseRemoteState updates pctx.WorkingDir to point to the .terragrunt-cache
	// directory (where backend.tf and .terraform/ live) when a terraform source is
	// configured. Propagate that back so pullState runs in the correct directory.
	srcOpts.WorkingDir = srcPctx.WorkingDir

	_, dstPctx := configbridge.NewParsingContext(ctx, l, dstOpts)
	dstPctx = dstPctx.WithVenv(&dstV)

	dstRemoteState, err := config.ParseRemoteState(ctx, l, dstPctx)
	if err != nil {
		return err
	}

	if dstRemoteState == nil {
		return fmt.Errorf("missing remote state configuration for destination module: %s", dstPath)
	}

	// Same for the destination: pushState needs the cache directory.
	dstOpts.WorkingDir = dstPctx.WorkingDir

	if !opts.ForceBackendMigrate {
		enabled, err := srcRemoteState.IsVersionControlEnabled(
			ctx,
			l,
			srcV,
			configbridge.RemoteStateOptsFromOpts(srcOpts),
		)
		if err != nil && !errors.As(err, new(backend.BucketDoesNotExistError)) {
			return err
		}

		if !enabled {
			return fmt.Errorf(
				"src bucket is not versioned, refusing to migrate backend state."+
					" If you are sure you want to migrate the backend state anyways, use the --%s flag",
				ForceBackendMigrateFlagName)
		}
	}

	return srcRemoteState.Migrate(
		ctx, l,
		srcV, dstV,
		configbridge.RemoteStateOptsFromOpts(srcOpts),
		configbridge.RemoteStateOptsFromOpts(dstOpts),
		dstRemoteState,
	)
}
