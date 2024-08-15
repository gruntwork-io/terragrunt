package runall

import (
	"context"
	"fmt"

	"github.com/gruntwork-io/go-commons/errors"
	"github.com/gruntwork-io/terragrunt/configstack"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/shell"
	"github.com/gruntwork-io/terragrunt/telemetry"
	"github.com/gruntwork-io/terragrunt/terraform"
)

// Known terraform commands that are explicitly not supported in run-all due to the nature of the command. This is
// tracked as a map that maps the terraform command to the reasoning behind disallowing the command in run-all.
var runAllDisabledCommands = map[string]string{ //nolint:gochecknoglobals
	terraform.CommandNameImport:      "terraform import should only be run against a single state representation to avoid injecting the wrong object in the wrong state representation.", //nolint:lll
	terraform.CommandNameTaint:       "terraform taint should only be run against a single state representation to avoid using the wrong state address.",                                 //nolint:lll
	terraform.CommandNameUntaint:     "terraform untaint should only be run against a single state representation to avoid using the wrong state address.",                               //nolint:lll
	terraform.CommandNameConsole:     "terraform console requires stdin, which is shared across all instances of run-all when multiple modules run concurrently.",                        //nolint:lll
	terraform.CommandNameForceUnlock: "lock IDs are unique per state representation and thus should not be run with run-all.",                                                            //nolint:lll

	// MAINTAINER'S NOTE: There are a few other commands that might not make sense, but we deliberately allow it for
	// certain use cases that are documented here:
	// - state          : Supporting `state` with run-all could be useful for a mass pull and push operation, which can
	//                    be done en masse with the use of relative pathing.
	// - login / logout : Supporting `login` with run-all could be useful when used in conjunction with mise and
	//                    multi-terraform version setups, where multiple terraform versions need to be configured.
	// - version        : Supporting `version` with run-all could be useful for sanity checking a multi-version setup.
}

// Run runs the command.
func Run(ctx context.Context, opts *options.TerragruntOptions) error {
	if opts.TerraformCommand == "" {
		return fmt.Errorf("command is missing: %w", errors.WithStackTrace(MissingCommandError{}))
	}

	reason, isDisabled := runAllDisabledCommands[opts.TerraformCommand]
	if isDisabled {
		return DisabledError{
			command: opts.TerraformCommand,
			reason:  reason,
		}
	}

	stack, err := configstack.FindStackInSubfolders(ctx, opts)
	if err != nil {
		return fmt.Errorf("could not find stack in subfolders: %w", err)
	}

	return OnStack(ctx, opts, stack)
}

// OnStack runs the specified Terraform command in each subfolder of the given stack.
func OnStack(ctx context.Context, opts *options.TerragruntOptions, stack *configstack.Stack) error {
	opts.Logger.Debugf("%s", stack.String())

	if err := stack.LogModuleDeployOrder(opts.Logger, opts.TerraformCommand); err != nil {
		return fmt.Errorf("error logging module deploy order: %w", err)
	}

	var prompt string

	switch opts.TerraformCommand {
	case terraform.CommandNameApply:
		prompt = "Are you sure you want to run 'terragrunt apply' in each folder of the stack described above?"
	case terraform.CommandNameDestroy:
		prompt = "WARNING: Are you sure you want to run `terragrunt destroy` in each folder of the stack described above? There is no undo!" //nolint:lll
	case terraform.CommandNameState:
		prompt = "Are you sure you want to manipulate the state with `terragrunt state` in each folder of the stack described above? Note that absolute paths are shared, while relative paths will be relative to each working directory." //nolint:lll
	}

	if prompt != "" {
		shouldRunAll, err := shell.PromptUserForYesNo(prompt, opts)
		if err != nil {
			return fmt.Errorf("error prompting user: %w", err)
		}

		if !shouldRunAll {
			return nil
		}
	}

	err := telemetry.Telemetry(ctx, opts, "run_all_on_stack", map[string]interface{}{
		"terraform_command": opts.TerraformCommand,
		"working_dir":       opts.WorkingDir,
	}, func(_ context.Context) error {
		// BUG: This might be a bug. Should this be `childCtx` instead of `ctx`? For now, I'm going to leave it as is.
		err := stack.Run(ctx, opts)
		if err != nil {
			return fmt.Errorf("error running stack: %w", err)
		}

		return nil
	})

	if err != nil {
		return fmt.Errorf("error running telemetry: %w", err)
	}

	return nil
}
