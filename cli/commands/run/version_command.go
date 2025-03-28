package run

import (
	"context"

	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/tf"
	"github.com/gruntwork-io/terragrunt/util"
)

func runVersionCommand(ctx context.Context, opts *options.TerragruntOptions) error {
	if tfPath, err := getTfPathFromConfig(ctx, opts); err != nil {
		return err
	} else if tfPath != "" {
		opts.TerraformPath = tfPath
	}

	return tf.RunCommand(ctx, opts, tf.CommandNameVersion)
}

func getTfPathFromConfig(ctx context.Context, opts *options.TerragruntOptions) (string, error) {
	if !util.FileExists(opts.TerragruntConfigPath) {
		opts.Logger.Debugf("Did not find the config file %s", opts.TerragruntConfigPath)

		return "", nil
	}

	cfg, err := getTerragruntConfig(ctx, opts)
	if err != nil {
		return "", err
	}

	return cfg.TerraformBinary, nil
}
