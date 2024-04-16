package command

import (
	"context"
	"io"

	"github.com/gruntwork-io/terragrunt/cli/commands/catalog/module"

	"github.com/gruntwork-io/terragrunt/cli/commands/scaffold"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/pkg/log"
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
	log.Infof("Run Scaffold for the module: %q", cmd.module.TerraformSourcePath())

	return scaffold.Run(context.Background(), cmd.terragruntOptions, cmd.module.TerraformSourcePath(), "")
}

func (cmd *Scaffold) SetStdin(io.Reader) {
}

func (cmd *Scaffold) SetStdout(io.Writer) {
}

func (cmd *Scaffold) SetStderr(io.Writer) {
}
