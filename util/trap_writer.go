package util

import (
	"io"

	"github.com/gruntwork-io/terragrunt/internal/errors"
)

// TrapWriter intercepts any messages received from the `writer` output.
// Used when necessary to filter logs from terraform.
type TrapWriter struct {
	writer io.Writer
	msgs   [][]byte
}

// NewTrapWriter returns a new TrapWriter instance.
func NewTrapWriter(writer io.Writer) *TrapWriter {
	return &TrapWriter{
		writer: writer,
	}
}

// Flush flushes intercepted messages to the writer.
func (trap *TrapWriter) Flush() error {
	for _, msg := range trap.msgs {
		if _, err := trap.writer.Write(msg); err != nil {
			return errors.New(err)
		}
	}

	return nil
}

// Write implements `io.Writer` interface.
func (trap *TrapWriter) Write(msg []byte) (int, error) {
	trap.msgs = append(trap.msgs, msg)

	return len(msg), nil
}
