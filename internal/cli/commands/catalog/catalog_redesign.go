package catalog

import (
	"context"
	"fmt"
	"runtime"
	"slices"
	"strings"
	"sync"

	"golang.org/x/sync/errgroup"

	"github.com/gruntwork-io/terragrunt/internal/cli/commands/catalog/tui/redesign"
	"github.com/gruntwork-io/terragrunt/internal/configbridge"
	"github.com/gruntwork-io/terragrunt/pkg/config"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/pkg/options"
)

// urlChannelBufferSize is the buffer size for the discovery URL channel. It
// absorbs short producer bursts from the two concurrent URL discoverers
// without blocking them on a slow consumer.
const urlChannelBufferSize = 10

// runRedesign is the entry point for the redesigned catalog experience.
// It is invoked when the catalog-redesign experiment is enabled.
//
// It launches the TUI immediately with a loading screen, then runs source
// discovery and component loading in the background. When loading completes,
// the TUI transitions to the component list or shows a welcome screen.
func runRedesign(ctx context.Context, l log.Logger, opts *options.TerragruntOptions, repoURL string) error {
	// Fail fast with a clear error when there is no terminal to attach the
	// TUI to, instead of surfacing bubbletea's raw TTY failure.
	if err := redesign.EnsureOSTTY(l); err != nil {
		return err
	}

	// If an explicit URL was passed via CLI, use the default path
	if repoURL != "" {
		return runDefault(ctx, l, opts, repoURL)
	}

	return redesign.RunRedesign(
		ctx, l, opts, opts.Writers.ErrWriter,
		func(
			ctx context.Context, status redesign.StatusFunc, componentCh chan<- *redesign.ComponentEntry,
		) error {
			urlCh := make(chan string, urlChannelBufferSize)

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

			// Per-source failures are collected rather than logged: log
			// writes during the alt-screen shred the TUI's rendering, and
			// swallowing them would leave the user staring at a misleading
			// "no sources" screen when every repository failed to load.
			var (
				failuresMu sync.Mutex
				failures   []redesign.SourceFailure
			)

			seen := make(map[string]struct{})

			for repoURL := range urlCh {
				if _, ok := seen[repoURL]; ok {
					continue
				}

				seen[repoURL] = struct{}{}

				loaders.Go(func() error {
					err := redesign.LoadURL(loadCtx, l, opts, repoURL, componentCh)
					if err == nil {
						return nil
					}

					// Suppress errors from context cancellation (user quit the TUI).
					if loadCtx.Err() != nil {
						return nil
					}

					failuresMu.Lock()
					defer failuresMu.Unlock()

					failures = append(failures, redesign.SourceFailure{URL: repoURL, Err: err})

					return nil
				})
			}

			if err := loaders.Wait(); err != nil {
				return fmt.Errorf("loading components: %w", err)
			}

			if err := g.Wait(); err != nil {
				return fmt.Errorf("discovering sources: %w", err)
			}

			if len(failures) > 0 {
				slices.SortFunc(failures, func(a, b redesign.SourceFailure) int {
					return strings.Compare(a.URL, b.URL)
				})

				return &redesign.SourceLoadError{Failures: failures, Attempted: len(seen)}
			}

			return nil
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
