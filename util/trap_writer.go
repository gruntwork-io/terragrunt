package util

import (
	"io"
	"regexp"
)

const ansi = "[\u001B\u009B][[\\]()#;?]*(?:(?:(?:[a-zA-Z\\d]*(?:;[a-zA-Z\\d]*)*)?\u0007)|(?:(?:\\d{1,4}(?:;\\d{0,4})*)?[\\dA-PRZcf-ntqry=><~]))"

var ansiReg = regexp.MustCompile(ansi)

type TrapWriter struct {
	writer      io.Writer
	reg         *regexp.Regexp
	trappedMsgs []string
}

func NewTrapWriter(writer io.Writer, reg *regexp.Regexp) *TrapWriter {
	return &TrapWriter{
		writer: writer,
		reg:    reg,
	}
}

func (trap *TrapWriter) Msgs() []string {
	return trap.trappedMsgs
}

func (trap *TrapWriter) Clear() {
	trap.trappedMsgs = nil
}

func (trap *TrapWriter) Write(msg []byte) (int, error) {
	msgWithoutAnsi := ansiReg.ReplaceAll(msg, []byte(""))

	if trap.reg.Match(msgWithoutAnsi) {
		trap.trappedMsgs = append(trap.trappedMsgs, string(msg))
		return len(msg), nil
	}

	return trap.writer.Write(msg)
}
