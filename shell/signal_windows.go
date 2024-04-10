//go:build windows
// +build windows

package shell

import (
	"os"
)

var InterruptSignals []os.Signal = []os.Signal{}
