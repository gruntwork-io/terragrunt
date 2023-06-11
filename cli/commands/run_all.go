package command

import (
	"fmt"

	"github.com/gruntwork-io/terragrunt/configstack"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/pkg/cli"
	"github.com/gruntwork-io/terragrunt/shell"
)

const (
	CmdRunAll = "run-all"
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

func NewRunAllCommand(opts *options.TerragruntOptions) *cli.Command {
	command := &cli.Command{
		Name:        CmdRunAll,
		Usage:       "Run a terraform command against a 'stack' by running the specified command in each subfolder.",
		Description: "Run a terraform command against a 'stack' by running the specified command in each subfolder. E.g., to run 'terragrunt apply' in each subfolder, use 'terragrunt run-all apply'.",
		Action:      func(ctx *cli.Context) error { return RunAll(opts) },
	}

	return command
}

func RunAll(terragruntOptions *options.TerragruntOptions) error {
	if terragruntOptions.TerraformCommand == "" {
		return MissingCommand{}
	}
	reason, isDisabled := runAllDisabledCommands[terragruntOptions.TerraformCommand]
	if isDisabled {
		return RunAllDisabledErr{
			command: terragruntOptions.TerraformCommand,
			reason:  reason,
		}
	}

	stack, err := configstack.FindStackInSubfolders(terragruntOptions, nil)
	if err != nil {
		return err
	}

	terragruntOptions.Logger.Debugf("%s", stack.String())
	if err := stack.LogModuleDeployOrder(terragruntOptions.Logger, terragruntOptions.TerraformCommand); err != nil {
		return err
	}

	var prompt string
	switch terragruntOptions.TerraformCommand {
	case "apply":
		prompt = "Are you sure you want to run 'terragrunt apply' in each folder of the stack described above?"
	case "destroy":
		prompt = "WARNING: Are you sure you want to run `terragrunt destroy` in each folder of the stack described above? There is no undo!"
	case "state":
		prompt = "Are you sure you want to manipulate the state with `terragrunt state` in each folder of the stack described above? Note that absolute paths are shared, while relative paths will be relative to each working directory."
	}
	if prompt != "" {
		shouldRunAll, err := shell.PromptUserForYesNo(prompt, terragruntOptions)
		if err != nil {
			return err
		}
		if shouldRunAll == false {
			return nil
		}
	}

	return stack.Run(terragruntOptions)
}

type RunAllDisabledErr struct {
	command string
	reason  string
}

func (err RunAllDisabledErr) Error() string {
	return fmt.Sprintf("%s with run-all is disabled: %s", err.command, err.reason)
}

type MissingCommand struct{}

func (commandName MissingCommand) Error() string {
	return "Missing run-all command argument (Example: terragrunt run-all plan)"
}
