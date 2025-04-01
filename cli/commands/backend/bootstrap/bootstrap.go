// Package bootstrap provides the ability to initialize remote state backend.
package bootstrap

import (
	"context"

	"github.com/gruntwork-io/terragrunt/config"
	"github.com/gruntwork-io/terragrunt/options"
)

func Run(ctx context.Context, opts *options.TerragruntOptions) error {
	remoteState, err := config.ParseRemoteState(ctx, opts)
	if err != nil {
		return err
	}

	if remoteState == nil {
		opts.Logger.Debug("Did not find remote `remote_state` block in the config")

		return nil
	}

	return remoteState.Bootstrap(ctx, opts)
}
