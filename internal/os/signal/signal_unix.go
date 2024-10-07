//go:build !windows
// +build !windows

package signal

import (
	"os"
	"syscall"
)

// InterruptSignal is an interrupt signal.
var InterruptSignal = syscall.SIGINT //nolint:gochecknoglobals

// InterruptSignals contains a list of signals that are treated as interrupts.
var InterruptSignals = []os.Signal{syscall.SIGTERM, syscall.SIGINT} //nolint:gochecknoglobals
