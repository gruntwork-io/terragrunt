//go:build !windows
// +build !windows

package shell

import (
	"os"
	"syscall"
)

var InterruptSignals []os.Signal = []os.Signal{syscall.SIGTERM, syscall.SIGINT}
