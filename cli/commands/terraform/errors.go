package terraform

import (
	"fmt"
	"strings"

	"github.com/gruntwork-io/terragrunt/options"
)

// Custom error types

type UnrecognizedCommand string

func (commandName UnrecognizedCommand) Error() string {
	return fmt.Sprintf("Unrecognized command: %s", string(commandName))
}

type ArgumentNotAllowed struct {
	Argument string
	Message  string
}

func (err ArgumentNotAllowed) Error() string {
	return fmt.Sprintf(err.Message, err.Argument)
}

type BackendNotDefined struct {
	Opts        *options.TerragruntOptions
	BackendType string
}

func (err BackendNotDefined) Error() string {
	return fmt.Sprintf("Found remote_state settings in %s but no backend block in the Terraform code in %s. You must define a backend block (it can be empty!) in your Terraform code or your remote state settings will have no effect! It should look something like this:\n\nterraform {\n  backend \"%s\" {}\n}\n\n", err.Opts.TerragruntConfigPath, err.Opts.WorkingDir, err.BackendType)
}

type NoTerraformFilesFound string

func (path NoTerraformFilesFound) Error() string {
	return fmt.Sprintf("Did not find any Terraform files (*.tf) in %s", string(path))
}

type ModuleIsProtected struct {
	Opts *options.TerragruntOptions
}

func (err ModuleIsProtected) Error() string {
	return fmt.Sprintf("Module is protected by the prevent_destroy flag in %s. Set it to false or delete it to allow destroying of the module.", err.Opts.TerragruntConfigPath)
}

type MaxRetriesExceeded struct {
	Opts *options.TerragruntOptions
}

func (err MaxRetriesExceeded) Error() string {
	return fmt.Sprintf("Exhausted retries (%v) for command %v %v", err.Opts.RetryMaxAttempts, err.Opts.TerraformPath, strings.Join(err.Opts.TerraformCliArgs, " "))
}
