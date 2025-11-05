package run

import (
	"fmt"

	"github.com/gruntwork-io/terragrunt/options"
)

// Custom error types

type MissingCommand struct{}

func (err MissingCommand) Error() string {
	return "Missing terraform command (Example: terragrunt run plan)"
}

type WrongTerraformCommand string

func (name WrongTerraformCommand) Error() string {
	return fmt.Sprintf("Terraform has no command named %q. To see all of Terraform's top-level commands, run: terraform -help", string(name))
}

type WrongTofuCommand string

func (name WrongTofuCommand) Error() string {
	return fmt.Sprintf("OpenTofu has no command named %q. To see all of OpenTofu's top-level commands, run: tofu -help", string(name))
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
	return "Did not find any Terraform files (*.tf) or OpenTofu files (*.tofu) in " + string(path)
}

type ModuleIsProtected struct {
	Opts *options.TerragruntOptions
}

func (err ModuleIsProtected) Error() string {
	return fmt.Sprintf("Unit is protected by the prevent_destroy flag in %s. Set it to false or remove it to allow destruction of the unit.", err.Opts.TerragruntConfigPath)
}

// Legacy retry error removed in favor of error handling via options.Errors

type RunAllDisabledErr struct {
	command string
	reason  string
}

func (err RunAllDisabledErr) Error() string {
	return fmt.Sprintf("%s with run --all is disabled: %s", err.command, err.reason)
}
