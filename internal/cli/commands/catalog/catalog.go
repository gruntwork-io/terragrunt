package catalog

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"io"
	"runtime"
	"slices"
	"strings"
	"sync"

	"golang.org/x/sync/errgroup"

	"github.com/gruntwork-io/terragrunt/internal/cli/commands/catalog/format"
	"github.com/gruntwork-io/terragrunt/internal/cli/commands/catalog/tui"
	"github.com/gruntwork-io/terragrunt/internal/cli/commands/login"
	"github.com/gruntwork-io/terragrunt/internal/configbridge"
	"github.com/gruntwork-io/terragrunt/internal/experiment"
	"github.com/gruntwork-io/terragrunt/internal/portal"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	viewtui "github.com/gruntwork-io/terragrunt/internal/view/tui"
	"github.com/gruntwork-io/terragrunt/pkg/config"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/pkg/options"
)

// urlChannelBufferSize is the buffer size for the discovery URL channel. It
// absorbs short producer bursts from the concurrent URL discoverers without
// blocking them on a slow consumer.
const urlChannelBufferSize = 10

// Run is the main entry point for the catalog command.
//
// When an explicit repo URL is given, only that URL is loaded; otherwise
// source discovery walks the configuration to find catalog and source URLs,
// and asks the portal for the repositories the user's organizations selected
// there. The components that turn up are either browsed in the TUI or written
// to standard output, depending on the requested format.
func Run(
	ctx context.Context,
	l log.Logger,
	v *venv.Venv,
	opts *Options,
	repoURL string,
) error {
	if opts.Format == FormatTUI {
		return runTUI(ctx, l, v, opts, repoURL)
	}

	renderer, err := format.NewRenderer(opts.Format)
	if err != nil {
		return err
	}

	tempDirs := tui.NewTempDirTracker(v.FS)

	streamCtx, stopStream := context.WithCancel(ctx)
	defer stopStream()

	stopNotify := notifyBrokenPipe(ctx, stopStream)
	defer stopNotify()

	defer tempDirs.Cleanup(l)

	return withSourceGuidance(Stream(
		streamCtx, l, v.Writers.Writer, renderer,
		newLoadFunc(l, v, opts, tempDirs, repoURL),
	))
}

// withSourceGuidance names the repositories a non-interactive run could not
// load. The plain formats put nothing on standard output, so this error is the
// whole report.
func withSourceGuidance(err error) error {
	var loadErr *tui.SourceLoadError
	if !errors.As(err, &loadErr) {
		return err
	}

	lines := make([]string, 0, len(loadErr.Failures)+1)

	for _, failure := range loadErr.Failures {
		lines = append(lines, "  "+failure.URL+": "+failure.Err.Error())
	}

	lines = append(lines, tui.SourceAccessHint)

	return fmt.Errorf("%w:\n%s", err, strings.Join(lines, "\n"))
}

// runTUI launches the TUI immediately with a loading screen, then loads
// components in the background. As components are found, the TUI transitions
// to the component list, or shows a welcome screen when nothing is discovered.
func runTUI(
	ctx context.Context,
	l log.Logger,
	v *venv.Venv,
	opts *Options,
	repoURL string,
) error {
	// Fail fast with a clear error when there is no terminal to attach the
	// TUI to, instead of surfacing bubbletea's raw TTY failure.
	if err := viewtui.EnsureOSTTY(); err != nil {
		return err
	}

	tempDirs := tui.NewTempDirTracker(v.FS)
	defer tempDirs.Cleanup(l)

	// While the TUI owns the alt screen, anything the background loaders write
	// to the log stream would draw over it, so they get a muted clone of the
	// logger and their warn-or-worse entries surface as toasts in the TUI. The
	// original logger stays with the TUI itself for work that runs while the
	// terminal is released (scaffolding) and for post-exit messages.
	warnCh := make(chan viewtui.Warning, viewtui.WarnChannelBuffer)
	loadLogger := l.WithOptions(log.WithOutput(io.Discard), log.WithHooks(viewtui.NewWarnHook(warnCh)))

	return tui.Run(
		ctx, l, v, opts.TerragruntOptions, warnCh,
		newLoadFunc(loadLogger, v, opts, tempDirs, repoURL),
	)
}

// newLoadFunc returns the loader that every output format drives.
func newLoadFunc(
	l log.Logger,
	v *venv.Venv,
	opts *Options,
	tempDirs *tui.TempDirTracker,
	repoURL string,
) tui.LoadFunc {
	return func(
		ctx context.Context, status tui.StatusFunc, componentCh chan<- *tui.ComponentEntry,
	) error {
		if repoURL != "" {
			status("Loading " + repoURL + "...")

			return tui.LoadURL(ctx, l, v, opts.TerragruntOptions, tempDirs, repoURL, componentCh)
		}

		return discoverAndLoad(ctx, l, v, opts, tempDirs, status, componentCh)
	}
}

// discoverAndLoad runs the concurrent URL discoverers and loads each distinct
// repo URL they surface into componentCh, bounded by parallelism.
func discoverAndLoad(
	ctx context.Context, l log.Logger, v *venv.Venv, opts *Options,
	tempDirs *tui.TempDirTracker,
	status tui.StatusFunc, componentCh chan<- *tui.ComponentEntry,
) error {
	urlCh := make(chan string, urlChannelBufferSize)

	g, gctx := errgroup.WithContext(ctx)

	g.Go(func() error {
		return discoverCatalogConfigURLs(gctx, l, v, opts.TerragruntOptions, urlCh)
	})

	g.Go(func() error {
		return discoverSourceFileURLs(gctx, l, v, opts.TerragruntOptions, urlCh)
	})

	g.Go(func() error {
		return discoverPortalURLs(gctx, l, v, opts, urlCh)
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

	// Per-source failures are collected rather than logged: log writes during
	// the alt-screen shred the TUI's rendering, and swallowing them would
	// leave the user staring at a misleading "no sources" screen when every
	// repository failed to load.
	var (
		failuresMu sync.Mutex
		failures   []tui.SourceFailure
	)

	seen := make(map[string]struct{})

	for repoURL := range urlCh {
		key := dedupKey(repoURL)
		if _, ok := seen[key]; ok {
			continue
		}

		seen[key] = struct{}{}

		loaders.Go(func() error {
			err := tui.LoadURL(loadCtx, l, v, opts.TerragruntOptions, tempDirs, repoURL, componentCh)
			if err == nil {
				return nil
			}

			// Suppress errors from context cancellation (user quit the TUI).
			if loadCtx.Err() != nil {
				return nil
			}

			failuresMu.Lock()
			defer failuresMu.Unlock()

			failures = append(failures, tui.SourceFailure{URL: repoURL, Err: err})

			return nil
		})
	}

	if err := loaders.Wait(); err != nil {
		return fmt.Errorf("loading components: %w", err)
	}

	if len(failures) > 0 {
		slices.SortFunc(failures, func(a, b tui.SourceFailure) int {
			return strings.Compare(a.URL, b.URL)
		})

		return &tui.SourceLoadError{Failures: failures, Attempted: len(seen)}
	}

	if err := g.Wait(); err != nil {
		return fmt.Errorf("discovering sources: %w", err)
	}

	return nil
}

// dedupKey is what a discovered repository is deduplicated on. Each discoverer
// names repositories in whatever spelling its own source holds, so the portal
// and a `catalog` block can reach one repository by two strings, which compared
// as they arrived would clone it once per spelling and list every component in
// it that many times.
func dedupKey(repoURL string) string {
	key := repoURL

	// The getter infers https for a URL that names no scheme, so the shorthand
	// the `catalog` block documents addresses what an absolute URL does.
	for _, scheme := range []string{"https://", "http://"} {
		if rest, found := strings.CutPrefix(key, scheme); found {
			key = rest

			break
		}
	}

	host, path, found := strings.Cut(key, "/")

	key = strings.ToLower(host)
	if found {
		key += "/" + path
	}

	return strings.TrimSuffix(strings.TrimSuffix(key, "/"), ".git")
}

// discoverCatalogConfigURLs reads catalog URLs from the root config and
// sends each to urlCh.
func discoverCatalogConfigURLs(
	ctx context.Context,
	l log.Logger,
	v *venv.Venv,
	opts *options.TerragruntOptions,
	urlCh chan<- string,
) error {
	_, pctx := configbridge.NewParsingContext(ctx, l, v, opts)

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
func discoverSourceFileURLs(
	ctx context.Context,
	l log.Logger,
	v *venv.Venv,
	opts *options.TerragruntOptions,
	urlCh chan<- string,
) error {
	ctx, pctx := configbridge.NewParsingContext(ctx, l, v, opts)

	urls, err := tui.DiscoverSourceURLs(ctx, l, pctx)
	if err != nil {
		l.Warnf("Failed to discover source URLs: %v", err)
		return nil
	}

	for _, u := range urls {
		urlCh <- u
	}

	return nil
}

// discoverPortalURLs sends the repositories the user's organizations selected
// in the Gruntwork Developer Portal to urlCh.
func discoverPortalURLs(
	ctx context.Context,
	l log.Logger,
	v *venv.Venv,
	opts *Options,
	urlCh chan<- string,
) error {
	// The experiment gates the whole portal feature, not only the login that
	// files the credential. Nothing else takes a stored credential out of use,
	// so switching the experiment off is all a signed-in user has to stop this
	// request with.
	if !opts.Experiments.Evaluate(experiment.TGLogin) {
		return nil
	}

	v.RequireHTTP()

	credentials, err := portal.LoadCredentials(l, v, opts.PortalBaseURL)
	if err != nil {
		l.Debugf("No portal credential could be read: %v", err)

		return nil
	}

	loginCmd := login.Command(opts.Experiments)

	for _, org := range credentials.Expired {
		l.Warnf(
			"The portal credential for %s has expired, so the repositories that"+
				" organization selected in the Gruntwork Developer Portal are not in"+
				" this catalog. Run `%s` to sign in again",
			orgName(org), loginCmd,
		)
	}

	// Organizations are asked in parallel. Each fetch bounds its own wait on the
	// portal, so asking in turn would let a portal that accepts the connection
	// and then says nothing hold the run for that wait once per organization.
	var wg sync.WaitGroup

	for _, token := range credentials.Valid {
		wg.Go(func() {
			repositories, err := portal.FetchCatalog(ctx, l, v.HTTP, opts.PortalBaseURL, token.AccessToken)
			if err != nil {
				reportPortalFailure(ctx, l, token.Org, err, loginCmd)

				return
			}

			for _, repository := range repositories {
				urlCh <- repository.URL
			}
		})
	}

	wg.Wait()

	return nil
}

// reportPortalFailure decides how loudly a failed fetch is reported. Only a
// failure that kept back a catalog the organization actually has warrants a
// warning: the components it would have contributed are missing from the list
// the user is looking at.
func reportPortalFailure(ctx context.Context, l log.Logger, org portal.Org, err error, loginCmd string) {
	name := orgName(org)

	if ctx.Err() != nil {
		l.Debugf("Abandoned the portal catalog for %s: %v", name, err)

		return
	}

	if errors.Is(err, portal.ErrNoHostedCatalog) {
		l.Debugf("The portal serves no catalog for %s: %v", name, err)

		return
	}

	if errors.Is(err, portal.ErrCredentialRejected) {
		l.Warnf(
			"The portal rejected the credential for %s, so the repositories that"+
				" organization selected in the Gruntwork Developer Portal are not in"+
				" this catalog. The credential has not expired, so run `%s --%s` to"+
				" replace it",
			name, loginCmd, login.ForceFlagName,
		)

		return
	}

	l.Warnf("Could not read the portal catalog for %s: %v", name, err)
}

// orgName is what an organization is called in output: the name the portal
// sent when it sent one, and the id it is filed under otherwise.
func orgName(org portal.Org) string {
	return cmp.Or(org.Name, org.ID)
}
