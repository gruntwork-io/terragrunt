package runall

import (
	"github.com/gruntwork-io/go-commons/errors"
	"github.com/gruntwork-io/terragrunt/configstack"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/shell"
)

// Known terraform commands that are explicitly not supported in run-all due to the nature of the command. This is
// tracked as a map that maps the terraform command to the reasoning behind disallowing the command in run-all.
var runAllDisabledCommands = map[string]string{
	"import":       "terraform import should only be run against a single state representation to avoid injecting the wrong object in the wrong state representation.",
	"taint":        "terraform taint should only be run against a single state representation to avoid using the wrong state address.",
	"untaint":      "terraform untaint should only be run against a single state representation to avoid using the wrong state address.",
	"console":      "terraform console requires stdin, which is shared across all instances of run-all when multiple modules run concurrently.",
	"force-unlock": "lock IDs are unique per state representation and thus should not be run with run-all.",

	// MAINTAINER'S NOTE: There are a few other commands that might not make sense, but we deliberately allow it for
	// certain use cases that are documented here:
	// - state          : Supporting `state` with run-all could be useful for a mass pull and push operation, which can
	//                    be done en masse with the use of relative pathing.
	// - login / logout : Supporting `login` with run-all could be useful when used in conjunction with tfenv and
	//                    multi-terraform version setups, where multiple terraform versions need to be configured.
	// - version        : Supporting `version` with run-all could be useful for sanity checking a multi-version setup.
}

func Run(opts *options.TerragruntOptions) error {
	if opts.TerraformCommand == "" {
		return errors.WithStackTrace(MissingCommand{})
	}

	reason, isDisabled := runAllDisabledCommands[opts.TerraformCommand]
	if isDisabled {
		return RunAllDisabledErr{
			command: opts.TerraformCommand,
			reason:  reason,
		}
	}

	stack, err := configstack.FindStackInSubfolders(opts, nil)
	if err != nil {
		return err
	}

	opts.Logger.Debugf("%s", stack.String())
	if err := stack.LogModuleDeployOrder(opts.Logger, opts.TerraformCommand); err != nil {
		return err
	}

	var prompt string
	switch opts.TerraformCommand {
	case "apply":
		prompt = "Are you sure you want to run 'terragrunt apply' in each folder of the stack described above?"
	case "destroy":
		prompt = "WARNING: Are you sure you want to run `terragrunt destroy` in each folder of the stack described above? There is no undo!"
	case "state":
		prompt = "Are you sure you want to manipulate the state with `terragrunt state` in each folder of the stack described above? Note that absolute paths are shared, while relative paths will be relative to each working directory."
	}
	if prompt != "" {
		shouldRunAll, err := shell.PromptUserForYesNo(prompt, opts)
		if err != nil {
			return err
		}
		if !shouldRunAll {
			return nil
		}
	}

	return stack.Run(opts)
}
