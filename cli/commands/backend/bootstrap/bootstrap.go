// Package bootstrap provides the ability to initialize remote state backend.
package bootstrap

import (
	"context"

	"github.com/gruntwork-io/terragrunt/config"
	"github.com/gruntwork-io/terragrunt/options"
)

func Run(ctx context.Context, opts *options.TerragruntOptions) error {
	remoteState, err := config.ParseRemoteState(ctx, opts)
	if err != nil || remoteState == nil {
		return err
	}

	return remoteState.Bootstrap(ctx, opts)
}
