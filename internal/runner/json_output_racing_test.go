package runner_test

import (
	"io"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/runner"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/sync/errgroup"
)

// TestWriteJSONOutputSamePathConcurrentWithRacing pins that the published plan is
// exactly one writer's output when several write to the same path at once, which is
// what two commands sharing a --json-out-dir do.
func TestWriteJSONOutputSamePathConcurrentWithRacing(t *testing.T) {
	t.Parallel()

	const (
		writers = 8
		// Enough 33-byte lines to run past the write buffer, so the writers overlap
		// instead of each landing in a single flush.
		lines    = 10000
		fillLine = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	)

	fsys := vfs.NewOSFS()
	path := filepath.Join(t.TempDir(), "plan.json")

	bodies := make([]string, writers)
	for i := range bodies {
		bodies[i] = strings.Repeat(strconv.Itoa(i)+fillLine+"\n", lines)
	}

	group, _ := errgroup.WithContext(t.Context())

	for i := range writers {
		group.Go(func() error {
			return runner.WriteJSONOutput(fsys, path, func(w io.Writer) error {
				_, err := io.WriteString(w, bodies[i])

				return err
			})
		})
	}

	require.NoError(t, group.Wait())

	contents, err := vfs.ReadFile(fsys, path)
	require.NoError(t, err)

	assert.Contains(
		t,
		bodies,
		string(contents),
		"the published plan is a mixture rather than any single writer's output",
	)
}

// TestWriteJSONOutputDistinctPathsConcurrentWithRacing writes distinguishable plans
// from several goroutines, the way a `run --all` does. The write buffers are shared
// between units, so a buffer reaching two of them at once shows up as one unit's
// plan carrying another unit's content.
func TestWriteJSONOutputDistinctPathsConcurrentWithRacing(t *testing.T) {
	t.Parallel()

	const (
		units = 8
		// Enough lines to run well past the 256 KB write buffer.
		lines    = 10000
		fillLine = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	)

	fsys := vfs.NewOSFS()
	dir := t.TempDir()

	group, _ := errgroup.WithContext(t.Context())

	for i := range units {
		group.Go(func() error {
			body := strings.Repeat(strconv.Itoa(i)+fillLine+"\n", lines)

			return runner.WriteJSONOutput(
				fsys,
				filepath.Join(dir, strconv.Itoa(i)+".json"),
				func(w io.Writer) error {
					_, err := io.WriteString(w, body)

					return err
				},
			)
		})
	}

	require.NoError(t, group.Wait())

	for i := range units {
		contents, err := vfs.ReadFile(fsys, filepath.Join(dir, strconv.Itoa(i)+".json"))
		require.NoError(t, err)

		want := strings.Repeat(strconv.Itoa(i)+fillLine+"\n", lines)
		assert.Equal(t, want, string(contents), "unit %d received another unit's content", i)
	}
}

// TestWriteJSONOutputLeavesNoScratchFiles pins that scratch files are removed on
// both the success and failure paths. Their names are random, so a leaked one
// accumulates rather than being overwritten by the next run.
func TestWriteJSONOutputLeavesNoScratchFiles(t *testing.T) {
	t.Parallel()

	fsys := vfs.NewMemMapFS()
	dir := t.TempDir()

	require.NoError(t, runner.WriteJSONOutput(fsys, filepath.Join(dir, "ok.json"), func(w io.Writer) error {
		_, err := io.WriteString(w, `{"ok":true}`)

		return err
	}))

	err := runner.WriteJSONOutput(fsys, filepath.Join(dir, "bad.json"), func(w io.Writer) error {
		return io.ErrClosedPipe
	})
	require.Error(t, err)

	entries, err := vfs.ReadDir(fsys, dir)
	require.NoError(t, err)

	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		names = append(names, entry.Name())
	}

	assert.ElementsMatch(t, []string{"ok.json"}, names)
}
