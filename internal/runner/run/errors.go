package run

import (
	"fmt"
	"strings"
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

// VarsFileExists is returned when a unit that runs in place already has a file where Terragrunt writes inputs for
// the run.
type VarsFileExists struct {
	Path string
}

func (err VarsFileExists) Error() string {
	return fmt.Sprintf(
		"%s already exists in the unit directory. Terragrunt writes inputs to this file for the run "+
			"and removes it afterwards, so it does not replace an existing file. Remove it if an interrupted "+
			"Terragrunt run left it behind; otherwise move or rename it.",
		err.Path,
	)
}

// InvalidVariableNameError is returned when a tfvar input is named with something other than an HCL identifier.
type InvalidVariableNameError struct {
	Name string
}

func (err InvalidVariableNameError) Error() string {
	return fmt.Sprintf(
		"input %q cannot be written to %s because it is not a valid variable name",
		err.Name,
		TFVarsFile,
	)
}

// UndeclaredInputsError is returned when a variables block with validate = true sets variables the module does not
// declare.
type UndeclaredInputsError struct {
	ModulePath string
	Names      []string
}

func (err UndeclaredInputsError) Error() string {
	return fmt.Sprintf(
		"a variables block with validate = true sets variables the module in %s does not declare: %s",
		err.ModulePath,
		strings.Join(err.Names, ", "),
	)
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
