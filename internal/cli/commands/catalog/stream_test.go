package catalog_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"slices"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/sync/errgroup"

	"github.com/gruntwork-io/terragrunt/internal/cli/commands/catalog"
	"github.com/gruntwork-io/terragrunt/internal/cli/commands/catalog/format"
	"github.com/gruntwork-io/terragrunt/internal/cli/commands/catalog/tui"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
)

func TestStreamWritesEveryDiscoveredComponent(t *testing.T) {
	t.Parallel()

	var buf strings.Builder

	load := func(_ context.Context, status tui.StatusFunc, componentCh chan<- *tui.ComponentEntry) error {
		status("Discovering catalog sources...")

		componentCh <- testEntry("modules/vpc")

		componentCh <- testEntry("modules/ecs")

		return nil
	}

	require.NoError(t, catalog.Stream(
		t.Context(), logger.CreateLogger(), &buf, format.NewJSONLRenderer(), load,
	))

	assert.Equal(t, []string{"modules/ecs", "modules/vpc"}, sortedDirs(t, buf.String()))
}

func TestStreamOnEmptyDiscovery(t *testing.T) {
	t.Parallel()

	var buf strings.Builder

	load := func(_ context.Context, _ tui.StatusFunc, _ chan<- *tui.ComponentEntry) error {
		return nil
	}

	require.NoError(t, catalog.Stream(
		t.Context(), logger.CreateLogger(), &buf, format.NewJSONLRenderer(), load,
	))

	assert.Empty(t, buf.String())
}

// TestStreamWritesEntriesAsTheyArrive pins progressive output without timing:
// the loader cannot send its second component until the test has observed the
// first one reach the writer.
func TestStreamWritesEntriesAsTheyArrive(t *testing.T) {
	t.Parallel()

	w := &blockingWriter{writes: make(chan string)}
	releaseSecond := make(chan struct{})

	load := func(_ context.Context, _ tui.StatusFunc, componentCh chan<- *tui.ComponentEntry) error {
		componentCh <- testEntry("modules/first")

		<-releaseSecond

		componentCh <- testEntry("modules/second")

		return nil
	}

	streamErr := make(chan error, 1)

	go func() {
		streamErr <- catalog.Stream(
			t.Context(), logger.CreateLogger(), w, format.NewJSONLRenderer(), load,
		)
	}()

	first := <-w.writes

	close(releaseSecond)

	second := <-w.writes

	require.NoError(t, <-streamErr)
	assert.Contains(t, first, "modules/first")
	assert.Contains(t, second, "modules/second")
}

// TestStreamOnBrokenPipe covers `terragrunt catalog --format=jsonl | head -1`:
// the reader is gone, so the command stops loading and exits quietly.
func TestStreamOnBrokenPipe(t *testing.T) {
	t.Parallel()

	loaderCancelled := make(chan struct{})

	load := func(ctx context.Context, _ tui.StatusFunc, componentCh chan<- *tui.ComponentEntry) error {
		componentCh <- testEntry("modules/vpc")

		<-ctx.Done()

		close(loaderCancelled)

		return nil
	}

	err := catalog.Stream(
		t.Context(), logger.CreateLogger(), errWriter{err: syscall.EPIPE},
		format.NewJSONLRenderer(), load,
	)
	require.NoError(t, err)

	select {
	case <-loaderCancelled:
	default:
		t.Fatal("the loader kept running after the consumer went away")
	}
}

func TestStreamOnClosedWriter(t *testing.T) {
	t.Parallel()

	load := func(_ context.Context, _ tui.StatusFunc, componentCh chan<- *tui.ComponentEntry) error {
		componentCh <- testEntry("modules/vpc")

		return nil
	}

	require.NoError(t, catalog.Stream(
		t.Context(), logger.CreateLogger(), errWriter{err: os.ErrClosed},
		format.NewJSONLRenderer(), load,
	))
}

func TestStreamReportsWriteFailures(t *testing.T) {
	t.Parallel()

	wantErr := errors.New("disk is full")

	load := func(_ context.Context, _ tui.StatusFunc, componentCh chan<- *tui.ComponentEntry) error {
		componentCh <- testEntry("modules/vpc")

		return nil
	}

	err := catalog.Stream(
		t.Context(), logger.CreateLogger(), errWriter{err: wantErr},
		format.NewJSONLRenderer(), load,
	)
	require.ErrorIs(t, err, wantErr)
}

// TestStreamReportsSourceFailures keeps the exit-code behavior of a source
// that could not be loaded, which callers already depend on.
func TestStreamReportsSourceFailures(t *testing.T) {
	t.Parallel()

	var buf strings.Builder

	loadErr := &tui.SourceLoadError{
		Failures:  []tui.SourceFailure{{URL: "github.com/acme/repo", Err: errors.New("clone failed")}},
		Attempted: 1,
	}

	load := func(_ context.Context, _ tui.StatusFunc, componentCh chan<- *tui.ComponentEntry) error {
		componentCh <- testEntry("modules/vpc")

		return loadErr
	}

	err := catalog.Stream(
		t.Context(), logger.CreateLogger(), &buf, format.NewJSONLRenderer(), load,
	)

	var sourceErr *tui.SourceLoadError

	require.ErrorAs(t, err, &sourceErr)
	assert.True(t, sourceErr.AllFailed())
	assert.Equal(t, []string{"modules/vpc"}, sortedDirs(t, buf.String()))
}

// TestStreamConcurrentProducersWithRacing loads from several sources at once,
// the way discovery does, and checks that no two components share a line.
func TestStreamConcurrentProducersWithRacing(t *testing.T) {
	t.Parallel()

	const (
		producers             = 8
		componentsPerProducer = 25
	)

	w := &lockedWriter{}

	load := func(ctx context.Context, _ tui.StatusFunc, componentCh chan<- *tui.ComponentEntry) error {
		g, _ := errgroup.WithContext(ctx)

		for producer := range producers {
			g.Go(func() error {
				for component := range componentsPerProducer {
					componentCh <- testEntry(
						"modules/" + strconv.Itoa(producer) + "/" + strconv.Itoa(component),
					)
				}

				return nil
			})
		}

		return g.Wait()
	}

	require.NoError(t, catalog.Stream(
		t.Context(), logger.CreateLogger(), w, format.NewJSONLRenderer(), load,
	))

	assert.Len(t, sortedDirs(t, w.String()), producers*componentsPerProducer)
}

func testEntry(dir string) *tui.ComponentEntry {
	component := tui.NewComponentForTest(
		tui.ComponentKindModule, "github.com/acme/repo", dir, "",
	)

	return tui.NewComponentEntry(component).WithSource("github.com/acme/repo")
}

// sortedDirs parses every rendered line and returns the directories they
// describe, sorted: components arrive in discovery order, which no test may
// depend on.
func sortedDirs(t *testing.T, out string) []string {
	t.Helper()

	trimmed := strings.TrimSuffix(out, "\n")
	if trimmed == "" {
		return nil
	}

	lines := strings.Split(trimmed, "\n")
	dirs := make([]string, 0, len(lines))

	for _, line := range lines {
		var entry format.Entry

		require.NoError(t, json.Unmarshal([]byte(line), &entry))

		dirs = append(dirs, entry.Dir)
	}

	slices.Sort(dirs)

	return dirs
}

// blockingWriter hands each write to a reader of writes and blocks until it
// is taken, so a test can observe output without waiting on the clock.
type blockingWriter struct {
	writes chan string
}

func (w *blockingWriter) Write(p []byte) (int, error) {
	w.writes <- string(p)

	return len(p), nil
}

// errWriter fails every write with a fixed error.
type errWriter struct {
	err error
}

func (w errWriter) Write(_ []byte) (int, error) {
	return 0, w.err
}

// lockedWriter collects output from concurrent writers.
type lockedWriter struct {
	buf strings.Builder
	mu  sync.Mutex
}

func (w *lockedWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	return w.buf.Write(p)
}

func (w *lockedWriter) String() string {
	w.mu.Lock()
	defer w.mu.Unlock()

	return w.buf.String()
}
