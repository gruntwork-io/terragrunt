// Package migrate provides the ability to bootstrap remote state backend.
package migrate

import (
	"context"

	"github.com/gruntwork-io/terragrunt/config"
	"github.com/gruntwork-io/terragrunt/configstack"
	"github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/gruntwork-io/terragrunt/internal/remotestate/backend"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/util"
)

func Run(ctx context.Context, srcPath, dstPath string, opts *options.TerragruntOptions) error {
	var err error

	srcPath, err = util.CanonicalPath(srcPath, opts.WorkingDir)
	if err != nil {
		return err
	}

	opts.Logger.Debugf("Source unit path %s", srcPath)

	dstPath, err = util.CanonicalPath(dstPath, opts.WorkingDir)
	if err != nil {
		return err
	}

	opts.Logger.Debugf("Destination unit path %s", dstPath)

	stack, err := configstack.FindStackInSubfolders(ctx, opts)
	if err != nil {
		return err
	}

	srcModule := stack.Modules.FindByPath(srcPath)
	if srcModule == nil {
		return errors.Errorf("src unit not found at %s", srcPath)
	}

	dstModule := stack.Modules.FindByPath(dstPath)
	if dstModule == nil {
		return errors.Errorf("dst unit not found at %s", dstPath)
	}

	srcRemoteState, err := config.ParseRemoteState(ctx, srcModule.TerragruntOptions)
	if err != nil || srcRemoteState == nil {
		return err
	}

	dstRemoteState, err := config.ParseRemoteState(ctx, dstModule.TerragruntOptions)
	if err != nil || dstRemoteState == nil {
		return err
	}

	if !opts.ForceBackendMigrate {
		enabled, err := srcRemoteState.IsVersionControlEnabled(ctx, srcModule.TerragruntOptions)
		if err != nil && !errors.As(err, new(backend.BucketDoesNotExistError)) {
			return err
		}

		if !enabled {
			return errors.Errorf("src bucket is not versioned, refusing to migrate backend state. If you are sure you want to migrate the backend state anyways, use the --%s flag", ForceBackendMigrateFlagName)
		}
	}

	return srcRemoteState.Migrate(ctx, srcModule.TerragruntOptions, dstModule.TerragruntOptions, dstRemoteState)
}
