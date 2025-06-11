// Package command provides the implementation of the terragrunt scaffold command
// This command is used to scaffold a new Terragrunt unit in the current directory.
package command

import (
	"context"
	"io"

	"github.com/gruntwork-io/terragrunt/internal/services/catalog"
	"github.com/gruntwork-io/terragrunt/internal/services/catalog/module"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/pkg/log"
)

type Scaffold struct {
	module            *module.Module
	terragruntOptions *options.TerragruntOptions
	svc               catalog.CatalogService
	logger            log.Logger
}

func NewScaffold(logger log.Logger, opts *options.TerragruntOptions, svc catalog.CatalogService, module *module.Module) *Scaffold {
	return &Scaffold{
		module:            module,
		terragruntOptions: opts,
		svc:               svc,
		logger:            logger,
	}
}

func (cmd *Scaffold) Run() error {
	return cmd.svc.Scaffold(context.Background(), cmd.logger, cmd.terragruntOptions, cmd.module)
}

func (cmd *Scaffold) SetStdin(io.Reader) {
}

func (cmd *Scaffold) SetStdout(io.Writer) {
}

func (cmd *Scaffold) SetStderr(io.Writer) {
}
