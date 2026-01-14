package exec

import (
	"context"

	"github.com/gruntwork-io/terragrunt/internal/clihelper"
	"github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/gruntwork-io/terragrunt/internal/report"
	"github.com/gruntwork-io/terragrunt/internal/runner/run"
	"github.com/gruntwork-io/terragrunt/internal/runner/runcfg"
	"github.com/gruntwork-io/terragrunt/internal/shell"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/pkg/options"
)

func Run(ctx context.Context, l log.Logger, opts *options.TerragruntOptions, cmdOpts *Options, args clihelper.Args) error {
	prepared, err := run.PrepareConfig(ctx, l, opts)
	if err != nil {
		return err
	}

	r := report.NewReport()

	// Download source
	updatedOpts, err := run.PrepareSource(ctx, l, prepared.Opts, prepared.Cfg, r)
	if err != nil {
		return err
	}

	runCfg := prepared.Cfg.ToRunConfig()

	// Generate config
	if err := run.PrepareGenerate(l, updatedOpts, runCfg); err != nil {
		return err
	}

	if cmdOpts.InDownloadDir {
		// Run terraform init
		if err := run.PrepareInit(ctx, l, opts, updatedOpts, runCfg, r); err != nil {
			return err
		}
	} else {
		// Just set inputs as env vars, skip init
		updatedOpts.AutoInit = false

		if err := run.PrepareInputsAsEnvVars(l, updatedOpts, runCfg); err != nil {
			return err
		}
	}

	return runTargetCommand(ctx, l, updatedOpts, runCfg, r, cmdOpts, args)
}

func runTargetCommand(
	ctx context.Context,
	l log.Logger,
	opts *options.TerragruntOptions,
	cfg *runcfg.RunConfig,
	r *report.Report,
	cmdOpts *Options,
	args clihelper.Args,
) error {
	var (
		command = args.CommandName()
		cmdArgs = args.Tail()
		dir     = opts.WorkingDir
	)

	if !cmdOpts.InDownloadDir {
		dir = opts.RootWorkingDir
	}

	return run.RunActionWithHooks(ctx, l, command, opts, cfg, r, func(ctx context.Context) error {
		_, err := shell.RunCommandWithOutput(ctx, l, opts, dir, false, false, command, cmdArgs...)
		if err != nil {
			return errors.Errorf("failed to run command in directory %s: %w", dir, err)
		}

		return nil
	})
}
