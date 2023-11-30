package catalog

import (
	"github.com/gruntwork-io/terragrunt/cli/commands/catalog/module"
	"github.com/gruntwork-io/terragrunt/cli/commands/catalog/tui"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/pkg/cli"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/pkg/errors"
)

func Run(ctx *cli.Context, opts *options.TerragruntOptions) error {
	var rootPath string

	if val := ctx.Args().Get(0); val != "" {
		rootPath = val
	}

	log.SetLogger(opts.Logger.Logger)

	modules, err := module.FindModules(ctx, rootPath)
	if err != nil {
		return err
	}
	if len(modules) == 0 {
		return errors.Errorf("specified repository does not contain modules")
	}

	return tui.Run(ctx.Context, modules)
}
