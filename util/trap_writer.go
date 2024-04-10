package util

import (
	"io"
	"regexp"
)

const ansi = "[\u001B\u009B][[\\]()#;?]*(?:(?:(?:[a-zA-Z\\d]*(?:;[a-zA-Z\\d]*)*)?\u0007)|(?:(?:\\d{1,4}(?:;\\d{0,4})*)?[\\dA-PRZcf-ntqry=><~]))"

// regexp matches ansi characters getting from a shell output, used for colors etc.
var ansiReg = regexp.MustCompile(ansi)

// TrapWriter intercepts any messages matching `reg` received from the `writer` output, but passes all others.
// Used when necessary to filter logs from terraform.
type TrapWriter struct {
	writer      io.Writer
	reg         *regexp.Regexp
	trappedMsgs []string
}

// NewTrapWriter returns a new TrapWriter instance.
func NewTrapWriter(writer io.Writer, reg *regexp.Regexp) *TrapWriter {
	return &TrapWriter{
		writer: writer,
		reg:    reg,
	}
}

// Msgs returns the intercepted messages.
func (trap *TrapWriter) Msgs() []string {
	return trap.trappedMsgs
}

// Clear clears all intercepted messages.
func (trap *TrapWriter) Clear() {
	trap.trappedMsgs = nil
}

// Write implements `io.Writer` interface.
func (trap *TrapWriter) Write(msg []byte) (int, error) {
	msgWithoutAnsi := ansiReg.ReplaceAll(msg, []byte(""))

	if trap.reg.Match(msgWithoutAnsi) {
		trap.trappedMsgs = append(trap.trappedMsgs, string(msg))
		return len(msg), nil
	}

	return trap.writer.Write(msg)
}
