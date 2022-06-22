//go:build !windows
// +build !windows

package shell

import (
	"os"
	"syscall"
)

var forwardSignals []os.Signal = []os.Signal{syscall.SIGTERM, syscall.SIGINT}
