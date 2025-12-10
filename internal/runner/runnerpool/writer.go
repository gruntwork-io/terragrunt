package runnerpool

import (
	"bytes"
	"context"
	"io"
	"sync"
)

// UnitWriter buffers output for a single unit and flushes incrementally during execution.
// This prevents interleaved output when multiple units run in parallel while ensuring
// output appears in real-time during execution, not just at completion.
// It also flushes when the context is cancelled to prevent output loss on interrupt.
type UnitWriter struct {
	out    io.Writer
	ctx    context.Context
	buffer bytes.Buffer
	mu     sync.Mutex
}

// NewUnitWriter returns a new UnitWriter instance.
// If ctx is nil, context.Background() is used.
//
//nolint:contextcheck // Context is passed from caller, not created here
func NewUnitWriter(ctx context.Context, out io.Writer) *UnitWriter {
	if ctx == nil {
		ctx = context.Background()
	}

	return &UnitWriter{
		out: out,
		ctx: ctx,
	}
}

func (writer *UnitWriter) Write(p []byte) (int, error) {
	writer.mu.Lock()
	defer writer.mu.Unlock()

	// Write to buffer
	n, err := writer.buffer.Write(p)
	if err != nil {
		return n, err
	}

	// Flush complete lines (ending with newline) immediately during execution
	// This ensures output appears incrementally, not just at completion
	if flushErr := writer.flushCompleteLines(); flushErr != nil {
		return n, flushErr
	}

	// If context is cancelled, flush any remaining buffered output immediately
	// This ensures output is not lost when the process is interrupted
	if flushErr := writer.flushOnCancel(); flushErr != nil {
		return n, flushErr
	}

	return n, err
}

// flushCompleteLines flushes any complete lines (ending with newline) from the buffer.
// Partial lines (without trailing newline) remain in the buffer.
// Returns any error from writing to the output writer.
func (writer *UnitWriter) flushCompleteLines() error {
	if writer.out == nil {
		return nil
	}

	// Find the last newline in the buffer
	buf := writer.buffer.Bytes()
	lastNewline := bytes.LastIndexByte(buf, '\n')

	if lastNewline >= 0 {
		// We have at least one complete line - flush up to and including the last newline
		lineCount := lastNewline + 1
		lines := writer.buffer.Next(lineCount)

		if _, err := writer.out.Write(lines); err != nil {
			// Write failed, put the data back
			writer.buffer.Write(lines)
			return err
		}
	}

	return nil
}

// flushOnCancel flushes any remaining buffered output if the context is cancelled.
// Returns any error from writing to the output writer.
func (writer *UnitWriter) flushOnCancel() error {
	if writer.ctx.Err() != nil && writer.buffer.Len() > 0 {
		return writer.flushUnsafe()
	}

	return nil
}

// Flush flushes buffer data to the output writer.
// It also flushes if the context is cancelled, ensuring output is not lost on interrupt.
func (writer *UnitWriter) Flush() error {
	writer.mu.Lock()
	defer writer.mu.Unlock()

	// Flush on cancel first, then flush remaining buffer
	if err := writer.flushOnCancel(); err != nil {
		return err
	}

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
