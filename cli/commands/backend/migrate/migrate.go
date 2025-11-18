// Package migrate provides the ability to bootstrap remote state backend.
package migrate

import (
	"context"

	"github.com/gruntwork-io/terragrunt/internal/runner"

	"github.com/gruntwork-io/terragrunt/config"
	"github.com/gruntwork-io/terragrunt/internal/component"
	"github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/gruntwork-io/terragrunt/internal/remotestate/backend"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/util"
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

	stack, err := runner.FindStackInSubfolders(ctx, l, opts)
	if err != nil {
		return err
	}

	srcModule := stack.GetStack().FindUnitByPath(srcPath)
	if srcModule == nil {
		return errors.Errorf("src unit not found at %s", srcPath)
	}

	dstModuleComp := stack.GetStack().FindUnitByPath(dstPath)
	if dstModuleComp == nil {
		return errors.Errorf("dst unit not found at %s", dstPath)
	}

	// Use type assertion to ensure components are Units
	srcUnit, ok := srcModule.(*component.Unit)
	if !ok {
		return errors.Errorf("src module is not a unit")
	}

	dstUnit, ok := dstModuleComp.(*component.Unit)
	if !ok {
		return errors.Errorf("dst module is not a unit")
	}

	//nolint:staticcheck // TerragruntOptions() is deprecated but required for config.ParseRemoteState and remotestate.Migrate which haven't been migrated to RunnerOptions yet
	srcOpts := srcUnit.TerragruntOptions()
	//nolint:staticcheck // TerragruntOptions() is deprecated but required for config.ParseRemoteState and remotestate.Migrate which haven't been migrated to RunnerOptions yet
	dstOpts := dstUnit.TerragruntOptions()

	srcRemoteState, err := config.ParseRemoteState(ctx, l, srcOpts)
	if err != nil || srcRemoteState == nil {
		return err
	}

	dstRemoteState, err := config.ParseRemoteState(ctx, l, dstOpts)
	if err != nil || dstRemoteState == nil {
		return err
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
