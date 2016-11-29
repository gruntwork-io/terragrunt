package options

import (
	"log"
	"path/filepath"
	"github.com/gruntwork-io/terragrunt/util"
)

// TerragruntOptions represents options that configure the behavior of the Terragrunt program
type TerragruntOptions struct {
	// Location of the .terragrunt config file
	TerragruntConfigPath string

	// Whether we should prompt the user for confirmation or always assume "yes"
	NonInteractive       bool

	// CLI args that are intended for Terraform (i.e. all the CLI args except the --terragrunt ones)
	TerraformCliArgs     []string

	// The working directory in which to run Terraform
	WorkingDir           string

	// The logger to use for all logging
	Logger               *log.Logger

	// A command that can be used to run Terragrunt with the given options. This is useful for running Terragrunt
	// multiple times (e.g. when spinning up a stack of Terraform modules). The actual command is normally defined
	// in the cli package, which depends on almost all other packages, so we declare it here so that other
	// packages can use the command without a direct reference back to the cli package (which would create a
	// circular dependency).
	RunTerragrunt        func(*TerragruntOptions) error
}

// Create a copy of this TerragruntOptions, but with different values for the given variables. This is useful for
// creating a TerragruntOptions that behaves the same way, but is used for a Terraform module in a different folder.
func (terragruntOptions *TerragruntOptions) Clone(terragruntConfigPath string) *TerragruntOptions {
	workingDir := filepath.Dir(terragruntConfigPath)

	return &TerragruntOptions {
		TerragruntConfigPath: terragruntConfigPath,
		NonInteractive: terragruntOptions.NonInteractive,
		TerraformCliArgs: terragruntOptions.TerraformCliArgs,
		WorkingDir: workingDir,
		Logger: util.CreateLogger(workingDir),
		RunTerragrunt: terragruntOptions.RunTerragrunt,
	}
}