// Package common provides common code that are used by many commands.
package common

import (
	"context"

	"github.com/gruntwork-io/terragrunt/config"
	"github.com/gruntwork-io/terragrunt/internal/experiment"
	"github.com/gruntwork-io/terragrunt/internal/remotestate"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/tf"
)

func GetRemoteState(ctx context.Context, opts *options.TerragruntOptions) (*remotestate.RemoteState, error) {
	cfg, err := config.ReadTerragruntConfig(ctx, opts, config.DefaultParserOptions(opts))
	if err != nil {
		return nil, err
	}

	if cfg.RemoteState == nil {
		return nil, nil
	}

	sourceURL, err := config.GetTerraformSourceURL(opts, cfg)
	if err != nil {
		return nil, err
	}

	if sourceURL != "" {
		walkWithSymlinks := opts.Experiments.Evaluate(experiment.Symlinks)

		tfSource, err := tf.NewSource(sourceURL, opts.DownloadDir, opts.WorkingDir, opts.Logger, walkWithSymlinks)
		if err != nil {
			return nil, err
		}

		opts = opts.Clone()
		opts.WorkingDir = tfSource.WorkingDir
	}

	return cfg.RemoteState, nil
}
