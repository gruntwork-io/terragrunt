package runnerpool

import (
	"bytes"
	"io"
	"sync"
)

// UnitWriter buffers output for a single unit and flushes atomically on completion.
// This prevents interleaved output when multiple units run in parallel.
type UnitWriter struct {
	out    io.Writer
	buffer bytes.Buffer
	mu     sync.Mutex
}

// NewUnitWriter returns a new UnitWriter instance.
func NewUnitWriter(out io.Writer) *UnitWriter {
	return &UnitWriter{
		out: out,
	}
}

func (writer *UnitWriter) Write(p []byte) (int, error) {
	writer.mu.Lock()
	defer writer.mu.Unlock()

	return writer.buffer.Write(p)
}

// Flush flushes buffer data to the output writer.
func (writer *UnitWriter) Flush() error {
	writer.mu.Lock()
	defer writer.mu.Unlock()

	return writer.flushUnsafe()
}

// flushUnsafe flushes buffer without acquiring a lock (useful when lock is held).
func (writer *UnitWriter) flushUnsafe() error {
	if writer.out != nil {
		if _, err := writer.buffer.WriteTo(writer.out); err != nil {
			return err
		}
	}

	return nil
}

// ParentWriter returns the underlying output writer that this UnitWriter wraps.
// This is used for creating writer-based locks to serialize concurrent flushes
// to the same parent writer.
func (writer *UnitWriter) ParentWriter() io.Writer {
	return writer.out
}
