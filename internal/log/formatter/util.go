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

func CheckIfTerminal(w io.Writer) bool {
	switch v := w.(type) {
	case *os.File:
		return term.IsTerminal(int(v.Fd()))
	default:
		return false
	}
}
