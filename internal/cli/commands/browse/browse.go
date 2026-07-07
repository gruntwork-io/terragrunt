package browse

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/gruntwork-io/terragrunt/internal/cli/commands/browse/tui"
	"github.com/gruntwork-io/terragrunt/internal/component"
	"github.com/gruntwork-io/terragrunt/internal/configbridge"
	"github.com/gruntwork-io/terragrunt/internal/discovery"
	"github.com/gruntwork-io/terragrunt/internal/os/stdout"
	"github.com/gruntwork-io/terragrunt/internal/stacks/generate"
	"github.com/gruntwork-io/terragrunt/internal/telemetry"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/gruntwork-io/terragrunt/internal/worktrees"
	"github.com/gruntwork-io/terragrunt/pkg/config"
	"github.com/gruntwork-io/terragrunt/pkg/log"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

// Run runs the browse command. It opens the browser immediately over a tree the
// TUI fills from the filesystem, and runs discovery in the background so unit and
// stack metadata and counts stream in without blocking the initial render.
func Run(ctx context.Context, l log.Logger, opts *Options) error {
	d, err := discovery.NewForDiscoveryCommand(l, &discovery.DiscoveryCommandOptions{
		WorkingDir:        opts.WorkingDir,
		WithRequiresParse: true,
		WithRelationships: true,
		Filters:           opts.Filters,
	})
	if err != nil {
		return err
	}

	// We do worktree generation here instead of in the discovery constructor
	// so that we can defer cleanup in the same context.
	gitFilters := opts.Filters.UniqueGitFilters()

	wts, worktreeErr := worktrees.NewWorktrees(ctx, l, worktrees.WorktreeOpts{
		WorkingDir:     opts.WorkingDir,
		GitExpressions: gitFilters,
		Experiments:    opts.Experiments,
	})
	if worktreeErr != nil {
		return fmt.Errorf("failed to create worktrees: %w", worktreeErr)
	}

	defer func() {
		cleanupErr := wts.Cleanup(ctx, l)
		if cleanupErr != nil {
			l.Errorf("failed to cleanup worktrees: %v", cleanupErr)
		}
	}()

	if err := generate.WorktreeStacks(ctx, l, opts.TerragruntOptions, wts); err != nil {
		return err
	}

	d = d.WithWorktrees(wts)

	// Discover in the background. A cancellable context lets the browser quitting
	// abort an in-flight discovery, and done lets us wait for it to unwind before
	// the deferred worktree cleanup removes trees discovery may still be reading.
	discoverCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	resultCh := make(chan tui.DiscoveryResult, 1)
	done := make(chan struct{})

	go func() {
		defer close(done)

		resultCh <- runDiscovery(discoverCtx, l, opts, d)
	}()

	root := tui.NewRoot(opts.WorkingDir)

	err = tui.Run(ctx, vfs.NewOSFS(), root, shouldColor(l), resultCh)

	cancel()
	<-done

	return err
}

// runDiscovery runs the full discovery pass and parses stack configs, returning
// the components for the browser to annotate its tree with. Errors are logged
// and the partial results returned, matching the browser's best-effort display.
func runDiscovery(ctx context.Context, l log.Logger, opts *Options, d *discovery.Discovery) tui.DiscoveryResult {
	var (
		components  component.Components
		discoverErr error
	)

	err := telemetry.TelemeterFromContext(ctx).Collect(ctx, "browse_discover", map[string]any{
		"working_dir": opts.WorkingDir,
	}, func(ctx context.Context) error {
		components, discoverErr = d.Discover(ctx, l, opts.TerragruntOptions)

		if span := trace.SpanFromContext(ctx); span.IsRecording() {
			span.SetAttributes(attribute.Int("component_count", len(components)))
		}

		return discoverErr
	})
	if err != nil {
		l.Debugf("Errors encountered while discovering components:\n%s", err)
	}

	components = components.Sort()

	parseStackConfigs(ctx, l, opts, components)

	return tui.DiscoveryResult{Components: components, Err: err}
}

// parseStackConfigs parses each discovered stack's terragrunt.stack.hcl so the
// TUI can show the units and stacks it defines. Discovery doesn't parse stack
// files, so we do it here, best-effort: a stack that fails to parse is left
// without config and simply omits those details in the preview.
func parseStackConfigs(ctx context.Context, l log.Logger, opts *Options, components component.Components) {
	var pctx *config.ParsingContext

	for _, c := range components {
		stack, ok := c.(*component.Stack)
		if !ok {
			continue
		}

		if pctx == nil {
			_, pctx = configbridge.NewParsingContext(ctx, l, opts.TerragruntOptions)
		}

		stackFile := filepath.Join(stack.Path(), stack.ConfigFile())

		cfg, err := config.ReadStackConfigFile(ctx, l, pctx, stackFile, nil)
		if err != nil {
			l.Debugf("Skipping stack config %s for browse preview: %v", stackFile, err)

			continue
		}

		stack.StoreConfig(cfg)
	}
}

// shouldColor returns true if the output should be colored.
func shouldColor(l log.Logger) bool {
	return !l.Formatter().DisabledColors() && !stdout.IsRedirected()
}
