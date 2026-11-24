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
	"github.com/gruntwork-io/terragrunt/internal/runner"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/gruntwork-io/terragrunt/pkg/log"
)

const (
	// stdoutMaxBytes caps the stdout kept for one unit. A unit whose stdout
	// passes it also holds this much in memory until the unit finishes.
	stdoutMaxBytes = 8 << 20

	// stdoutFileNameMaxLen caps the part of a stdout file's name taken from
	// the unit's directory name.
	stdoutFileNameMaxLen = 64

	// stdoutRawSuffix marks the file a unit's stdout streams into before its
	// run ends.
	stdoutRawSuffix = ".raw"

	// stdoutPlanDir is the directory, under a call's directory, that holds
	// the plan files of a plan call.
	stdoutPlanDir = "plans"

	stdoutFilePerms = 0o600

	stdoutTruncatedNote = "[stdout truncated: Terragrunt keeps the last %d bytes per unit]\n\n"

	ansiEscape = 0x1b
)

// stdoutCapture is the stdout of one call. It is a
// [runner.UnitOutputSplitter], so each unit's output goes to a file of its own,
// and output that belongs to no unit is dropped.
type stdoutCapture struct {
	fsys  vfs.FS
	units map[string]*unitStdout
	dir   string
	mu    sync.Mutex
}

// newStdoutCapture creates the call's directory under root on fsys, named
// after command, and returns the capture that writes into it.
func newStdoutCapture(fsys vfs.FS, root, command string) (*stdoutCapture, error) {
	dir, err := vfs.MkdirTemp(fsys, root, command+"-")
	if err != nil {
		return nil, fmt.Errorf("creating the stdout directory: %w", err)
	}

	return &stdoutCapture{fsys: fsys, dir: dir, units: map[string]*unitStdout{}}, nil
}

var _ runner.UnitOutputSplitter = (*stdoutCapture)(nil)

func (c *stdoutCapture) Write(p []byte) (int, error) {
	return len(p), nil
}

// ForUnit opens the file the stdout of the unit at unitPath streams into on
// fsys, and returns the writer for it. The unit's file appears when the
// writer is closed, and a unit that prints nothing has none.
func (c *stdoutCapture) ForUnit(fsys vfs.FS, unitPath string) io.WriteCloser {
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

// planDir returns the directory a plan call writes its plan files into.
func (c *stdoutCapture) planDir() string {
	return filepath.Join(c.dir, stdoutPlanDir)
}

// close finishes the file of every unit whose writer is still open, and logs
// each file it could not finish to l.
func (c *stdoutCapture) close(l log.Logger) {
	c.mu.Lock()
	defer c.mu.Unlock()

	for unitPath, out := range c.units {
		if err := out.Close(); err != nil {
			l.Errorf("Error closing the stdout file of unit %s: %v", unitPath, err)
		}
	}
}

// discard removes the call's directory and everything in it.
func (c *stdoutCapture) discard() error {
	return c.fsys.RemoveAll(c.dir)
}

// annotate fills in the stdout file of each unit result, and returns a note
// for each unit whose stdout could not be written in full.
func (c *stdoutCapture) annotate(units []runAllUnitResult) []string {
	c.mu.Lock()
	defer c.mu.Unlock()

	var notes []string

	for i := range units {
		out, ok := c.units[units[i].absPath]
		if !ok {
			continue
		}

		path, truncated, err := out.result()
		if err != nil {
			notes = append(
				notes,
				fmt.Sprintf("stdout for %s is incomplete: %v", units[i].Path, err),
			)
		}

		units[i].Stdout = path
		units[i].StdoutTruncated = truncated
	}

	return notes
}

// unitStdout writes one unit's stdout, without color, and keeps the last
// [stdoutMaxBytes] of it. The first stdoutMaxBytes stream to a raw file and
// the rest to a ring buffer, allocated only then. Closing it turns what it
// kept into the unit's file and frees the ring buffer.
//
// A failed write does not fail the run. The error is reported alongside the
// result.
type unitStdout struct {
	fsys      vfs.FS
	file      vfs.File
	tail      *tailBuffer
	err       error
	path      string
	written   int
	mu        sync.Mutex
	wrote     bool
	truncated bool
	closed    bool
}

// openUnitStdout creates the raw file the unit's stdout streams into. path is
// the unit's file, which replaces the raw file when the writer is closed. When
// the raw file cannot be created, the writer drops what it is given and
// reports the error with the result.
func openUnitStdout(fsys vfs.FS, path string) *unitStdout {
	file, err := fsys.OpenFile(
		path+stdoutRawSuffix,
		os.O_RDWR|os.O_CREATE|os.O_EXCL,
		stdoutFilePerms,
	)
	if err != nil {
		return &unitStdout{fsys: fsys, path: path, err: err}
	}

	return &unitStdout{fsys: fsys, file: file, path: path}
}

func (o *unitStdout) Write(p []byte) (int, error) {
	o.mu.Lock()
	defer o.mu.Unlock()

	if o.closed {
		return len(p), nil
	}

	text := p
	if bytes.IndexByte(p, ansiEscape) >= 0 {
		text = []byte(ansi.Strip(string(p)))
	}

	o.store(text)

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

		o.tail = newTailBuffer(stdoutMaxBytes + 1)
	}

	o.tail.write(text)
}

// Close turns the raw file into the unit's file. The error it returns is also
// reported with the result.
func (o *unitStdout) Close() error {
	o.mu.Lock()
	defer o.mu.Unlock()

	if o.closed {
		return nil
	}

	o.closed = true

	if o.file == nil {
		return nil
	}

	err := o.finish()
	o.err = errors.Join(o.err, err)
	o.tail = nil

	return err
}

// result returns the unit's file, whether its start was cut, and the error
// that kept any of its stdout from it. The path is empty when there is no
// file.
func (o *unitStdout) result() (string, bool, error) {
	o.mu.Lock()
	defer o.mu.Unlock()

	if !o.wrote {
		return "", false, o.err
	}

	return o.path, o.truncated, o.err
}

// finish renames the raw file into place when the stdout fit in it. When it did
// not, finish writes the kept tail to the unit's file and removes the raw file.
// A unit that printed nothing gets no file.
func (o *unitStdout) finish() error {
	raw := o.path + stdoutRawSuffix

	if o.written == 0 {
		return errors.Join(o.file.Close(), o.fsys.Remove(raw))
	}

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
	if tailErr == nil {
		o.wrote, o.truncated = true, true
	}

	return errors.Join(tailErr, o.file.Close(), o.fsys.Remove(raw))
}

// writeTail writes the note and then the last [stdoutMaxBytes] of the stdout
// to the unit's file. A file that could not be written in full is removed.
func (o *unitStdout) writeTail() error {
	file, err := o.fsys.OpenFile(o.path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, stdoutFilePerms)
	if err != nil {
		return err
	}

	if err := o.copyTail(file); err != nil {
		return errors.Join(err, file.Close(), o.fsys.Remove(o.path))
	}

	if err := file.Close(); err != nil {
		return errors.Join(err, o.fsys.Remove(o.path))
	}

	return nil
}

// copyTail writes the note to w, then the kept window. The window starts on a
// whole line, unless it holds no line break at all.
func (o *unitStdout) copyTail(w io.Writer) error {
	if _, err := fmt.Fprintf(w, stdoutTruncatedNote, stdoutMaxBytes); err != nil {
		return err
	}

	win, err := o.window()
	if err != nil {
		return err
	}

	lw := &lineStartWriter{w: w, started: win.byteBefore == '\n'}

	if !lw.started {
		found, err := win.hasNewline()
		if err != nil {
			return err
		}

		lw.started = !found
	}

	if win.raw != nil {
		if _, err := io.Copy(lw, win.raw); err != nil {
			return err
		}
	}

	for _, part := range [][]byte{win.older, win.newer} {
		if _, err := lw.Write(part); err != nil {
			return err
		}
	}

	return nil
}

// tailWindow is the last [stdoutMaxBytes] of a unit's stdout, in order: a
// section of the raw file, then the older and newer parts of the ring buffer.
type tailWindow struct {
	raw        *io.SectionReader
	older      []byte
	newer      []byte
	byteBefore byte
}

// window returns the kept window. A ring buffer longer than the cap holds the
// whole window and the byte before it. A shorter one leaves the window
// starting as far into the full raw file as the ring buffer is long.
func (o *unitStdout) window() (tailWindow, error) {
	older, newer := o.tail.bytes()

	skip := o.tail.size()
	if skip > stdoutMaxBytes {
		return tailWindow{older: older[1:], newer: newer, byteBefore: older[0]}, nil
	}

	before := make([]byte, 1)
	if _, err := o.file.ReadAt(before, int64(skip-1)); err != nil {
		return tailWindow{}, err
	}

	return tailWindow{
		raw:        io.NewSectionReader(o.file, int64(skip), int64(stdoutMaxBytes-skip)),
		older:      older,
		newer:      newer,
		byteBefore: before[0],
	}, nil
}

// hasNewline reports whether the window holds a line break.
func (win tailWindow) hasNewline() (bool, error) {
	if bytes.IndexByte(win.older, '\n') >= 0 || bytes.IndexByte(win.newer, '\n') >= 0 {
		return true, nil
	}

	if win.raw == nil {
		return false, nil
	}

	scan := &newlineScanner{}
	if _, err := io.Copy(scan, io.NewSectionReader(win.raw, 0, win.raw.Size())); err != nil {
		return false, err
	}

	return scan.found, nil
}

// newlineScanner records whether anything written to it held a line break.
type newlineScanner struct {
	found bool
}

func (s *newlineScanner) Write(p []byte) (int, error) {
	s.found = s.found || bytes.IndexByte(p, '\n') >= 0

	return len(p), nil
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

// bytes returns the held bytes in the order they arrived, as the older part
// and then the newer part.
func (b *tailBuffer) bytes() ([]byte, []byte) {
	if !b.full {
		return b.buf[:b.pos], nil
	}

	return b.buf[b.pos:], b.buf[:b.pos]
}

// lineStartWriter drops what it is given up to and including the first
// newline, so what reaches w starts on a whole line. Once started, it passes
// everything through.
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
