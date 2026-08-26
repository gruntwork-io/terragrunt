//go:build windows

package signal

import (
	"os"
)

// InterruptSignal is the signal sent to ask a process to stop. Windows has no interrupt
// one process can deliver to another, so termination is the only thing that lands.
var InterruptSignal os.Signal = os.Kill //nolint:gochecknoglobals

// InterruptSignals contains a list of signals that are treated as interrupts.
var InterruptSignals []os.Signal = []os.Signal{}
