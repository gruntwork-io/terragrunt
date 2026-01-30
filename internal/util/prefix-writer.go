package util

import (
	"bytes"
	"io"
)

func PrefixedWriter(writer io.Writer, prefix string) io.Writer {
	return &prefixedWriter{writer: writer, prefix: prefix, beginningOfANewLine: true}
}

func (pf *prefixedWriter) Write(p []byte) (int, error) {
	buf := bytes.Buffer{}

	for _, b := range p {
		if pf.beginningOfANewLine {
			buf.WriteString(pf.prefix)
			pf.beginningOfANewLine = false
		}

		buf.WriteByte(b)

		pf.beginningOfANewLine = b == '\n'
	}

	n, err := pf.writer.Write(buf.Bytes())
	if n > len(p) {
		n = len(p)
	}

	return n, err
}

type prefixedWriter struct {
	writer              io.Writer
	prefix              string
	beginningOfANewLine bool
}
