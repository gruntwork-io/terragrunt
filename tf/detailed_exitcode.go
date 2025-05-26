package tf

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

// Set updates the exit code following OpenTofu's exit code convention:
// - 0 = Success
// - 1 = Error
// - 2 = Success with changes pending
// The method only updates if:
// - The current code is not 1 (error state)
// - The new code is greater than current OR equals 1
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
