package runall

import (
	"context"
	"os"

	"github.com/gruntwork-io/terragrunt/configstack"
	"github.com/gruntwork-io/terragrunt/internal/cli"
	"github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/gruntwork-io/terragrunt/internal/experiment"
	"github.com/gruntwork-io/terragrunt/internal/os/stdout"
	"github.com/gruntwork-io/terragrunt/internal/report"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/shell"
	"github.com/gruntwork-io/terragrunt/telemetry"
	"github.com/gruntwork-io/terragrunt/tf"
)

// Known terraform commands that are explicitly not supported in run --all due to the nature of the command. This is
// tracked as a map that maps the terraform command to the reasoning behind disallowing the command in run --all.
var runAllDisabledCommands = map[string]string{
	tf.CommandNameImport:      "terraform import should only be run against a single state representation to avoid injecting the wrong object in the wrong state representation.",
	tf.CommandNameTaint:       "terraform taint should only be run against a single state representation to avoid using the wrong state address.",
	tf.CommandNameUntaint:     "terraform untaint should only be run against a single state representation to avoid using the wrong state address.",
	tf.CommandNameConsole:     "terraform console requires stdin, which is shared across all instances of run --all when multiple modules run concurrently.",
	tf.CommandNameForceUnlock: "lock IDs are unique per state representation and thus should not be run with run --all.",

	// MAINTAINER'S NOTE: There are a few other commands that might not make sense, but we deliberately allow it for
	// certain use cases that are documented here:
	// - state          : Supporting `state` with run --all could be useful for a mass pull and push operation, which can
	//                    be done en masse with the use of relative pathing.
	// - login / logout : Supporting `login` with run --all could be useful when used in conjunction with mise and
	//                    multi-terraform version setups, where multiple terraform versions need to be configured.
	// - version        : Supporting `version` with run --all could be useful for sanity checking a multi-version setup.
}

func Run(ctx context.Context, l log.Logger, opts *options.TerragruntOptions) error {
	if opts.TerraformCommand == "" {
		return errors.New(MissingCommand{})
	}

	reason, isDisabled := runAllDisabledCommands[opts.TerraformCommand]
	if isDisabled {
		return RunAllDisabledErr{
			command: opts.TerraformCommand,
			reason:  reason,
		}
	}

	stackOpts := []configstack.Option{}

	if opts.Experiments.Evaluate(experiment.Report) {
		r := report.NewReport().WithWorkingDir(opts.WorkingDir)

		if l.Formatter().DisabledColors() || stdout.IsRedirected() {
			r.WithDisableColor()
		}

		if opts.ReportFormat != "" {
			r.WithFormat(opts.ReportFormat)
		}

		if opts.SummaryPerUnit {
			r.WithShowUnitLevelSummary()
		}

		stackOpts = append(stackOpts, configstack.WithReport(r))

		if opts.ReportSchemaFile != "" {
			defer r.WriteSchemaToFile(opts.ReportSchemaFile) //nolint:errcheck
		}

		if opts.ReportFile != "" {
			defer r.WriteToFile(opts.ReportFile) //nolint:errcheck
		}

		if !opts.SummaryDisable {
			defer r.WriteSummary(opts.Writer) //nolint:errcheck
		}
	}

	stack, err := configstack.FindStackInSubfolders(ctx, l, opts, stackOpts...)
	if err != nil {
		return err
	}

	return RunAllOnStack(ctx, l, opts, stack)
}

func RunAllOnStack(ctx context.Context, l log.Logger, opts *options.TerragruntOptions, stack configstack.Stack) error {
	l.Debugf("%s", stack.String())

	if err := stack.LogModuleDeployOrder(l, opts.TerraformCommand); err != nil {
		return err
	}

	var prompt string

	switch opts.TerraformCommand {
	case tf.CommandNameApply:
		prompt = "Are you sure you want to run 'terragrunt apply' in each folder of the stack described above?"
	case tf.CommandNameDestroy:
		prompt = "WARNING: Are you sure you want to run `terragrunt destroy` in each folder of the stack described above? There is no undo!"
	case tf.CommandNameState:
		prompt = "Are you sure you want to manipulate the state with `terragrunt state` in each folder of the stack described above? Note that absolute paths are shared, while relative paths will be relative to each working directory."
	}

	if prompt != "" {
		shouldRunAll, err := shell.PromptUserForYesNo(ctx, l, prompt, opts)
		if err != nil {
			return err
		}

		if !shouldRunAll {
			// We explicitly exit here to avoid running any defers that might be registered, like from the run summary.
			os.Exit(0)
		}
	}

	return telemetry.TelemeterFromContext(ctx).Collect(ctx, "run_all_on_stack", map[string]any{
		"terraform_command": opts.TerraformCommand,
		"working_dir":       opts.WorkingDir,
	}, func(ctx context.Context) error {
		err := stack.Run(ctx, l, opts)
		if err != nil {
			// At this stage, we can't handle the error any further, so we just log it and return nil.
			// After this point, we'll need to report on what happened, and we want that to happen
			// after the error summary.
			l.Errorf("Run failed: %v", err)

			// Update the exit code in ctx
			exitCode := tf.DetailedExitCodeFromContext(ctx)
			if exitCode == nil {
				exitCode = &tf.DetailedExitCode{
					Code: 1,
				}
			}

			exitCode.Set(int(cli.ExitCodeGeneralError))

			return nil
		}

		return nil
	})
}
