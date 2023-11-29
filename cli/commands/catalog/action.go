package catalog

import (
	"github.com/gruntwork-io/terragrunt/cli/commands/catalog/service"
	"github.com/gruntwork-io/terragrunt/cli/commands/catalog/tui"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/pkg/cli"
)

func Run(ctx *cli.Context, opts *options.TerragruntOptions) error {
	var rootPath string

	if val := ctx.Args().Get(0); val != "" {
		rootPath = val
	}

	modules, err := service.FindModules(rootPath)
	if err != nil {
		return err
	}

	return tui.Run(ctx.Context, modules)
}
