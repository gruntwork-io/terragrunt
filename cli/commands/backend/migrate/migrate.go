// Package migrate provides the ability to bootstrap remote state backend.
package migrate

import (
	"context"

	"github.com/gruntwork-io/terragrunt/config"
	"github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/gruntwork-io/terragrunt/internal/remotestate"
	"github.com/gruntwork-io/terragrunt/internal/remotestate/backend"
	"github.com/gruntwork-io/terragrunt/options"
)

func Run(ctx context.Context, srcPath, dstPath string, opts *options.TerragruntOptions) error {
	remoteState, err := config.ParseRemoteState(ctx, opts)
	if err != nil {
		return err
	}

	if remoteState == nil {
		opts.Logger.Debug("Did not find remote `remote_state` block in the config")

		return nil
	}

	if !opts.ForceBackendMigrate {
		if err := checkIfVersionControlEnabled(ctx, remoteState, opts); err != nil {
			return err
		}
	}

	return remoteState.Migrate(ctx, srcPath, dstPath, opts)
}

func checkIfVersionControlEnabled(ctx context.Context, remoteState *remotestate.RemoteState, opts *options.TerragruntOptions) error {
	enabled, err := remoteState.IsVersionControlEnabled(ctx, opts)
	if err != nil && !errors.As(err, new(backend.BucketDoesNotExistError)) {
		return err
	}

	if !enabled {
		return errors.Errorf("bucket is not versioned, refusing to migrate backend state. If you are sure you want to migrate the backend state anyways, use the --%s flag", ForceBackendMigrateFlagName)
	}

	return nil
}
