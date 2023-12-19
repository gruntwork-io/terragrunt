package catalog

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/gruntwork-io/go-commons/errors"
	"github.com/gruntwork-io/go-commons/files"
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
	log.SetLogger(opts.Logger.Logger)

	repoURLs := []string{repoURL}

	if repoURLs[0] == "" {
		config, err := readTerragruntConfig(opts)
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

func readTerragruntConfig(opts *options.TerragruntOptions) (*config.TerragruntConfig, error) {
	if err := updateConfigPath(opts); err != nil {
		return nil, err
	}

	config, err := config.ReadTerragruntConfig(opts)
	if err != nil {
		return nil, err
	}

	return config, nil
}

// if the config file does not exist in the path specified in `opts.TerragruntConfigPath`,
// tries to search for this file in the parent directories and updates `opts.TerragruntConfigPath` with the found path.
func updateConfigPath(opts *options.TerragruntOptions) error {
	if files.FileExists(opts.TerragruntConfigPath) {
		return nil
	}

	if configPath, err := config.FindInParentFolders([]string{opts.TerragruntConfigPath}, nil, opts); err != nil {
		return err
	} else if configPath != "" {
		opts.TerragruntConfigPath = configPath
	}

	return nil
}
