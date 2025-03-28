package run

import (
	"context"

	"github.com/gruntwork-io/terragrunt/config"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/tf"
	"github.com/gruntwork-io/terragrunt/util"
)

func runVersionCommand(ctx context.Context, opts *options.TerragruntOptions) error {
	if !util.FileExists(opts.TerragruntConfigPath) {
		opts.Logger.Debugf("Did not find the config file %s", opts.TerragruntConfigPath)

		return nil
	}

	configCtx := config.NewParsingContext(ctx, opts).WithDecodeList(
		config.TerragruntVersionConstraints, config.FeatureFlagsBlock)

	cfg, err := config.PartialParseConfigFile( //nolint: contextcheck
		configCtx,
		opts.TerragruntConfigPath,
		nil,
	)
	if err != nil {
		return err
	}

	opts.TerraformPath = cfg.TerraformBinary

	return tf.RunCommand(ctx, opts, tf.CommandNameVersion)
}
