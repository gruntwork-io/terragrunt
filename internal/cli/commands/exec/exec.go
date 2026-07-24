package exec

import (
	"context"
	"fmt"

	"github.com/gruntwork-io/terragrunt/internal/clihelper"
	"github.com/gruntwork-io/terragrunt/internal/configbridge"
	"github.com/gruntwork-io/terragrunt/internal/prepare"
	"github.com/gruntwork-io/terragrunt/internal/report"
	"github.com/gruntwork-io/terragrunt/internal/runner/run"
	"github.com/gruntwork-io/terragrunt/internal/runner/runcfg"
	"github.com/gruntwork-io/terragrunt/internal/shell"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/pkg/options"
)

func Run(
	ctx context.Context,
	l log.Logger,
	v venv.Venv,
	opts *options.TerragruntOptions,
	cmdOpts *Options,
	args clihelper.Args,
) error {
	prepared, err := prepare.PrepareConfig(ctx, l, v, opts)
	if err != nil {
		return err
	}

	r := report.NewReport()

	// Download source
	updatedOpts, err := prepare.PrepareSource(ctx, l, v, prepared.Opts, prepared.Cfg, r)
	if err != nil {
		return err
	}

	runCfg := prepared.Cfg.ToRunConfig(l)

	// Generate config
	if err := prepare.PrepareGenerate(l, v, updatedOpts, runCfg); err != nil {
		return err
	}

	if cmdOpts.InDownloadDir {
		// Run terraform init
		if err := prepare.PrepareInit(ctx, l, v, opts, updatedOpts, runCfg, r); err != nil {
			return err
		}
	} else {
		// Just set inputs as env vars, skip init
		updatedOpts.AutoInit = false

		if err := prepare.PrepareInputsAsEnvVars(l, v, updatedOpts, runCfg); err != nil {
			return err
		}
	}

	return runTargetCommand(ctx, l, v, updatedOpts, runCfg, r, cmdOpts, args)
}

func runTargetCommand(
	ctx context.Context,
	l log.Logger,
	v venv.Venv,
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

	runOpts := configbridge.NewRunOptions(opts)

	return run.RunActionWithHooks(
		ctx,
		l,
		v,
		command,
		runOpts,
		cfg,
		r,
		func(ctx context.Context) error {
			_, err := shell.RunCommandWithOutput(
				ctx,
				l,
				v,
				configbridge.ShellRunOptsFromOpts(opts),
				dir,
				false,
				false,
				command,
				cmdArgs...,
			)
			if err != nil {
				return fmt.Errorf("failed to run command in directory %s: %w", dir, err)
			}

			return nil
		},
	)
}
