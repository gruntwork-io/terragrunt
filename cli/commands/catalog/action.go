package catalog

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/gruntwork-io/terragrunt/cli/commands/catalog/module"
	"github.com/gruntwork-io/terragrunt/cli/commands/catalog/tui"
	"github.com/gruntwork-io/terragrunt/config"
	"github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/gruntwork-io/terragrunt/internal/experiment"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/util"
)

const (
	tempDirFormat = "catalog%x"
)

func Run(ctx context.Context, opts *options.TerragruntOptions, repoURL string) error {
	repoURLs := []string{repoURL}

	if repoURL == "" {
		config, err := config.ReadCatalogConfig(ctx, opts)
		if err != nil {
			return err
		}

		if config != nil && len(config.URLs) > 0 {
			repoURLs = config.URLs
		}
	}

	repoURLs = util.RemoveDuplicatesFromList(repoURLs)

	var modules module.Modules

	walkWithSymlinks := opts.Experiments.Evaluate(experiment.Symlinks)
	allowCAS := opts.Experiments.Evaluate(experiment.CAS)

	for _, repoURL := range repoURLs {
		path := util.EncodeBase64Sha1(repoURL)

		// NOTE: We do this check again later, but it's just to make sure that we're leaving
		// the separation of concerns for the directory destination and repo cloning.
		if strings.HasPrefix(repoURL, "cas://") {
			if !allowCAS {
				return errors.Errorf("cas:// protocol is not allowed without using the `cas` experiment. Please enable the experiment and try again.")
			}

			homeDir, err := os.UserHomeDir()
			if err != nil {
				return err
			}

			path = filepath.Join(homeDir, ".cache", "terragrunt", "cas", "catalog", path)
		} else {
			path = filepath.Join(os.TempDir(), fmt.Sprintf(tempDirFormat, path))
		}

		repo, err := module.NewRepo(ctx, opts.Logger, repoURL, path, walkWithSymlinks, allowCAS)
		if err != nil {
			return err
		}

		repoModules, err := repo.FindModules(ctx)
		if err != nil {
			return err
		}

		opts.Logger.Infof("Found %d modules in repository %q", len(repoModules), repoURL)

		modules = append(modules, repoModules...)
	}

	if len(modules) == 0 {
		return errors.Errorf("no modules found")
	}

	return tui.Run(ctx, modules, opts)
}
