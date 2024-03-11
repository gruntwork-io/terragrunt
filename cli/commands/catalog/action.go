package catalog

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/gruntwork-io/go-commons/errors"
	"github.com/gruntwork-io/terragrunt/cli/commands/catalog/module"
	"github.com/gruntwork-io/terragrunt/cli/commands/catalog/tui"
	"github.com/gruntwork-io/terragrunt/config"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/pkg/log"
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

	for _, repoURL := range repoURLs {
		tempDir := filepath.Join(os.TempDir(), fmt.Sprintf(tempDirFormat, util.EncodeBase64Sha1(repoURL)))

		repo, err := module.NewRepo(ctx, repoURL, tempDir)
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
