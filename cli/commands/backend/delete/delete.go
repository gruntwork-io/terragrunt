// Package delete provides the ability to remove remote state files/buckets.
package delete

import (
	"context"

	"github.com/gruntwork-io/terragrunt/config"
	"github.com/gruntwork-io/terragrunt/internal/experiment"
	"github.com/gruntwork-io/terragrunt/tf"
)

func Run(ctx context.Context, cmdOpts *Options) error {
	opts := cmdOpts.TerragruntOptions

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

	if cmdOpts.DeleteBucket {
		return cfg.RemoteState.DeleteBucket(ctx, opts)
	}

	return nil
}
