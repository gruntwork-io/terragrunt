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
	// For tf.RunCommand we still need TerragruntOptions, create minimal opts
	// TODO: Eventually tf.RunCommand should accept RunnerOptions
	opts := &options.TerragruntOptions{
		TFPath:                  runnerOpts.TFPath,
		TFPathExplicitlySet:     runnerOpts.TFPathExplicitlySet,
		TerraformCliArgs:        runnerOpts.TerraformCliArgs,
		TerragruntConfigPath:    runnerOpts.TerragruntConfigPath,
		WorkingDir:              runnerOpts.WorkingDir,
		Writer:                  runnerOpts.Writer,
		ErrWriter:               runnerOpts.ErrWriter,
		Env:                     runnerOpts.Env,
		TerraformImplementation: runnerOpts.TerraformImplementation,
	}

	if !runnerOpts.TFPathExplicitlySet {
		if tfPath, err := getTfPathFromConfig(ctx, l, opts); err != nil {
			return err
		} else if tfPath != "" {
			opts.TFPath = tfPath
			runnerOpts.TFPath = tfPath
		}
	}

	return tf.RunCommand(ctx, l, opts, runnerOpts.TerraformCliArgs...)
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
