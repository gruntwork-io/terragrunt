package browse

import (
	"context"
	"io"

	"github.com/gruntwork-io/terragrunt/internal/cli/commands/browse/tui"
	"github.com/gruntwork-io/terragrunt/internal/cli/commands/discoverysetup"
	"github.com/gruntwork-io/terragrunt/internal/component"
	"github.com/gruntwork-io/terragrunt/internal/discovery"
	"github.com/gruntwork-io/terragrunt/internal/os/stdout"
	"github.com/gruntwork-io/terragrunt/internal/telemetry"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	viewtui "github.com/gruntwork-io/terragrunt/internal/view/tui"
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
	warnCh := make(chan viewtui.Warning, viewtui.WarnChannelBuffer)
	done := make(chan struct{})

	// While the browser owns the alt screen, anything discovery writes to the
	// log stream would draw over it, so discovery gets a muted clone of the
	// logger and its warn-or-worse entries surface as toasts in the browser
	// instead.
	discoveryLogger := l.WithOptions(log.WithOutput(io.Discard), log.WithHooks(viewtui.NewWarnHook(warnCh)))

	var res tui.DiscoveryResult

	go func() {
		defer close(done)

		res = runDiscovery(discoverCtx, discoveryLogger, v, opts, d)
		resultCh <- res
	}()

	root := tui.NewRoot(opts.WorkingDir)

	err = tui.Run(ctx, l, vfs.NewOSFS(), root, stdout.ShouldColor(l), resultCh, warnCh)

	cancel()
	<-done
	close(warnCh)

	// Deferred until the browser has released the screen so the full error
	// lands on the log stream instead of drawing over the TUI.
	if res.Err != nil {
		l.Debugf("Errors encountered while discovering components:\n%s", res.Err)
	}

	return err
}

// runDiscovery runs the full discovery pass, returning the components for the
// browser to annotate its tree with. Errors ride back on the result alongside
// the partial components, matching the browser's best-effort display; the
// caller logs them once the browser has released the screen.
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

	components = components.Sort()

	return tui.DiscoveryResult{Components: components, Err: err}
}
