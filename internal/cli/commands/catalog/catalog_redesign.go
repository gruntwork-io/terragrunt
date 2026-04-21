package catalog

import (
	"context"
	"fmt"
	"runtime"

	"golang.org/x/sync/errgroup"

	"github.com/gruntwork-io/terragrunt/internal/cli/commands/catalog/tui/redesign"
	"github.com/gruntwork-io/terragrunt/internal/configbridge"
	"github.com/gruntwork-io/terragrunt/internal/services/catalog"
	"github.com/gruntwork-io/terragrunt/internal/services/catalog/module"
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

	return redesign.RunRedesign(
		ctx, l, opts,
		func(
			ctx context.Context, status redesign.StatusFunc, moduleCh chan<- *redesign.ModuleEntry,
		) (catalog.CatalogService, error) {
			svc := catalog.NewCatalogService(opts)

			urlCh := make(chan string, 10) //nolint:mnd

			g, gctx := errgroup.WithContext(ctx)

			g.Go(func() error {
				return discoverCatalogConfigURLs(gctx, l, opts, urlCh)
			})

			g.Go(func() error {
				return discoverSourceFileURLs(gctx, l, opts, urlCh)
			})

			go func() {
				_ = g.Wait()

				close(urlCh)
			}()

			status("Discovering catalog sources...")

			maxWorkers := max(1, min(opts.Parallelism, runtime.GOMAXPROCS(0)))

			// Derive from ctx (not gctx) so loaders survive discovery-group
			// cancellation. gctx is cancelled automatically when g.Wait returns.
			loaders, loadCtx := errgroup.WithContext(ctx)
			loaders.SetLimit(maxWorkers)

			seen := make(map[string]struct{})

			for repoURL := range urlCh {
				if _, ok := seen[repoURL]; ok {
					continue
				}

				seen[repoURL] = struct{}{}

				loaders.Go(func() error {
					var collected []*module.Module

					onModule := func(mod *module.Module) {
						collected = append(collected, mod)
					}

					if err := svc.LoadStreamingURL(loadCtx, l, repoURL, onModule); err != nil {
						// Suppress errors from context cancellation (user quit the TUI).
						if loadCtx.Err() == nil {
							l.Warnf("Error loading %s: %v", repoURL, err)
						}

						return nil
					}

					// All modules from the same repo share a *Repo. Resolve the
					// latest tag once using the cloned repo on disk.
					if len(collected) > 0 {
						collected[0].Repo.ResolveLatestTag(loadCtx)
					}

					for _, mod := range collected {
						source := redesign.ExtractRepoURL(mod.TerraformSourcePath())
						entry := redesign.NewModuleEntry(mod).WithSource(source)

						if mod.Repo.LatestTag != "" {
							entry = entry.WithVersion(mod.Repo.LatestTag)
						}

						select {
						case moduleCh <- entry:
						case <-loadCtx.Done():
							return nil
						}
					}

					return nil
				})
			}

			if err := loaders.Wait(); err != nil {
				return nil, fmt.Errorf("loading modules: %w", err)
			}

			if err := g.Wait(); err != nil {
				return nil, fmt.Errorf("discovering sources: %w", err)
			}

			if len(svc.Modules()) == 0 {
				return nil, nil
			}

			return svc, nil
		})
}

// discoverCatalogConfigURLs reads catalog URLs from the root config and
// sends each to urlCh.
func discoverCatalogConfigURLs(ctx context.Context, l log.Logger, opts *options.TerragruntOptions, urlCh chan<- string) error {
	_, pctx := configbridge.NewParsingContext(ctx, l, opts)

	catalogCfg, err := config.ReadCatalogConfig(ctx, l, pctx)
	if err != nil {
		l.Debugf("No catalog config found: %v", err)
		return nil
	}

	if catalogCfg == nil {
		return nil
	}

	for _, u := range catalogCfg.URLs {
		urlCh <- u
	}

	return nil
}

// discoverSourceFileURLs walks terragrunt.hcl files, extracts
// terraform.source URLs, and sends each repo URL to urlCh.
func discoverSourceFileURLs(ctx context.Context, l log.Logger, opts *options.TerragruntOptions, urlCh chan<- string) error {
	ctx, pctx := configbridge.NewParsingContext(ctx, l, opts)

	urls, err := redesign.DiscoverSourceURLs(ctx, l, pctx)
	if err != nil {
		l.Warnf("Failed to discover source URLs: %v", err)
		return nil
	}

	for _, u := range urls {
		urlCh <- u
	}

	return nil
}
