package runner

import (
	"bufio"
	"errors"
	"io"
	"path/filepath"
	"sync"

	"github.com/gruntwork-io/terragrunt/internal/vfs"
)

const (
	jsonOutputDirPerms  = 0o700
	jsonOutputFilePerms = 0o600

	// jsonOutputBufferSize coalesces the small chunks arriving from `show -json`
	// into fewer, larger writes. Without it, streaming trades one large allocation
	// for a write syscall per chunk and runs slower than the buffering it replaced.
	// Every unit running concurrently holds one, so it stays fixed rather than
	// scaling with the document.
	jsonOutputBufferSize = 256 << 10

	jsonOutputTempPattern = ".*.tmp"
)

// jsonOutputWriters bounds buffer memory by how many units write at once. A fresh
// buffer per call costs jsonOutputBufferSize for every unit in a `run --all`.
var jsonOutputWriters = sync.Pool{
	New: func() any {
		return bufio.NewWriterSize(io.Discard, jsonOutputBufferSize)
	},
}

// WriteJSONOutput streams whatever fn writes into the file at path, creating the
// parent directory first. Plan JSON runs to tens of megabytes on large units and
// every unit in a `run --all` produces its own, so the writer is handed straight
// to fn rather than accumulating the document in memory.
//
// The document lands in a temporary file that replaces path only once fn returns
// cleanly. A failed run therefore leaves whatever was already at path untouched,
// and no reader observes a half-written plan.
func WriteJSONOutput(fsys vfs.FS, path string, fn func(w io.Writer) error) error {
	if err := fsys.MkdirAll(filepath.Dir(path), jsonOutputDirPerms); err != nil {
		return err
	}

	// Writers can aim at one path concurrently, so the scratch file cannot be named
	// after it.
	file, err := vfs.CreateTemp(fsys, filepath.Dir(path), filepath.Base(path)+jsonOutputTempPattern)
	if err != nil {
		return err
	}

	tmpPath := file.Name()

	// The rename carries this mode onto the published plan.
	if err := fsys.Chmod(tmpPath, jsonOutputFilePerms); err != nil {
		return errors.Join(err, file.Close(), fsys.Remove(tmpPath))
	}

	// Nothing but New and releaseJSONOutputWriter fills this pool, so the assertion holds.
	buffered := jsonOutputWriters.Get().(*bufio.Writer)
	buffered.Reset(file)

	defer releaseJSONOutputWriter(buffered)

	if err := fn(buffered); err != nil {
		return errors.Join(err, file.Close(), fsys.Remove(tmpPath))
	}

	if err := errors.Join(buffered.Flush(), file.Close()); err != nil {
		return errors.Join(err, fsys.Remove(tmpPath))
	}

	if err := fsys.Rename(tmpPath, path); err != nil {
		return errors.Join(err, fsys.Remove(tmpPath))
	}

	return nil
}

// releaseJSONOutputWriter returns w to the pool. Resetting it away from the file
// stops a parked buffer from holding the last unit's closed file for as long as the
// pool keeps it.
func releaseJSONOutputWriter(w *bufio.Writer) {
	w.Reset(io.Discard)
	jsonOutputWriters.Put(w)
}
