package shell

import (
	"sync"
)

const (
	DetailedExitCodeError = 1
)

// DetailedExitCode is the TF detailed exit code. https://opentofu.org/docs/cli/commands/plan/
type DetailedExitCode struct {
	Code int
	mu   sync.RWMutex
}

// Get returns exit code.
func (coder *DetailedExitCode) Get() int {
	coder.mu.RLock()
	defer coder.mu.RUnlock()

	return coder.Code
}

// Set sets the newCode if the previous value is not 1 and the new value is greater than the previous one.
func (coder *DetailedExitCode) Set(newCode int) {
	coder.mu.Lock()
	defer coder.mu.Unlock()

	if coder.Code == DetailedExitCodeError {
		return
	}

	if coder.Code < newCode || newCode == DetailedExitCodeError {
		coder.Code = newCode
	}
}
