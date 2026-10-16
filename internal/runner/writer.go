package runner

import (
	"bytes"
	"io"
	"sync"
)

// UnitOutputSplitter is a writer that gives each unit a destination of its own.
// When the venv handed to [Runner.Run] writes to one, each unit's stdout goes to
// the writer ForUnit returns for that unit's path, and none of it reaches the
// shared stream.
type UnitOutputSplitter interface {
	io.Writer
	ForUnit(unitPath string) io.Writer
}

// UnitWriter buffers output for a single unit and flushes each complete line as it
// arrives. Buffering keeps parallel units from interleaving their output. Flushing per
// line keeps that output appearing during the run instead of only at the end.
type UnitWriter struct {
	out    io.Writer
	buffer bytes.Buffer
	mu     sync.Mutex
}

// NewUnitWriter returns a [UnitWriter] that writes to w.
func NewUnitWriter(w io.Writer) *UnitWriter {
	return &UnitWriter{
		out: w,
	}
}

func (writer *UnitWriter) Write(p []byte) (int, error) {
	writer.mu.Lock()
	defer writer.mu.Unlock()

	n, err := writer.buffer.Write(p)
	if err != nil {
		return n, err
	}

	if flushErr := writer.flushCompleteLines(); flushErr != nil {
		return n, flushErr
	}

	return n, err
}

// flushCompleteLines flushes any complete lines (ending with newline) from the buffer.
// Partial lines (without trailing newline) remain in the buffer.
func (writer *UnitWriter) flushCompleteLines() error {
	if writer.out == nil {
		return nil
	}

	buf := writer.buffer.Bytes()
	lastNewline := bytes.LastIndexByte(buf, '\n')

	if lastNewline >= 0 {
		lineCount := lastNewline + 1
		lines := writer.buffer.Next(lineCount)

		if _, err := writer.out.Write(lines); err != nil {
			writer.buffer.Write(lines)
			return err
		}
	}

	return nil
}

// Flush flushes all buffered data to the output writer.
func (writer *UnitWriter) Flush() error {
	writer.mu.Lock()
	defer writer.mu.Unlock()

	if writer.out != nil {
		if _, err := writer.buffer.WriteTo(writer.out); err != nil {
			return err
		}
	}

	return nil
}

// Unwrap returns the writer this [UnitWriter] wraps.
func (writer *UnitWriter) Unwrap() io.Writer {
	return writer.out
}

// unitOutput returns the writer for the stdout of the unit at unitPath. That
// is the writer w gives the unit when w is a [UnitOutputSplitter], and w
// itself otherwise.
func unitOutput(w io.Writer, unitPath string) io.Writer {
	splitter, ok := w.(UnitOutputSplitter)
	if !ok {
		return w
	}

	return splitter.ForUnit(unitPath)
}
