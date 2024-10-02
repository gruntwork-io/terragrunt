//go:build windows
// +build windows

package signal

import (
	"os"
)

// InterruptSignal is an interrupt signal.
const InterruptSignal os.Signal = os.Signal{}

// InterruptSignals contains a list of signals that are treated as interrupts.
var InterruptSignals []os.Signal = []os.Signal{}
