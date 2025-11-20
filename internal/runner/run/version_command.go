package run

import (
	"context"

	runnertypes "github.com/gruntwork-io/terragrunt/internal/runner/types"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/tf"
	"github.com/gruntwork-io/terragrunt/util"
)

func runVersionCommand(ctx context.Context, l log.Logger, runnerOpts *runnertypes.RunnerOptions) error {
	// Convert to TFOptions for tf package call
	tfOpts := toTFOptions(runnerOpts)

	// Check for terraform_binary override in config
	if !tfOpts.TFPathExplicitlySet {
		// getTfPathFromConfig still needs minimal TerragruntOptions for config parsing
		opts := &options.TerragruntOptions{
			TerragruntConfigPath: tfOpts.TerragruntConfigPath,
			WorkingDir:           tfOpts.WorkingDir,
		}

		if tfPath, err := getTfPathFromConfig(ctx, l, opts); err != nil {
			return err
		} else if tfPath != "" {
			tfOpts.TFPath = tfPath
		}
	}

	// Use RunCommandWithOptions which accepts TFOptions
	return tf.RunCommandWithOptions(ctx, l, tfOpts, tfOpts.TerraformCliArgs...)
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
