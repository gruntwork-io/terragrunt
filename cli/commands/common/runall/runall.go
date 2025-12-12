package runall

import (
	"context"
	"os"
	"path/filepath"

	"github.com/gruntwork-io/terragrunt/config"
	"github.com/gruntwork-io/terragrunt/internal/filter"
	"github.com/gruntwork-io/terragrunt/internal/runner"
	"github.com/gruntwork-io/terragrunt/internal/runner/common"
	"github.com/gruntwork-io/terragrunt/internal/stacks/generate"
	"github.com/gruntwork-io/terragrunt/internal/worktrees"

	"github.com/gruntwork-io/terragrunt/internal/cli"
	"github.com/gruntwork-io/terragrunt/internal/errors"
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

	stackOpts := []common.Option{}

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

	stackOpts = append(stackOpts, common.WithReport(r))

	if opts.ReportSchemaFile != "" {
		defer r.WriteSchemaToFile(opts.ReportSchemaFile) //nolint:errcheck
	}

	if opts.ReportFile != "" {
		defer r.WriteToFile(opts.ReportFile) //nolint:errcheck
	}

	// Skip summary for programmatic interactions:
	// - When JSON output is requested (--json or report format is JSON)
	// - When running 'output' command (typically for programmatic consumption)
	if !opts.SummaryDisable && !shouldSkipSummary(opts) {
		defer r.WriteSummary(opts.Writer) //nolint:errcheck
	}

	filters, err := filter.ParseFilterQueries(opts.FilterQueries)
	if err != nil {
		return errors.Errorf("failed to parse filters: %w", err)
	}

	gitFilters := filters.UniqueGitFilters()

	// Only create worktrees when git filter expressions are present
	var wts *worktrees.Worktrees
	if len(gitFilters) > 0 {
		wts, err = worktrees.NewWorktrees(ctx, l, opts.WorkingDir, gitFilters)
		if err != nil {
			return errors.Errorf("failed to create worktrees: %w", err)
		}

		defer func() {
			cleanupErr := wts.Cleanup(ctx, l)
			if cleanupErr != nil {
				l.Errorf("failed to cleanup worktrees: %v", cleanupErr)
			}
		}()
	}

	if !opts.NoStackGenerate {
		// Set the stack config path to the default location in the working directory
		opts.TerragruntStackConfigPath = filepath.Join(opts.WorkingDir, config.DefaultStackFile)

		// Clean stack folders before calling `generate` when the `--source-update` flag is passed
		if opts.SourceUpdate {
			errClean := telemetry.TelemeterFromContext(ctx).Collect(ctx, "stack_clean", map[string]any{
				"stack_config_path": opts.TerragruntStackConfigPath,
				"working_dir":       opts.WorkingDir,
			}, func(ctx context.Context) error {
				l.Debugf("Running stack clean for %s, as part of generate command", opts.WorkingDir)
				return config.CleanStacks(ctx, l, opts)
			})
			if errClean != nil {
				return errors.Errorf("failed to clean stack directories under %q: %w", opts.WorkingDir, errClean)
			}
		}

		// Generate the stack configuration with telemetry tracking
		err := telemetry.TelemeterFromContext(ctx).Collect(ctx, "stack_generate", map[string]any{
			"stack_config_path": opts.TerragruntStackConfigPath,
			"working_dir":       opts.WorkingDir,
		}, func(ctx context.Context) error {
			return generate.GenerateStacks(ctx, l, opts)
		})

		// Handle any errors during stack generation
		if err != nil {
			return errors.Errorf("failed to generate stack file: %w", err)
		}
	} else {
		l.Debugf("Skipping stack generation in %s", opts.WorkingDir)
	}

	// Pass worktrees to runner for git filter expressions
	if wts != nil && len(wts.WorktreePairs) > 0 {
		stackOpts = append(stackOpts, common.WithWorktrees(wts))
	}

	stack, err := runner.FindStackInSubfolders(ctx, l, opts, stackOpts...)
	if err != nil {
		return err
	}

	return RunAllOnStack(ctx, l, opts, stack)
}

func RunAllOnStack(ctx context.Context, l log.Logger, opts *options.TerragruntOptions, runner common.StackRunner) error {
	l.Debugf("%s", runner.GetStack().String())

	if err := runner.LogUnitDeployOrder(l, opts.TerraformCommand); err != nil {
		return err
	}

	var prompt string

	switch opts.TerraformCommand {
	case tf.CommandNameApply:
		prompt = "Are you sure you want to run 'terragrunt apply' in each unit of the run queue displayed above?"
	case tf.CommandNameDestroy:
		prompt = "WARNING: Are you sure you want to run `terragrunt destroy` in each unit of the run queue displayed above? There is no undo!"
	case tf.CommandNameState:
		prompt = "Are you sure you want to manipulate the state with `terragrunt state` in each unit of the run queue displayed above? Note that absolute paths are shared, while relative paths will be relative to each working directory."
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

	var runErr error

	telemetryErr := telemetry.TelemeterFromContext(ctx).Collect(ctx, "run_all_on_stack", map[string]any{
		"terraform_command": opts.TerraformCommand,
		"working_dir":       opts.WorkingDir,
	}, func(ctx context.Context) error {
		err := runner.Run(ctx, l, opts)
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

			// Save error to potentially return after telemetry completes
			runErr = err

			// Return nil to allow telemetry and reporting to complete
			return nil
		}

		return nil
	})

	// log telemetry error and continue execution
	if telemetryErr != nil {
		l.Warnf("Telemetry collection failed: %v", telemetryErr)
	}

	return runErr
}

// shouldSkipSummary determines if summary output should be skipped for programmatic interactions.
// Summary is skipped when:
// - The command is 'output' (typically used for programmatic consumption)
// - JSON output is requested via terraform CLI args (-json flag)
// - JSON report format is specified (--report-format=json)
func shouldSkipSummary(opts *options.TerragruntOptions) bool {
	// Skip summary for 'output' command as it's typically used programmatically
	if opts.TerraformCommand == tf.CommandNameOutput {
		return true
	}

	// Skip summary when JSON output is requested via -json flag
	if opts.TerraformCliArgs.Normalize(cli.SingleDashFlag).Contains(tf.FlagNameJSON) {
		return true
	}

	return false
}
