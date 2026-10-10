package runner_test

import (
	"bytes"
	"errors"
	"io"
	"os"
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

// TestWriteJSONOutputLeavesNoFileWhenRunPanics pins the cleanup against a
// panic, which unwinds past every return in the function. The scratch file
// holds a plan, so one left behind under a name nothing looks for outlives the
// process that wrote it.
func TestWriteJSONOutputLeavesNoFileWhenRunPanics(t *testing.T) {
	t.Parallel()

	fsys := vfs.NewMemMapFS()
	dir := t.TempDir()
	path := filepath.Join(dir, "plan.json")

	require.PanicsWithValue(t, "show exploded", func() {
		err := runner.WriteJSONOutput(fsys, path, func(w io.Writer) error {
			if _, err := io.WriteString(w, `{"format_ver`); err != nil {
				return err
			}

			panic("show exploded")
		})
		require.NoError(t, err)
	})

	entries, err := vfs.ReadDir(fsys, dir)
	require.NoError(t, err)
	assert.Empty(t, entries, "a panic must leave no scratch file behind")
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

func TestWriteJSONOutputReportsFilesystemFailures(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name string
		op   string
	}{
		{name: "directory cannot be created", op: faultMkdirAll},
		{name: "scratch file cannot be created", op: faultOpenFile},
		{name: "scratch file mode cannot be set", op: faultChmod},
		{name: "plan cannot be flushed", op: faultWrite},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			base := vfs.NewMemMapFS()
			path := filepath.Join("/out", "plan.json")

			err := runner.WriteJSONOutput(&faultFS{FS: base, op: tc.op}, path, func(w io.Writer) error {
				_, err := io.WriteString(w, `{"format_version":"1.2"}`)

				return err
			})
			require.ErrorIs(t, err, errInjected)

			assert.False(t, vfs.Exists(base, path), "a failed write must not publish a plan")

			if tc.op == faultMkdirAll {
				return
			}

			entries, err := vfs.ReadDir(base, "/out")
			require.NoError(t, err)
			assert.Empty(t, entries, "the scratch file is removed")
		})
	}
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

// BenchmarkWriteJSONOutputPerUnit writes a plan the size a small unit produces.
// Moving content to disk dominates the larger sizes below and hides the setup each
// call does first. At this size that setup is most of the cost, and a `run --all`
// pays it once per unit.
func BenchmarkWriteJSONOutputPerUnit(b *testing.B) {
	const smallPlan = 64 << 10

	fsys := vfs.NewOSFS()
	path := filepath.Join(b.TempDir(), "plan.json")

	b.ReportAllocs()
	b.SetBytes(smallPlan)

	for b.Loop() {
		require.NoError(b, runner.WriteJSONOutput(fsys, path, func(w io.Writer) error {
			return showJSON(w, smallPlan)
		}))
	}
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
					require.NoError(b, strategy.write(fsys, path, func(w io.Writer) error {
						return showJSON(w, size.size)
					}))
				}
			})
		}
	}
}

// errInjected is the failure faultFS reports.
var errInjected = errors.New("injected failure")

// Operations faultFS can be told to fail.
const (
	faultMkdirAll = "mkdirall"
	faultOpenFile = "openfile"
	faultChmod    = "chmod"
	faultWrite    = "write"
)

// faultFS fails op with errInjected on paths under under, or on every path when under is empty.
// It hides the wrapped filesystem's optional interfaces.
type faultFS struct {
	vfs.FS
	op    string
	under string
}

func (fsys *faultFS) fails(op, path string) bool {
	if fsys.op != op {
		return false
	}

	return fsys.under == "" || vfs.Within(fsys.FS, fsys.under, path)
}

func (fsys *faultFS) MkdirAll(path string, perm os.FileMode) error {
	if fsys.fails(faultMkdirAll, path) {
		return &os.PathError{Op: "mkdir", Path: path, Err: errInjected}
	}

	return fsys.FS.MkdirAll(path, perm)
}

func (fsys *faultFS) OpenFile(name string, flag int, perm os.FileMode) (vfs.File, error) {
	if fsys.fails(faultOpenFile, name) {
		return nil, &os.PathError{Op: "open", Path: name, Err: errInjected}
	}

	file, err := fsys.FS.OpenFile(name, flag, perm)
	if err != nil {
		return nil, err
	}

	if fsys.fails(faultWrite, name) {
		return &failWriteFile{File: file}, nil
	}

	return file, nil
}

func (fsys *faultFS) Chmod(name string, mode os.FileMode) error {
	if fsys.fails(faultChmod, name) {
		return &os.PathError{Op: "chmod", Path: name, Err: errInjected}
	}

	return fsys.FS.Chmod(name, mode)
}

// failWriteFile fails every write.
type failWriteFile struct {
	vfs.File
}

func (f *failWriteFile) Write([]byte) (int, error) {
	return 0, errInjected
}
