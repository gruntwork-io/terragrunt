// Package command provides the implementation of the terragrunt scaffold command
// This command is used to scaffold a new Terraform module in the current directory
// based on the template specified in the source URL.
package command

import (
	"context"
	"fmt"
	"io"

	"github.com/gruntwork-io/terragrunt/cli/commands/catalog/module"

	"github.com/gruntwork-io/terragrunt/cli/commands/scaffold"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/pkg/log"
)

// Scaffold is the struct that represents the terragrunt scaffold command.
type Scaffold struct {
	module            *module.Module
	terragruntOptions *options.TerragruntOptions
}

// NewScaffold creates a new instance of the Scaffold struct.
func NewScaffold(opts *options.TerragruntOptions, module *module.Module) *Scaffold {
	return &Scaffold{
		module:            module,
		terragruntOptions: opts,
	}
}

// Run runs the terragrunt scaffold command.
func (cmd *Scaffold) Run() error {
	log.Infof("Run Scaffold for the module: %q", cmd.module.TerraformSourcePath())

	err := scaffold.Run(context.Background(), cmd.terragruntOptions, cmd.module.TerraformSourcePath(), "")
	if err != nil {
		return fmt.Errorf("error running scaffold: %w", err)
	}

	return nil
}

// SetStdin sets the standard input for the command.
func (cmd *Scaffold) SetStdin(io.Reader) {
}

// SetStdout sets the standard output for the command.
func (cmd *Scaffold) SetStdout(io.Writer) {
}

// SetStderr sets the standard error for the command.
func (cmd *Scaffold) SetStderr(io.Writer) {
}
