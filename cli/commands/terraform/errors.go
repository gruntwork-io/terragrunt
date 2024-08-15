package terraform

import (
	"fmt"
	"strings"

	"github.com/gruntwork-io/terragrunt/options"
)

// Custom error types

// MissingCommandError represents an error where the user did not specify a Terraform command.
type MissingCommandError struct{}

// Error returns the string representation of the error.
func (err MissingCommandError) Error() string {
	return "Missing terraform command (Example: terragrunt plan)"
}

// WrongTerraformCommandError represents an error where the user did not specify a Terraform version.
type WrongTerraformCommandError string

// Error returns the string representation of the error.
func (name WrongTerraformCommandError) Error() string {
	return fmt.Sprintf(
		"Terraform has no command named %q. To see all of Terraform's top-level commands, run: terraform -help",
		string(name),
	)
}

// WrongTofuCommandError represents an error where the user did not specify a Terraform version.
type WrongTofuCommandError string

// Error returns the string representation of the error.
func (name WrongTofuCommandError) Error() string {
	return fmt.Sprintf(
		"OpenTofu has no command named %q. To see all of OpenTofu's top-level commands, run: tofu -help",
		string(name),
	)
}

// BackendNotDefinedError represents an error where the user has remote state settings
// in their Terragrunt config file, but no backend block in their Terraform code.
type BackendNotDefinedError struct {
	Opts        *options.TerragruntOptions
	BackendType string
}

// Error returns the string representation of the error.
func (err BackendNotDefinedError) Error() string {
	return fmt.Sprintf(
		"Found remote_state settings in %s but no backend block in the Terraform code in %s. You must define a backend block (it can be empty!) in your Terraform code or your remote state settings will have no effect! It should look something like this:\n\nterraform {\n  backend \"%s\" {}\n}\n\n", //nolint:lll
		err.Opts.TerragruntConfigPath,
		err.Opts.WorkingDir,
		err.BackendType,
	)
}

// NoTerraformFilesFoundError represents an error where no Terraform files were found in the specified path.
type NoTerraformFilesFoundError string

// Error returns the string representation of the error.
func (path NoTerraformFilesFoundError) Error() string {
	return "Did not find any Terraform files (*.tf) in " + string(path)
}

// ModuleIsProtectedError represents an error where the user has set the prevent_destroy
// flag to true in their Terragrunt config file, which means they don't want to allow
// anyone to run 'terragrunt destroy' on this module.
type ModuleIsProtectedError struct {
	Opts *options.TerragruntOptions
}

// Error returns the string representation of the error.
func (err ModuleIsProtectedError) Error() string {
	return fmt.Sprintf(
		"Module is protected by the prevent_destroy flag in %s. Set it to false or delete it to allow destroying of the module.", //nolint:lll
		err.Opts.TerragruntConfigPath,
	)
}

// MaxRetriesExceededError represents an error where the user has exceeded the maximum number of retries for a command.
type MaxRetriesExceededError struct {
	Opts *options.TerragruntOptions
}

// Error returns the string representation of the error.
func (err MaxRetriesExceededError) Error() string {
	return fmt.Sprintf(
		"Exhausted retries (%v) for command %v %v",
		err.Opts.RetryMaxAttempts,
		err.Opts.TerraformPath,
		strings.Join(err.Opts.TerraformCliArgs, " "),
	)
}
