// Package delete provides the ability to remove remote state files/buckets.
package delete

import (
	"context"

	"github.com/gruntwork-io/terragrunt/cli/commands/common"
)

func Run(ctx context.Context, cmdOpts *Options) error {
	opts := cmdOpts.TerragruntOptions

	remoteState, err := common.GetRemoteState(ctx, opts)
	if err != nil {
		return err
	}

	if cmdOpts.DeleteBucket {
		return remoteState.DeleteBucket(ctx, opts)
	}

	return remoteState.Delete(ctx, opts)
}
