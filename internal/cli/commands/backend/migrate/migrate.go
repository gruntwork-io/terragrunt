// Package migrate provides the ability to bootstrap remote state backend.
package migrate

import (
	"context"

	"github.com/gruntwork-io/terragrunt/internal/configbridge"
	"github.com/gruntwork-io/terragrunt/internal/runner"

	"github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/gruntwork-io/terragrunt/internal/remotestate/backend"
	"github.com/gruntwork-io/terragrunt/internal/util"
	"github.com/gruntwork-io/terragrunt/pkg/config"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/pkg/options"
)

func Run(ctx context.Context, l log.Logger, srcPath, dstPath string, opts *options.TerragruntOptions) error {
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

	rnr, err := runner.NewStackRunner(ctx, l, opts)
	if err != nil {
		return err
	}

	srcUnit := rnr.GetStack().FindUnitByPath(srcPath)
	if srcUnit == nil {
		return errors.Errorf("src unit not found at %s", srcPath)
	}

	dstUnit := rnr.GetStack().FindUnitByPath(dstPath)
	if dstUnit == nil {
		return errors.Errorf("dst unit not found at %s", dstPath)
	}

	srcOpts, _, err := runner.BuildUnitOpts(l, opts, srcUnit)
	if err != nil {
		return errors.Errorf("failed to build opts for src unit %s: %w", srcPath, err)
	}

	dstOpts, _, err := runner.BuildUnitOpts(l, opts, dstUnit)
	if err != nil {
		return errors.Errorf("failed to build opts for dst unit %s: %w", dstPath, err)
	}

	_, srcPctx := configbridge.NewParsingContext(ctx, l, srcOpts)

	srcRemoteState, err := config.ParseRemoteState(ctx, l, srcPctx)
	if err != nil {
		return err
	}

	if srcRemoteState == nil {
		return errors.Errorf("missing remote state configuration for source module: %s", srcPath)
	}

	_, dstPctx := configbridge.NewParsingContext(ctx, l, dstOpts)

	dstRemoteState, err := config.ParseRemoteState(ctx, l, dstPctx)
	if err != nil {
		return err
	}

	if dstRemoteState == nil {
		return errors.Errorf("missing remote state configuration for destination module: %s", dstPath)
	}

	if !opts.ForceBackendMigrate {
		enabled, err := srcRemoteState.IsVersionControlEnabled(ctx, l, configbridge.RemoteStateOptsFromOpts(srcOpts))
		if err != nil && !errors.As(err, new(backend.BucketDoesNotExistError)) {
			return err
		}

		if !enabled {
			return errors.Errorf("src bucket is not versioned, refusing to migrate backend state. If you are sure you want to migrate the backend state anyways, use the --%s flag", ForceBackendMigrateFlagName)
		}
	}

	return srcRemoteState.Migrate(ctx, l, configbridge.RemoteStateOptsFromOpts(srcOpts), configbridge.RemoteStateOptsFromOpts(dstOpts), dstRemoteState)
}
