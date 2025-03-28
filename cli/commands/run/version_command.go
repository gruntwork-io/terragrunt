package run

import (
	"context"

	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/tf"
	"github.com/gruntwork-io/terragrunt/util"
)

func runVersionCommand(ctx context.Context, opts *options.TerragruntOptions) error {
	if util.FileExists(opts.TerragruntConfigPath) {
		cfg, err := getTerragruntConfig(ctx, opts)
		if err != nil {
			return err
		}

		if cfg.TerraformBinary != "" {
			opts.TerraformPath = cfg.TerraformBinary
		}
	} else {
		opts.Logger.Debugf("Did not find the config file %s", opts.TerragruntConfigPath)
	}

	return tf.RunCommand(ctx, opts, tf.CommandNameVersion)
}
