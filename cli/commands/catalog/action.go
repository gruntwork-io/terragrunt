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
	"golang.org/x/sync/errgroup"
)

const (
	tempDirFormat = "catalog%x"
)

func Run(ctx context.Context, opts *options.TerragruntOptions, repoURL string) error {
	log.SetLogger(opts.Logger.Logger)

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

	// Create one goroutine to listen on a channel, receive parsed module data, and add it to the modules slice
	modulesChan := make(chan module.Modules)
	var modules module.Modules
	go func() {
		for repoModules := range modulesChan {
			modules = append(modules, repoModules...)
		}
	}()

	// Create a bunch more goroutines that concurrently 'git clone' repos, parse module data from them, and send that
	// data to the channel created above
	errGroup := new(errgroup.Group)
	for _, repoURL := range repoURLs {
		// Copy the value so each goroutine gets the right one: https://golang.org/doc/faq#closures_and_goroutines
		repoURL := repoURL
		errGroup.Go(func() error {
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

			modulesChan <- repoModules
			return nil
		})
	}

	if err := errGroup.Wait(); err != nil {
		return err
	}

	if len(modules) == 0 {
		return errors.Errorf("no modules found")
	}

	return tui.Run(ctx, modules, opts)
}
