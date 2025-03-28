// Package delete provides the ability to remove remote state files/buckets.
package delete

import (
	"context"

	"github.com/gruntwork-io/terragrunt/config"
	"github.com/gruntwork-io/terragrunt/internal/errors"
)

func Run(ctx context.Context, cmdOpts *Options) error {
	opts := cmdOpts.TerragruntOptions

	remoteState, err := config.ParseRemoteState(ctx, opts)
	if err != nil {
		return err
	}

	if remoteState == nil {
		opts.Logger.Debug("Did not find remote `remote_state` block in the config")

		return nil
	}

	if cmdOpts.DeleteBucket {
		// TODO: Do an extra check before commenting out the code. //return remoteState.DeleteBucket(ctx, opts)
		return errors.Errorf("flag -%s is not supported yet", BucketFlagName)
	}

	return remoteState.Delete(ctx, opts)
}
