//go:build windows
// +build windows

package signal

import (
	"os"
)

// InterruptSignal is an interrupt signal.
var InterruptSignal os.Signal = nil

// InterruptSignals contains a list of signals that are treated as interrupts.
var InterruptSignals []os.Signal = []os.Signal{}
