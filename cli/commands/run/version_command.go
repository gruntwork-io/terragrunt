package run

import (
	"context"

	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/tf"
	"github.com/gruntwork-io/terragrunt/util"
)

func runVersionCommand(ctx context.Context, l log.Logger, opts *options.TerragruntOptions) error {
	if !opts.TFPathExplicitlySet {
		if tfPath, err := getTfPathFromConfig(ctx, l, opts); err != nil {
			return err
		} else if tfPath != "" {
			opts.TFPath = tfPath
		}
	}

	return tf.RunCommand(ctx, l, opts, opts.TerraformCliArgs...)
}

func getTfPathFromConfig(ctx context.Context, l log.Logger, opts *options.TerragruntOptions) (string, error) {
	if !util.FileExists(opts.TerragruntConfigPath) {
		l.Debugf("Did not find the config file %s", opts.TerragruntConfigPath)

		return "", nil
	}

	cfg, err := getTerragruntConfig(ctx, l, opts)
	if err != nil {
		return "", err
	}

	return cfg.TerraformBinary, nil
}
