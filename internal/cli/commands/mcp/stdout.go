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

// errStdoutStoreClosed is returned when a run starts after the server began
// removing its stdout directory.
var errStdoutStoreClosed = errors.New("the server is shutting down")

// stdoutStore creates the directory the plan, apply, and destroy tools write
// each unit's stdout into, and the plan tool its plan files. Every call's
// files stay until [stdoutStore.removeAll] removes the directory when the
// server stops.
type stdoutStore struct {
	root   string
	mu     sync.Mutex
	closed bool
}

// startRun returns the writer the next call's units write their stdout
// through. command names the call's directory.
func (s *stdoutStore) startRun(fsys vfs.FS, command string) (*stdoutCapture, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return nil, errStdoutStoreClosed
	}

	if s.root == "" {
		root, err := vfs.MkdirTemp(fsys, "", "terragrunt-mcp-stdout-")
		if err != nil {
			return nil, fmt.Errorf("creating the stdout directory: %w", err)
		}

		s.root = root
	}

	dir, err := vfs.MkdirTemp(fsys, s.root, command+"-")
	if err != nil {
		return nil, fmt.Errorf("creating the stdout directory: %w", err)
	}

	return &stdoutCapture{fsys: fsys, dir: dir, units: map[string]*unitStdout{}}, nil
}

// removeAll removes every file the server wrote, and refuses any run that
// starts afterwards.
func (s *stdoutStore) removeAll(fsys vfs.FS) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.closed = true

	if s.root == "" {
		return nil
	}

	if err := fsys.RemoveAll(s.root); err != nil {
		return fmt.Errorf("removing the stdout directory: %w", err)
	}

	s.root = ""

	return nil
}

// stdoutCapture is the stdout of one call. It is a
// [runner.UnitOutputSplitter], so each unit's output goes to a file of its own,
// and output that belongs to no unit is dropped. A plan call's plan files go
// under the same directory, dir.
type stdoutCapture struct {
	fsys  vfs.FS
	units map[string]*unitStdout
	dir   string
	mu    sync.Mutex
}

var _ runner.UnitOutputSplitter = (*stdoutCapture)(nil)

func (c *stdoutCapture) Write(p []byte) (int, error) {
	return len(p), nil
}

// ForUnit returns the writer for the stdout of the unit at unitPath. Its file
// appears when the run ends, and a unit that prints nothing has none.
func (c *stdoutCapture) ForUnit(unitPath string) io.Writer {
	c.mu.Lock()
	defer c.mu.Unlock()

	key := filepath.Clean(unitPath)
	if out, ok := c.units[key]; ok {
		return out
	}

	name := fmt.Sprintf("%03d-%s.txt", len(c.units), stdoutFileName(filepath.Base(key)))
	out := &unitStdout{fsys: c.fsys, path: filepath.Join(c.dir, name)}
	c.units[key] = out

	return out
}

// close finishes every unit's file.
func (c *stdoutCapture) close() {
	c.mu.Lock()
	defer c.mu.Unlock()

	for _, out := range c.units {
		out.close()
	}
}

// annotate fills in the stdout file of each unit result, and returns a note
// for each unit whose stdout could not be written in full. units must be the
// summary of r, which keeps r.Runs order.
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

// unitStdout writes one unit's stdout, without color. The first
// [stdoutMaxBytes] stream to a raw file. Text past that goes to a ring buffer
// of the same size, which only a unit that outgrows the cap allocates, and
// when the run ends the last stdoutMaxBytes of the whole text become the
// unit's file.
//
// A failed write does not fail the run. The error is reported alongside the
// result.
type unitStdout struct {
	fsys    vfs.FS
	file    vfs.File
	tail    *tailBuffer
	err     error
	path    string
	written int
	mu      sync.Mutex
	wrote   bool
	closed  bool
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
		if o.file == nil {
			file, err := o.fsys.OpenFile(
				o.path+stdoutRawSuffix,
				os.O_RDWR|os.O_CREATE|os.O_EXCL,
				stdoutFilePerms,
			)
			if err != nil {
				o.err = err

				return
			}

			o.file = file
		}

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
func (o *unitStdout) close() {
	o.mu.Lock()
	defer o.mu.Unlock()

	if o.closed {
		return
	}

	o.closed = true

	if o.file == nil {
		return
	}

	o.err = errors.Join(o.err, o.finish())
}

// finish renames the raw file into place when the stdout fit in it. When it did
// not, finish writes the kept tail to the unit's file and removes the raw file.
func (o *unitStdout) finish() error {
	raw := o.path + stdoutRawSuffix

	if o.tail == nil {
		if err := o.file.Close(); err != nil {
			return err
		}

		if err := o.fsys.Rename(raw, o.path); err != nil {
			return err
		}

		o.wrote = true

		return nil
	}

	tailErr := o.writeTail()
	closeErr := o.file.Close()
	removeErr := o.fsys.Remove(raw)

	return errors.Join(tailErr, closeErr, removeErr)
}

// writeTail writes the note and then the last [stdoutMaxBytes] of the
// stdout to the unit's file: the end of the raw file that the ring buffer
// does not cover, then the ring buffer, starting on a whole line.
func (o *unitStdout) writeTail() error {
	file, err := o.fsys.OpenFile(o.path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, stdoutFilePerms)
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

// size returns how many bytes the buffer holds.
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
