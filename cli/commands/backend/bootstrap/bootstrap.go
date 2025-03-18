// Package bootstrap provides the ability to initialize remote state backend.
package bootstrap

import (
	"context"

	"github.com/gruntwork-io/terragrunt/cli/commands/common"
	"github.com/gruntwork-io/terragrunt/options"
)

func Run(ctx context.Context, opts *options.TerragruntOptions) error {
	remoteState, err := common.GetRemoteState(ctx, opts)
	if err != nil {
		return err
	}

	return remoteState.Init(ctx, opts)
}
