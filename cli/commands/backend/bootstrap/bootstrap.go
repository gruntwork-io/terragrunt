// Package bootstrap provides the ability to initialize remote state backend.
package bootstrap

import (
	"context"

	"github.com/gruntwork-io/terragrunt/config"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/pkg/log"
)

func Run(ctx context.Context, l log.Logger, opts *options.TerragruntOptions) error {
	remoteState, err := config.ParseRemoteState(ctx, l, opts)
	if err != nil || remoteState == nil {
		return err
	}

	return remoteState.Bootstrap(ctx, l, opts)
}
