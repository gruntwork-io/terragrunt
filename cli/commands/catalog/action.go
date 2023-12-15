package catalog

import (
	"context"

	"github.com/gruntwork-io/go-commons/errors"
	"github.com/gruntwork-io/terragrunt/cli/commands/catalog/module"
	"github.com/gruntwork-io/terragrunt/cli/commands/catalog/tui"
	"github.com/gruntwork-io/terragrunt/config"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/util"
)

func Run(ctx context.Context, opts *options.TerragruntOptions, repoURL string) error {
	log.SetLogger(opts.Logger.Logger)

	repoURLs := []string{repoURL}

	if repoURLs[0] == "" {
		if configPath, err := config.FindInParentFolders([]string{opts.TerragruntConfigPath}, nil, opts); err != nil {
			return err
		} else if configPath != "" {
			opts.TerragruntConfigPath = configPath
		}

		config, err := config.ReadTerragruntConfig(opts)
		if err != nil {
			return err
		}

		if len(config.Catalog.URLs) > 0 {
			repoURLs = config.Catalog.URLs
		}
	}

	repoURLs = util.RemoveDuplicatesFromList(repoURLs)

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

		log.Infof("Found %d modules in repository %q", len(repoModules), repoURL)

		modules = append(modules, repoModules...)
	}

	if len(modules) == 0 {
		return errors.Errorf("no modules found")
	}

	return tui.Run(ctx, modules, opts)
}
