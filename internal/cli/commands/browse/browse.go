package browse

import (
	"context"

	"github.com/gruntwork-io/terragrunt/internal/cli/commands/browse/tui"
	"github.com/gruntwork-io/terragrunt/internal/cli/commands/discoverysetup"
	"github.com/gruntwork-io/terragrunt/internal/component"
	"github.com/gruntwork-io/terragrunt/internal/discovery"
	"github.com/gruntwork-io/terragrunt/internal/os/stdout"
	"github.com/gruntwork-io/terragrunt/internal/telemetry"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/gruntwork-io/terragrunt/pkg/log"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

// Run runs the browse command. It opens the browser immediately over a tree the
// TUI fills from the filesystem, and runs discovery in the background so unit and
// stack metadata and counts stream in without blocking the initial render.
func Run(ctx context.Context, l log.Logger, v venv.Venv, opts *Options) error {
	d, err := discovery.NewForDiscoveryCommand(l, &discovery.DiscoveryCommandOptions{
		WorkingDir:        opts.WorkingDir,
		WithRequiresParse: true,
		WithRelationships: true,
		ParseStackConfigs: true,
		Filters:           opts.Filters,
	})
	if err != nil {
		return err
	}

	d, cleanupWorktrees, err := discoverysetup.Worktrees(ctx, l, v, opts.TerragruntOptions, d)

	defer cleanupWorktrees(ctx)

	if err != nil {
		return err
	}

	// Discover in the background. A cancellable context lets the browser quitting
	// abort an in-flight discovery, and done lets us wait for it to unwind before
	// the deferred worktree cleanup removes trees discovery may still be reading.
	discoverCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	resultCh := make(chan tui.DiscoveryResult, 1)
	done := make(chan struct{})

	go func() {
		defer close(done)

		resultCh <- runDiscovery(discoverCtx, l, v, opts, d)
	}()

	root := tui.NewRoot(opts.WorkingDir)

	err = tui.Run(ctx, vfs.NewOSFS(), root, stdout.ShouldColor(l), resultCh)

	cancel()
	<-done

	return err
}

// runDiscovery runs the full discovery pass, returning the components for the
// browser to annotate its tree with. Errors are logged and the partial results
// returned, matching the browser's best-effort display.
func runDiscovery(
	ctx context.Context,
	l log.Logger,
	v venv.Venv,
	opts *Options,
	d *discovery.Discovery,
) tui.DiscoveryResult {
	var (
		components  component.Components
		discoverErr error
	)

	err := telemetry.TelemeterFromContext(ctx).Collect(ctx, l, "browse_discover", map[string]any{
		"working_dir": opts.WorkingDir,
	}, func(ctx context.Context, l log.Logger) error {
		components, discoverErr = d.Discover(ctx, l, v, opts.TerragruntOptions)

		if span := trace.SpanFromContext(ctx); span.IsRecording() {
			span.SetAttributes(attribute.Int("component_count", len(components)))
		}

		return discoverErr
	})
	if err != nil {
		l.Debugf("Errors encountered while discovering components:\n%s", err)
	}

	components = components.Sort()

	return tui.DiscoveryResult{Components: components, Err: err}
}
