package catalog

import (
	"github.com/gruntwork-io/terragrunt/cli/commands/catalog/service"
	"github.com/gruntwork-io/terragrunt/cli/commands/catalog/tui"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/pkg/cli"
	"github.com/pkg/errors"
)

func Run(ctx *cli.Context, opts *options.TerragruntOptions) error {
	var rootPath string

	if val := ctx.Args().Get(0); val != "" {
		rootPath = val
	}

	modules, err := service.FindModules(ctx, rootPath)
	if err != nil {
		return err
	}
	if len(modules) == 0 {
		return errors.Errorf("specified repository does not contain modules")
	}

	return tui.Run(ctx.Context, modules)
}
