package run

import (
	"fmt"
)

// Custom error types

type MissingCommand struct{}

func (err MissingCommand) Error() string {
	return "Missing OpenTofu/Terraform command (Example: terragrunt run plan)"
}

type BackendNotDefined struct {
	ConfigPath  string
	WorkingDir  string
	BackendType string
}

func (err BackendNotDefined) Error() string {
	return fmt.Sprintf(
		"Found remote_state settings in %s but no backend block "+
			"in the OpenTofu/Terraform code in %s. "+
			"You must define a backend block (it can be empty!) in your OpenTofu/Terraform code "+
			"or your remote state settings will have no effect! "+
			"It should look something like this:\n\n"+
			"terraform {\n  backend \"%s\" {}\n}\n\n",
		err.ConfigPath,
		err.WorkingDir,
		err.BackendType,
	)
}

type NoTerraformFilesFound string

func (path NoTerraformFilesFound) Error() string {
	return "Did not find any Terraform files (*.tf) or OpenTofu files (*.tofu) in " + string(path)
}

type ModuleIsProtected struct {
	ConfigPath string
}

func (err ModuleIsProtected) Error() string {
	return fmt.Sprintf(
		"Unit is protected by the prevent_destroy flag in %s. Set it to false or remove it to allow destruction of the unit.",
		err.ConfigPath,
	)
}

// Legacy retry error removed in favor of error handling via options.Errors

type RunAllDisabledErr struct {
	command string
	reason  string
}

func (err RunAllDisabledErr) Error() string {
	return fmt.Sprintf("%s with run --all is disabled: %s", err.command, err.reason)
}
