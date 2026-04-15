package catalog

import (
	"context"
	"fmt"

	"github.com/gruntwork-io/terragrunt/internal/cli/commands/catalog/tui"
	"github.com/gruntwork-io/terragrunt/internal/configbridge"
	"github.com/gruntwork-io/terragrunt/internal/services/catalog"
	"github.com/gruntwork-io/terragrunt/internal/util"
	"github.com/gruntwork-io/terragrunt/pkg/config"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/pkg/options"
)

// runRedesign is the entry point for the redesigned catalog experience.
// It is invoked when the catalog-redesign experiment is enabled.
//
// It launches the TUI immediately with a loading screen, then runs source
// discovery and module loading in the background. When loading completes,
// the TUI transitions to the module list or shows a welcome screen.
func runRedesign(ctx context.Context, l log.Logger, opts *options.TerragruntOptions, repoURL string) error {
	// If an explicit URL was passed via CLI, use the default path
	if repoURL != "" {
		return runDefault(ctx, l, opts, repoURL)
	}

	return tui.RunRedesign(ctx, l, opts, func(ctx context.Context, status tui.StatusFunc) (catalog.CatalogService, error) {
		status("Scanning terragrunt.hcl files for module sources...")

		// Create parsing context for source discovery and catalog config
		ctx, pctx := configbridge.NewParsingContext(ctx, l, opts)

		// Discover source URLs from terraform.source in terragrunt.hcl files
		discoveredURLs, err := DiscoverSourceURLs(ctx, l, pctx)
		if err != nil {
			l.Warnf("Failed to discover source URLs: %v", err)
		}

		// Also read catalog config if it exists
		catalogCfg, catalogErr := config.ReadCatalogConfig(ctx, l, pctx)
		if catalogErr != nil {
			l.Debugf("No catalog config found: %v", catalogErr)
		}

		// Merge: catalog URLs first, then discovered URLs
		var allURLs []string
		if catalogCfg != nil {
			allURLs = append(allURLs, catalogCfg.URLs...)
		}

		allURLs = append(allURLs, discoveredURLs...)
		allURLs = util.RemoveDuplicates(allURLs)

		if len(allURLs) == 0 {
			return nil, nil
		}

		status(fmt.Sprintf("Found %d source(s), cloning repositories...", len(allURLs)))

		// Load modules from all discovered repos
		svc := catalog.NewCatalogService(opts)
		svc.WithRepoURLs(allURLs)

		if err := svc.Load(ctx, l); err != nil {
			return svc, err
		}

		status(fmt.Sprintf("Found %d module(s), loading catalog...", len(svc.Modules())))

		return svc, nil
	})
}
