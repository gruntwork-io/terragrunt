package options

import (
	"log"
	"path/filepath"
	"github.com/gruntwork-io/terragrunt/util"
	"fmt"
	"github.com/gruntwork-io/terragrunt/errors"
)

// TerragruntOptions represents options that configure the behavior of the Terragrunt program
type TerragruntOptions struct {
	// Location of the .terragrunt config file
	TerragruntConfigPath string

	// Location of the terraform binary
	TerraformPath        string

	// Whether we should prompt the user for confirmation or always assume "yes"
	NonInteractive       bool

	// CLI args that are intended for Terraform (i.e. all the CLI args except the --terragrunt ones)
	TerraformCliArgs     []string

	// The working directory in which to run Terraform
	WorkingDir           string

	// The logger to use for all logging
	Logger               *log.Logger

	// Environment variables at runtime
	Env                  map[string]string

	// Download Terraform configurations from the specified source location into a temporary folder and run
	// Terraform in that temporary folder
	Source               string

	// If set to true, delete the contents of the temporary folder before downloading Terraform source code into it
	SourceUpdate  bool

	// A command that can be used to run Terragrunt with the given options. This is useful for running Terragrunt
	// multiple times (e.g. when spinning up a stack of Terraform modules). The actual command is normally defined
	// in the cli package, which depends on almost all other packages, so we declare it here so that other
	// packages can use the command without a direct reference back to the cli package (which would create a
	// circular dependency).
	RunTerragrunt func(*TerragruntOptions) error
}

// Create a new TerragruntOptions object with reasonable defaults for real usage
func NewTerragruntOptions(terragruntConfigPath string) *TerragruntOptions {
	workingDir := filepath.Dir(terragruntConfigPath)

	return &TerragruntOptions{
		TerragruntConfigPath: terragruntConfigPath,
		TerraformPath: "terraform",
		NonInteractive: false,
		TerraformCliArgs: []string{},
		WorkingDir: workingDir,
		Logger: util.CreateLogger(""),
		Env: map[string]string{},
		Source: "",
		SourceUpdate: false,
		RunTerragrunt: func(terragruntOptions *TerragruntOptions) error {
			return errors.WithStackTrace(RunTerragruntCommandNotSet)
		},
	}
}

// Create a new TerragruntOptions object with reasonable defaults for test usage
func NewTerragruntOptionsForTest(terragruntConfigPath string) *TerragruntOptions {
	opts := NewTerragruntOptions(terragruntConfigPath)

	opts.NonInteractive = true

	return opts
}

// Create a copy of this TerragruntOptions, but with different values for the given variables. This is useful for
// creating a TerragruntOptions that behaves the same way, but is used for a Terraform module in a different folder.
func (terragruntOptions *TerragruntOptions) Clone(terragruntConfigPath string) *TerragruntOptions {
	workingDir := filepath.Dir(terragruntConfigPath)

	return &TerragruntOptions {
		TerragruntConfigPath: terragruntConfigPath,
		TerraformPath: terragruntOptions.TerraformPath,
		NonInteractive: terragruntOptions.NonInteractive,
		TerraformCliArgs: terragruntOptions.TerraformCliArgs,
		WorkingDir: workingDir,
		Logger: util.CreateLogger(workingDir),
		Env: terragruntOptions.Env,
		Source: terragruntOptions.Source,
		SourceUpdate: terragruntOptions.SourceUpdate,
		RunTerragrunt: terragruntOptions.RunTerragrunt,
	}
}

// Custom error types

var RunTerragruntCommandNotSet = fmt.Errorf("The RunTerragrunt option has not been set on this TerragruntOptions object")