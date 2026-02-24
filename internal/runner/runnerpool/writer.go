package runnerpool

import (
	"bytes"
	"io"
	"sync"
)

// UnitWriter buffers output for a single unit and flushes incrementally during execution.
// This prevents interleaved output when multiple units run in parallel while ensuring
// output appears in real-time during execution, not just at completion.
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

// ParentWriter returns the underlying output writer that this UnitWriter wraps.
func (writer *UnitWriter) ParentWriter() io.Writer {
	return writer.out
}
