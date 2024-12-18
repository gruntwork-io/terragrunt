package util

import (
	"fmt"
	"io"
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
			fmt.Println("occurred formatter error: " + err.Error())
			return nil
		}
	}

	return nil
}

// Write implements `io.Writer` interface.
func (trap *TrapWriter) Write(d []byte) (int, error) {
	msg := make([]byte, len(d))
	copy(msg, d)

	trap.msgs = append(trap.msgs, msg)

	return len(msg), nil
}
