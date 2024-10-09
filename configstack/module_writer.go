package configstack

import (
	"bytes"
	"fmt"
	"io"

	"github.com/gruntwork-io/terragrunt/internal/errors"
)

// ModuleWriter represents a Writer with data buffering.
// We should avoid outputting data directly to the output out,
// since when modules run in parallel, the output data may be mixed with each other, thereby spoiling each other's results.
type ModuleWriter struct {
	buffer *bytes.Buffer
	out    io.Writer
}

// NewModuleWriter returns a new ModuleWriter instance.
func NewModuleWriter(out io.Writer) *ModuleWriter {
	return &ModuleWriter{
		buffer: &bytes.Buffer{},
		out:    out,
	}
}

// Write appends the contents of p to the buffer.
func (writer *ModuleWriter) Write(p []byte) (n int, err error) {
	return writer.buffer.Write(p)
}

// Flush flushes buffer data to the `out` writer.
func (writer *ModuleWriter) Flush() error {
	if _, err := fmt.Fprint(writer.out, writer.buffer); err != nil {
		return errors.New(err)
	}

	writer.buffer.Reset()

	return nil
}
