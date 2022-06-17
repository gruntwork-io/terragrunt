//go:build windows
// +build windows

package shell

import (
	"os"
)

var forwardSignals []os.Signal = []os.Signal{}
