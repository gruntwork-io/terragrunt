// Package catalog provides the ability to interact with a catalog of OpenTofu/Terraform modules.
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

// Run runs the catalog command.
func Run(ctx context.Context, opts *options.TerragruntOptions, repoURL string) error {
	repoURLs := []string{repoURL}

	if repoURL == "" {
		config, err := config.ReadCatalogConfig(ctx, opts)
		if err != nil {
			return fmt.Errorf("error reading catalog config: %w", err)
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
			return fmt.Errorf("error creating repository: %w", err)
		}

		repoModules, err := repo.FindModules(ctx)
		if err != nil {
			return fmt.Errorf("error finding modules in repository %q: %w", repoURL, err)
		}

		log.Infof("Found %d modules in repository %q", len(repoModules), repoURL)

		modules = append(modules, repoModules...)
	}

	if len(modules) == 0 {
		return errors.Errorf("no modules found")
	}

	err := tui.Run(ctx, modules, opts)
	if err != nil {
		return fmt.Errorf("error running catalog: %w", err)
	}

	return nil
}
