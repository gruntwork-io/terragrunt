// Package migrate provides the ability to bootstrap remote state backend.
package migrate

import (
	"context"

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

	stackRunner, unitOpts, _, err := runner.FindStackInSubfolders(ctx, l, opts)
	if err != nil {
		return err
	}

	srcModule := stackRunner.GetStack().FindUnitByPath(srcPath)
	if srcModule == nil {
		return errors.Errorf("src unit not found at %s", srcPath)
	}

	dstModule := stackRunner.GetStack().FindUnitByPath(dstPath)
	if dstModule == nil {
		return errors.Errorf("dst unit not found at %s", dstPath)
	}

	srcOpts := unitOpts[srcPath]
	if srcOpts == nil {
		return errors.Errorf("src unit has no execution context at %s", srcPath)
	}

	dstOpts := unitOpts[dstPath]
	if dstOpts == nil {
		return errors.Errorf("dst unit has no execution context at %s", dstPath)
	}

	srcRemoteState, err := config.ParseRemoteState(ctx, l, srcOpts)
	if err != nil {
		return err
	}

	if srcRemoteState == nil {
		return errors.Errorf("missing remote state configuration for source module: %s", srcPath)
	}

	dstRemoteState, err := config.ParseRemoteState(ctx, l, dstOpts)
	if err != nil {
		return err
	}

	if dstRemoteState == nil {
		return errors.Errorf("missing remote state configuration for destination module: %s", dstPath)
	}

	if !opts.ForceBackendMigrate {
		enabled, err := srcRemoteState.IsVersionControlEnabled(ctx, l, srcOpts)
		if err != nil && !errors.As(err, new(backend.BucketDoesNotExistError)) {
			return err
		}

		if !enabled {
			return errors.Errorf("src bucket is not versioned, refusing to migrate backend state. If you are sure you want to migrate the backend state anyways, use the --%s flag", ForceBackendMigrateFlagName)
		}
	}

	return srcRemoteState.Migrate(ctx, l, srcOpts, dstOpts, dstRemoteState)
}
