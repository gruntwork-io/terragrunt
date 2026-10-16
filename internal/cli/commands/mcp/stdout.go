package mcp

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/charmbracelet/x/ansi"
	"github.com/gruntwork-io/terragrunt/internal/report"
	"github.com/gruntwork-io/terragrunt/internal/runner"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
)

const (
	// stdoutMaxBytes caps the stdout kept for one unit. A unit whose stdout
	// passes it also holds this much in memory until its run ends.
	stdoutMaxBytes = 8 << 20

	// stdoutFileNameMaxLen caps the part of a stdout file's name taken from
	// the unit's directory name.
	stdoutFileNameMaxLen = 64

	// stdoutRawSuffix marks the file a unit's stdout streams into before its
	// run ends.
	stdoutRawSuffix = ".raw"

	stdoutFilePerms = 0o600

	stdoutTruncatedNote = "[stdout truncated: Terragrunt keeps the last %d bytes per unit]\n\n"
)

// stdoutCapture is the stdout of one call. It is a
// [runner.UnitOutputSplitter], so each unit's output goes to a file of its own,
// and output that belongs to no unit is dropped.
type stdoutCapture struct {
	units map[string]*unitStdout
	dir   string
	mu    sync.Mutex
}

// newStdoutCapture creates the call's directory under root, named after
// command, and returns the capture that writes into it.
func newStdoutCapture(fsys vfs.FS, root, command string) (*stdoutCapture, error) {
	dir, err := vfs.MkdirTemp(fsys, root, command+"-")
	if err != nil {
		return nil, fmt.Errorf("creating the stdout directory: %w", err)
	}

	return &stdoutCapture{dir: dir, units: map[string]*unitStdout{}}, nil
}

var _ runner.UnitOutputSplitter = (*stdoutCapture)(nil)

func (c *stdoutCapture) Write(p []byte) (int, error) {
	return len(p), nil
}

// ForUnit opens the file the stdout of the unit at unitPath streams into on
// fsys, and returns the writer for it. The unit's file appears when the run
// ends, and a unit that prints nothing has none.
func (c *stdoutCapture) ForUnit(fsys vfs.FS, unitPath string) io.Writer {
	c.mu.Lock()
	defer c.mu.Unlock()

	key := filepath.Clean(unitPath)
	if out, ok := c.units[key]; ok {
		return out
	}

	name := fmt.Sprintf("%03d-%s.txt", len(c.units), stdoutFileName(filepath.Base(key)))
	out := openUnitStdout(fsys, filepath.Join(c.dir, name))
	c.units[key] = out

	return out
}

// close finishes every unit's file.
func (c *stdoutCapture) close(fsys vfs.FS) {
	c.mu.Lock()
	defer c.mu.Unlock()

	for _, out := range c.units {
		out.close(fsys)
	}
}

// annotate fills in the stdout file of each unit result, and returns a note
// for each unit whose stdout could not be written in full. units is the
// summary of r, in r.Runs order.
func (c *stdoutCapture) annotate(r *report.Report, units []runAllUnitResult) []string {
	c.mu.Lock()
	defer c.mu.Unlock()

	var notes []string

	for i, unitRun := range r.Runs {
		out, ok := c.units[filepath.Clean(unitRun.Path)]
		if !ok {
			continue
		}

		if out.err != nil {
			notes = append(
				notes,
				fmt.Sprintf("stdout for %s is incomplete: %v", units[i].Path, out.err),
			)
		}

		if out.wrote {
			units[i].Stdout = out.path
			units[i].StdoutTruncated = out.tail != nil
		}
	}

	return notes
}

// unitStdout writes one unit's stdout, without color, and keeps the last
// [stdoutMaxBytes] of it. The first stdoutMaxBytes stream to a raw file and
// the rest to a ring buffer of the same size, allocated only then. When the
// run ends the kept tail becomes the unit's file.
//
// A failed write does not fail the run. The error is reported alongside the
// result.
type unitStdout struct {
	file    vfs.File
	tail    *tailBuffer
	err     error
	path    string
	written int
	mu      sync.Mutex
	wrote   bool
	closed  bool
}

// openUnitStdout creates the raw file the unit's stdout streams into. path is
// the unit's file, which replaces the raw file when the run ends. When the
// raw file cannot be created, the writer drops what it is given and reports
// the error with the result.
func openUnitStdout(fsys vfs.FS, path string) *unitStdout {
	file, err := fsys.OpenFile(
		path+stdoutRawSuffix,
		os.O_RDWR|os.O_CREATE|os.O_EXCL,
		stdoutFilePerms,
	)
	if err != nil {
		return &unitStdout{path: path, err: err}
	}

	return &unitStdout{file: file, path: path}
}

func (o *unitStdout) Write(p []byte) (int, error) {
	o.mu.Lock()
	defer o.mu.Unlock()

	if o.closed {
		return len(p), nil
	}

	o.store([]byte(ansi.Strip(string(p))))

	return len(p), nil
}

// store appends text to the raw file up to [stdoutMaxBytes], and the rest
// to the ring buffer.
func (o *unitStdout) store(text []byte) {
	if o.err != nil || len(text) == 0 {
		return
	}

	if o.tail == nil {
		head := text
		if room := stdoutMaxBytes - o.written; len(head) > room {
			head = head[:room]
		}

		n, err := o.file.Write(head)
		o.written += n

		if err != nil {
			o.err = err

			return
		}

		text = text[len(head):]
		if len(text) == 0 {
			return
		}

		o.tail = newTailBuffer(stdoutMaxBytes)
	}

	o.tail.write(text)
}

// close turns the raw file into the unit's file.
func (o *unitStdout) close(fsys vfs.FS) {
	o.mu.Lock()
	defer o.mu.Unlock()

	if o.closed {
		return
	}

	o.closed = true

	if o.file == nil {
		return
	}

	o.err = errors.Join(o.err, o.finish(fsys))
}

// finish renames the raw file into place when the stdout fit in it. When it did
// not, finish writes the kept tail to the unit's file and removes the raw file.
// A unit that printed nothing gets no file.
func (o *unitStdout) finish(fsys vfs.FS) error {
	raw := o.path + stdoutRawSuffix

	if o.written == 0 {
		return errors.Join(o.file.Close(), fsys.Remove(raw))
	}

	if o.tail == nil {
		if err := o.file.Close(); err != nil {
			return err
		}

		if err := fsys.Rename(raw, o.path); err != nil {
			return err
		}

		o.wrote = true

		return nil
	}

	tailErr := o.writeTail(fsys)
	closeErr := o.file.Close()
	removeErr := fsys.Remove(raw)

	return errors.Join(tailErr, closeErr, removeErr)
}

// writeTail writes the note and then the last [stdoutMaxBytes] of the
// stdout to the unit's file: the end of the raw file that the ring buffer
// does not cover, then the ring buffer, starting on a whole line.
func (o *unitStdout) writeTail(fsys vfs.FS) error {
	file, err := fsys.OpenFile(o.path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, stdoutFilePerms)
	if err != nil {
		return err
	}

	o.wrote = true

	if _, err := fmt.Fprintf(file, stdoutTruncatedNote, stdoutMaxBytes); err != nil {
		return errors.Join(err, file.Close())
	}

	w := &lineStartWriter{w: file}

	// The raw file is full, so the kept window starts as far into it as the
	// ring buffer is long.
	if skip := o.tail.size(); skip < stdoutMaxBytes {
		if _, err := o.file.Seek(int64(skip), io.SeekStart); err != nil {
			return errors.Join(err, file.Close())
		}

		if _, err := io.Copy(w, o.file); err != nil {
			return errors.Join(err, file.Close())
		}
	}

	if err := o.tail.writeTo(w); err != nil {
		return errors.Join(err, file.Close())
	}

	return file.Close()
}

// tailBuffer is a ring buffer that keeps the last len(buf) bytes written to
// it.
type tailBuffer struct {
	buf  []byte
	pos  int
	full bool
}

func newTailBuffer(size int) *tailBuffer {
	return &tailBuffer{buf: make([]byte, size)}
}

func (b *tailBuffer) write(p []byte) {
	if len(p) == 0 {
		return
	}

	if len(p) >= len(b.buf) {
		copy(b.buf, p[len(p)-len(b.buf):])
		b.pos, b.full = 0, true

		return
	}

	n := copy(b.buf[b.pos:], p)
	copy(b.buf, p[n:])

	b.pos = (b.pos + len(p)) % len(b.buf)
	b.full = b.full || n < len(p) || b.pos == 0
}

func (b *tailBuffer) size() int {
	if b.full {
		return len(b.buf)
	}

	return b.pos
}

// writeTo writes the held bytes to w in the order they arrived.
func (b *tailBuffer) writeTo(w io.Writer) error {
	if !b.full {
		_, err := w.Write(b.buf[:b.pos])

		return err
	}

	if _, err := w.Write(b.buf[b.pos:]); err != nil {
		return err
	}

	_, err := w.Write(b.buf[:b.pos])

	return err
}

// lineStartWriter drops what it is given up to and including the first
// newline, so what reaches w starts on a whole line.
type lineStartWriter struct {
	w       io.Writer
	started bool
}

func (s *lineStartWriter) Write(p []byte) (int, error) {
	if s.started {
		return s.w.Write(p)
	}

	_, rest, found := bytes.Cut(p, []byte{'\n'})
	if !found {
		return len(p), nil
	}

	s.started = true

	if _, err := s.w.Write(rest); err != nil {
		return 0, err
	}

	return len(p), nil
}

// stdoutFileName keeps the characters of a unit's directory name that are
// safe in a file name on every platform, and caps its length.
func stdoutFileName(name string) string {
	safe := strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z',
			r >= 'A' && r <= 'Z',
			r >= '0' && r <= '9',
			r == '-',
			r == '_',
			r == '.':
			return r
		}

		return '_'
	}, name)

	if len(safe) > stdoutFileNameMaxLen {
		return safe[:stdoutFileNameMaxLen]
	}

	return safe
}
