package runner_test

import (
	"bytes"
	"errors"
	"io"
	"path/filepath"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/runner"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWriteJSONOutput(t *testing.T) {
	t.Parallel()

	fsys := vfs.NewMemMapFS()
	path := filepath.Join(t.TempDir(), "nested", "plan.json")

	err := runner.WriteJSONOutput(fsys, path, func(w io.Writer) error {
		_, err := io.WriteString(w, `{"format_version":"1.2"}`)

		return err
	})
	require.NoError(t, err)

	contents, err := vfs.ReadFile(fsys, path)
	require.NoError(t, err)
	assert.JSONEq(t, `{"format_version":"1.2"}`, string(contents))
}

func TestWriteJSONOutputLeavesNoFileWhenRunFails(t *testing.T) {
	t.Parallel()

	fsys := vfs.NewMemMapFS()
	path := filepath.Join(t.TempDir(), "plan.json")
	sentinel := errors.New("show failed")

	err := runner.WriteJSONOutput(fsys, path, func(w io.Writer) error {
		if _, err := io.WriteString(w, `{"format_ver`); err != nil {
			return err
		}

		return sentinel
	})
	require.ErrorIs(t, err, sentinel)

	assert.False(
		t,
		vfs.Exists(fsys, path),
		"a failed run must not leave a partial plan behind",
	)
}

func TestWriteJSONOutputKeepsPreviousPlanWhenRunFails(t *testing.T) {
	t.Parallel()

	fsys := vfs.NewMemMapFS()
	path := filepath.Join(t.TempDir(), "plan.json")

	require.NoError(t, runner.WriteJSONOutput(fsys, path, func(w io.Writer) error {
		_, err := io.WriteString(w, `{"run":"first"}`)

		return err
	}))

	err := runner.WriteJSONOutput(fsys, path, func(w io.Writer) error {
		if _, err := io.WriteString(w, `{"run":"sec`); err != nil {
			return err
		}

		return errors.New("show failed")
	})
	require.Error(t, err)

	contents, err := vfs.ReadFile(fsys, path)
	require.NoError(t, err)
	assert.JSONEq(t, `{"run":"first"}`, string(contents))
}

func TestWriteJSONOutputTruncatesExistingFile(t *testing.T) {
	t.Parallel()

	fsys := vfs.NewMemMapFS()
	path := filepath.Join(t.TempDir(), "plan.json")

	require.NoError(t, runner.WriteJSONOutput(fsys, path, func(w io.Writer) error {
		_, err := io.WriteString(w, `{"stale":true,"padding":"aaaaaaaaaaaaaaaa"}`)

		return err
	}))

	require.NoError(t, runner.WriteJSONOutput(fsys, path, func(w io.Writer) error {
		_, err := io.WriteString(w, `{"fresh":true}`)

		return err
	}))

	contents, err := vfs.ReadFile(fsys, path)
	require.NoError(t, err)
	assert.JSONEq(t, `{"fresh":true}`, string(contents))
}

// planJSONChunk is one pipe-sized read of a plan document, so the benchmark feeds
// the writer in chunks the way a real run does rather than in one large write.
func planJSONChunk() []byte {
	const chunkSize = 32 << 10

	line := []byte(
		`{"address":"module.vpc.aws_subnet.private[0]","mode":"managed",` +
			`"type":"aws_subnet","change":{"actions":["create"]}},`,
	)

	return bytes.Repeat(line, chunkSize/len(line)+1)
}

func showJSON(w io.Writer, size int) error {
	chunk := planJSONChunk()

	for written := 0; written < size; written += len(chunk) {
		if _, err := w.Write(chunk); err != nil {
			return err
		}
	}

	return nil
}

// writeJSONOutputBuffered is the strategy the unit runner used before. It stays
// here as the baseline the streaming numbers are read against, since the code it
// mirrors is gone.
func writeJSONOutputBuffered(fsys vfs.FS, path string, fn func(w io.Writer) error) error {
	const (
		dirPerms  = 0o700
		filePerms = 0o600
	)

	var buf bytes.Buffer

	if err := fn(&buf); err != nil {
		return err
	}

	if err := fsys.MkdirAll(filepath.Dir(path), dirPerms); err != nil {
		return err
	}

	return vfs.WriteFile(fsys, path, buf.Bytes(), filePerms)
}

// BenchmarkWriteJSONOutput contrasts streaming a plan document to disk against
// buffering it first. Read the bytes-per-operation column. Under `run --all` every
// unit running concurrently pays that figure at the same time.
func BenchmarkWriteJSONOutput(b *testing.B) {
	sizes := []struct {
		name string
		size int
	}{
		{name: "1MiB", size: 1 << 20},
		{name: "16MiB", size: 16 << 20},
		{name: "64MiB", size: 64 << 20},
	}

	strategies := []struct {
		write func(vfs.FS, string, func(io.Writer) error) error
		name  string
	}{
		{write: runner.WriteJSONOutput, name: "stream"},
		{write: writeJSONOutputBuffered, name: "buffered"},
	}

	for _, strategy := range strategies {
		for _, size := range sizes {
			b.Run(strategy.name+"/"+size.name, func(b *testing.B) {
				fsys := vfs.NewOSFS()
				path := filepath.Join(b.TempDir(), "plan.json")

				b.ReportAllocs()
				b.SetBytes(int64(size.size))

				for b.Loop() {
					err := strategy.write(fsys, path, func(w io.Writer) error {
						return showJSON(w, size.size)
					})
					if err != nil {
						b.Fatal(err)
					}
				}
			})
		}
	}
}
