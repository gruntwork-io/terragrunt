package tf

import (
	"sync"
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
func (coder *DetailedExitCode) Set(newCode int) {
	coder.mu.Lock()
	defer coder.mu.Unlock()

	coder.Code = newCode
}
