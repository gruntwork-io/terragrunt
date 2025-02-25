// Package command provides the implementation of the terragrunt scaffold command
// This command is used to scaffold a new Terragrunt unit in the current directory.
package command

import (
	"context"
	"io"

	"github.com/gruntwork-io/terragrunt/cli/commands/catalog/module"

	"github.com/gruntwork-io/terragrunt/cli/commands/scaffold"
	"github.com/gruntwork-io/terragrunt/options"
)

type Scaffold struct {
	module            *module.Module
	terragruntOptions *options.TerragruntOptions
}

func NewScaffold(opts *options.TerragruntOptions, module *module.Module) *Scaffold {
	return &Scaffold{
		module:            module,
		terragruntOptions: opts,
	}
}

func (cmd *Scaffold) Run() error {
	cmd.module.Logger().Infof("Run Scaffold for the module: %q", cmd.module.TerraformSourcePath())

	return scaffold.Run(context.Background(), cmd.terragruntOptions, cmd.module.TerraformSourcePath(), "")
}

func (cmd *Scaffold) SetStdin(io.Reader) {
}

func (cmd *Scaffold) SetStdout(io.Writer) {
}

func (cmd *Scaffold) SetStderr(io.Writer) {
}
