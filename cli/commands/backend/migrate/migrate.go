// Package migrate provides the ability to bootstrap remote state backend.
package migrate

import (
	"context"
	"fmt"

	"github.com/gruntwork-io/terragrunt/config"
	"github.com/gruntwork-io/terragrunt/configstack"
	"github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/gruntwork-io/terragrunt/internal/remotestate"
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

	stack, err := configstack.FindStackInSubfolders(ctx, l, opts)
	if err != nil {
		return err
	}

	srcModule := stack.FindModuleByPath(srcPath)
	if srcModule == nil {
		return errors.Errorf("src unit not found at %s", srcPath)
	}

	dstModule := stack.FindModuleByPath(dstPath)
	if dstModule == nil {
		return errors.Errorf("dst unit not found at %s", dstPath)
	}

	// Ensure experiment flags are propagated to module options
	srcModule.TerragruntOptions.Experiments = opts.Experiments
	dstModule.TerragruntOptions.Experiments = opts.Experiments

	l.Debugf("Migration: Source experiments: %v", srcModule.TerragruntOptions.Experiments)
	l.Debugf("Migration: Destination experiments: %v", dstModule.TerragruntOptions.Experiments)

	// Re-register backends with updated experiment flags
	remotestate.RegisterBackends(srcModule.TerragruntOptions)

	srcRemoteState, err := config.ParseRemoteState(ctx, l, srcModule.TerragruntOptions)
	if err != nil || srcRemoteState == nil {
		return err
	}

	dstRemoteState, err := config.ParseRemoteState(ctx, l, dstModule.TerragruntOptions)
	if err != nil || dstRemoteState == nil {
		return err
	}

	// Update backends to use newly registered backends (e.g., Azure backend after experiment flag is set)
	srcRemoteState.UpdateBackend()
	dstRemoteState.UpdateBackend()

	l.Debugf("Migration: Source backend: %s (type: %T)", srcRemoteState.BackendName, srcRemoteState)
	l.Debugf("Migration: Destination backend: %s (type: %T)", dstRemoteState.BackendName, dstRemoteState)

	if !opts.ForceBackendMigrate {
		enabled, err := srcRemoteState.IsVersionControlEnabled(ctx, l, srcModule.TerragruntOptions)
		if err != nil && !errors.As(err, new(backend.BucketDoesNotExistError)) {
			return err
		}

		if !enabled {
			return errors.Errorf("src bucket is not versioned, refusing to migrate backend state. If you are sure you want to migrate the backend state anyways, use the --%s flag", ForceBackendMigrateFlagName)
		}
	}

	err = srcRemoteState.Migrate(ctx, l, srcModule.TerragruntOptions, dstModule.TerragruntOptions, dstRemoteState)
	if err != nil {
		return err
	}

	l.Infof("Successfully migrated remote state from %s to %s", srcPath, dstPath)
	fmt.Printf("Backend migration completed successfully from %s to %s\n", srcPath, dstPath)

	return nil
}
