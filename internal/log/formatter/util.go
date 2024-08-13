package formatter

import (
	"io"
	"os"
	"time"

	"golang.org/x/term"
)

var baseTimestamp time.Time = time.Now()

func miniTS() int {
	return int(time.Since(baseTimestamp) / time.Second)
}

// Whether the logger's out is to a terminal.
var isTerminal bool

func init() {
	isTerminal = checkIfTerminal(os.Stderr)
}

func checkIfTerminal(w io.Writer) bool {
	switch v := w.(type) {
	case *os.File:
		return term.IsTerminal(int(v.Fd()))
	default:
		return false
	}
}
