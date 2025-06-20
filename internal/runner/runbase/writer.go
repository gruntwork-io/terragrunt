package runbase

import (
	"bytes"
	"fmt"
	"io"

	"github.com/gruntwork-io/terragrunt/internal/errors"
)

// UnitWriter represents a Writer with data buffering.
// We should avoid outputting data directly to the output out,
// since when units run in parallel, the output data may be mixed with each other, thereby spoiling each other's results.
type UnitWriter struct {
	buffer *bytes.Buffer
	out    io.Writer
}

// NewUnitWriter returns a new UnitWriter instance.
func NewUnitWriter(out io.Writer) *UnitWriter {
	return &UnitWriter{
		buffer: &bytes.Buffer{},
		out:    out,
	}
}

// Write appends the contents of p to the buffer.
func (writer *UnitWriter) Write(p []byte) (int, error) {
	n, err := writer.buffer.Write(p)
	if err != nil {
		return n, errors.New(err)
	}

	// If the last byte is a newline character, flush the buffer early.
	if writer.buffer.Len() > 0 {
		if p[len(p)-1] == '\n' {
			if err := writer.Flush(); err != nil {
				return n, errors.New(err)
			}
		}
	}

	return n, nil
}

// Flush flushes buffer data to the `out` writer.
func (writer *UnitWriter) Flush() error {
	if _, err := fmt.Fprint(writer.out, writer.buffer); err != nil {
		return errors.New(err)
	}

	writer.buffer.Reset()

	return nil
}
