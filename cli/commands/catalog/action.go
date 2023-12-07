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
	var repoPath string

	if val := ctx.Args().Get(0); val != "" {
		repoPath = val
	}

	log.SetLogger(opts.Logger.Logger)

	repo, err := module.NewRepo(ctx, repoPath)
	if err != nil {
		return err
	}
	//nolint:errcheck
	defer repo.RemoveTempData()

	modules, err := repo.FindModules(ctx)
	if err != nil {
		return err
	}
	if len(modules) == 0 {
		return errors.Errorf("specified repository %q does not contain modules", repoPath)
	}

	return tui.Run(ctx.Context, modules)
}
