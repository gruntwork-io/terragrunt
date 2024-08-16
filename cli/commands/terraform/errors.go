package terraform

import (
	"fmt"
	"strings"

	"github.com/gruntwork-io/terragrunt/options"
)

// Custom error types

type MissingCommand struct{}

func (err MissingCommand) Error() string {
	return "Missing terraform command (Example: terragrunt plan)"
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
	return "Did not find any Terraform files (*.tf) in " + string(path)
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
