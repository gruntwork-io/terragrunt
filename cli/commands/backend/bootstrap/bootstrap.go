// Package bootstrap provides the ability to initialize remote state backend.
package bootstrap

import (
	"context"

	"github.com/gruntwork-io/terragrunt/config"
	"github.com/gruntwork-io/terragrunt/internal/experiment"
	"github.com/gruntwork-io/terragrunt/internal/remotestate"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/tf"
)

func Run(ctx context.Context, opts *options.TerragruntOptions) error {
	cfg, err := config.ReadTerragruntConfig(ctx, opts, config.DefaultParserOptions(opts))
	if err != nil {
		return err
	}

	if cfg.RemoteState == nil {
		return nil
	}

	sourceURL, err := config.GetTerraformSourceURL(opts, cfg)
	if err != nil {
		return err
	}

	if sourceURL != "" {
		walkWithSymlinks := opts.Experiments.Evaluate(experiment.Symlinks)

		tfSource, err := tf.NewSource(sourceURL, opts.DownloadDir, opts.WorkingDir, opts.Logger, walkWithSymlinks)
		if err != nil {
			return err
		}

		opts = opts.Clone()
		opts.WorkingDir = tfSource.WorkingDir
	}

	return initRemoteState(ctx, opts, cfg.RemoteState)
}

func initRemoteState(ctx context.Context, opts *options.TerragruntOptions, remoteState *remotestate.RemoteState) error {
	remoteStateNeedsInit, err := remoteState.NeedsInit(ctx, opts)
	if err != nil || !remoteStateNeedsInit {
		return err
	}

	if err := remoteState.Init(ctx, opts); err != nil {
		return err
	}

	return nil
}
