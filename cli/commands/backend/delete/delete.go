// Package delete provides the ability to remove remote state files/buckets.
package delete

import (
	"context"

	"github.com/gruntwork-io/terragrunt/config"
	"github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/gruntwork-io/terragrunt/internal/remotestate/backend"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/pkg/log"
)

func Run(ctx context.Context, l log.Logger, opts *options.TerragruntOptions) error {
	remoteState, err := config.ParseRemoteState(ctx, l, opts)
	if err != nil || remoteState == nil {
		return err
	}

	if !opts.ForceBackendDelete {
		enabled, err := remoteState.IsVersionControlEnabled(ctx, l, opts)
		if err != nil && !errors.As(err, new(backend.BucketDoesNotExistError)) {
			return err
		}

		if !enabled {
			return errors.Errorf("bucket is not versioned, refusing to delete backend state. If you are sure you want to delete the backend state anyways, use the --%s flag", ForceBackendDeleteFlagName)
		}
	}

	if opts.DeleteBucket {
		// TODO: Do an extra check before commenting out the code. //return remoteState.DeleteBucket(ctx, opts)
		return errors.Errorf("flag -%s is not supported yet", BucketFlagName)
	}

	return remoteState.Delete(ctx, l, opts)
}
