package catalog

import (
	"context"

	"github.com/gruntwork-io/terragrunt/cli/commands/catalog/module"
	"github.com/gruntwork-io/terragrunt/cli/commands/catalog/tui"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/pkg/errors"
)

func Run(ctx context.Context, opts *options.TerragruntOptions, repoPath string) error {
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

	return tui.Run(ctx, modules, opts)
}
