package catalog

import (
	"context"
	"errors"
	"io"
	"os"
	"syscall"

	"golang.org/x/sync/errgroup"

	"github.com/gruntwork-io/terragrunt/internal/cli/commands/catalog/format"
	"github.com/gruntwork-io/terragrunt/internal/cli/commands/catalog/tui"
	"github.com/gruntwork-io/terragrunt/pkg/log"
)

// componentChannelBufferSize absorbs short bursts from the concurrent loaders
// without blocking them on the renderer.
const componentChannelBufferSize = 10

// Stream renders every component the loader discovers to w as it arrives.
//
// Components are written in discovery order, which interleaves across sources
// and differs between runs. Sorting them would mean holding every component
// until the slowest source finished loading, which is the opposite of what
// this output is for.
//
// A consumer that stops reading, as in `terragrunt catalog --format=jsonl |
// head -1`, ends the stream: the loader is cancelled and Stream returns
// without an error, so the caller's cleanup still runs.
func Stream(
	ctx context.Context,
	l log.Logger,
	w io.Writer,
	renderer format.Renderer,
	load tui.LoadFunc,
) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	notifyBrokenPipe(ctx, cancel)

	componentCh := make(chan *tui.ComponentEntry, componentChannelBufferSize)

	g, gctx := errgroup.WithContext(ctx)

	g.Go(func() error {
		defer close(componentCh)

		// Progress updates go to the log, never to w: every line written to
		// w is a catalog component.
		return load(gctx, func(msg string) { l.Debugf("%s", msg) }, componentCh)
	})

	renderErr := render(w, renderer, componentCh)

	// A loader that is mid-send when rendering stops early parks on a channel
	// nobody reads, so release it before waiting on the group.
	cancel()

	loadErr := g.Wait()

	if renderErr != nil {
		return renderErr
	}

	return loadErr
}

// render drains componentCh into renderer until the loader closes it.
func render(w io.Writer, renderer format.Renderer, componentCh <-chan *tui.ComponentEntry) error {
	if err := renderer.Open(w); err != nil {
		return writeErr(err)
	}

	var (
		entries int
		sources = make(map[string]struct{})
	)

	for entry := range componentCh {
		if err := renderer.Entry(w, entry); err != nil {
			return writeErr(err)
		}

		entries++

		sources[entry.Source] = struct{}{}
	}

	summary := format.Summary{Entries: entries, Sources: len(sources)}

	return writeErr(renderer.Close(w, summary))
}

// writeErr reports which write failures the command should surface. A closed
// pipe means the consumer read everything it wanted, so the stream ends
// quietly instead of spraying write errors over the tail of the output.
func writeErr(err error) error {
	if errors.Is(err, syscall.EPIPE) || errors.Is(err, os.ErrClosed) {
		return nil
	}

	return err
}
