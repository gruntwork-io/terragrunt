package catalog

import (
	"context"

	"github.com/gruntwork-io/go-commons/errors"
	"github.com/gruntwork-io/terragrunt/cli/commands/catalog/module"
	"github.com/gruntwork-io/terragrunt/cli/commands/catalog/tui"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/pkg/log"
)

func Run(ctx context.Context, opts *options.TerragruntOptions, repoURL string) error {
	log.SetLogger(opts.Logger.Logger)

	repoURLs := []string{repoURL}
	if repoURLs[0] == "" {
		config := NewConfig()

		if configPath := findFileInParentDirs(opts.TerragruntConfigPath); configPath != "" {
			opts.TerragruntConfigPath = configPath
		}

		if err := config.Load(opts.TerragruntConfigPath); err != nil {
			return nil
		}

		if len(config.URLs) > 0 {
			repoURLs = config.URLs
		}
	}

	var modules module.Modules

	for _, repoURL := range repoURLs {
		repo, err := module.NewRepo(ctx, repoURL)
		if err != nil {
			return err
		}

		repoModules, err := repo.FindModules(ctx)
		if err != nil {
			return err
		}

		modules = append(modules, repoModules...)
	}

	if len(modules) == 0 {
		return errors.Errorf("specified repository %q does not contain modules", repoURL)
	}

	return tui.Run(ctx, modules, opts)
}
