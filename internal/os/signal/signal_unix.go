//go:build !windows
// +build !windows

package signal

import (
	"os"
	"syscall"
)

// InterruptSignal is an interrupt signal.
const InterruptSignal = syscall.SIGINT

// InterruptSignals contains a list of signals that are treated as interrupts.
var InterruptSignals []os.Signal = []os.Signal{syscall.SIGTERM, syscall.SIGINT}
