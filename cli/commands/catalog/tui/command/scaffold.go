// Package command provides the implementation of the terragrunt scaffold command
// This command is used to scaffold a new Terragrunt unit in the current directory.
package command

import (
	"context"
	"io"

	"github.com/gruntwork-io/terragrunt/cli/commands/catalog/service"
	"github.com/gruntwork-io/terragrunt/cli/commands/catalog/service/module"

	"github.com/gruntwork-io/terragrunt/options"
)

type Scaffold struct {
	module            *module.Module
	terragruntOptions *options.TerragruntOptions
	svc               service.CatalogService
}

func NewScaffold(opts *options.TerragruntOptions, svc service.CatalogService, module *module.Module) *Scaffold {
	return &Scaffold{
		module:            module,
		terragruntOptions: opts,
		svc:               svc,
	}
}

func (cmd *Scaffold) Run() error {
	return cmd.svc.Scaffold(context.Background(), cmd.terragruntOptions, cmd.module)
}

func (cmd *Scaffold) SetStdin(io.Reader) {
}

func (cmd *Scaffold) SetStdout(io.Writer) {
}

func (cmd *Scaffold) SetStderr(io.Writer) {
}
