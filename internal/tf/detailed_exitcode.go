package tf

import (
	"sync"
)

const (
	DetailedExitCodeSuccess = 0
	DetailedExitCodeError   = 1
	DetailedExitCodeChanges = 2
)

// DetailedExitCodeMap stores exit codes per unit path. https://opentofu.org/docs/cli/commands/plan/
type DetailedExitCodeMap struct {
	codes map[string]int
	mu    sync.RWMutex
}

// NewDetailedExitCodeMap creates a new DetailedExitCodeMap.
func NewDetailedExitCodeMap() *DetailedExitCodeMap {
	return &DetailedExitCodeMap{
		codes: make(map[string]int),
	}
}

// Set stores the exit code for the given path. Always updates the map without conditional logic.
func (m *DetailedExitCodeMap) Set(path string, code int) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.codes == nil {
		m.codes = make(map[string]int)
	}

	m.codes[path] = code
}

// Get returns the exit code for the given path, or 0 if not found.
func (m *DetailedExitCodeMap) Get(path string) int {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.codes == nil {
		return 0
	}

	return m.codes[path]
}

// GetFinalDetailedExitCode computes the final exit code following OpenTofu's exit code convention:
// - 0 = Success
// - 1 = Error
// - 2 = Success with changes pending
// Aggregation rules for run --all:
// - If any exit code is 1 (or > 2), return the max exit code
// - If all exit codes are 0 or 2, return 2
// - If all exit codes are 0, return 0
func (m *DetailedExitCodeMap) GetFinalDetailedExitCode() int {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if len(m.codes) == 0 {
		return 0
	}

	hasError := false
	hasChanges := false
	maxCode := 0

	for _, code := range m.codes {
		if code == DetailedExitCodeError || code > DetailedExitCodeChanges {
			hasError = true

			maxCode = max(maxCode, code)

			continue
		}

		if code == DetailedExitCodeChanges {
			hasChanges = true
		}
	}

	if hasError {
		return maxCode
	}

	if hasChanges {
		return DetailedExitCodeChanges
	}

	return DetailedExitCodeSuccess
}

// GetFinalExitCode computes the final exit code assuming the user hasn't supplied the -detailed-exitcode flag.
//
// In this case, we only care about any non-zero exit codes, so we'll return the highest exit code we can find.
func (m *DetailedExitCodeMap) GetFinalExitCode() int {
	m.mu.RLock()
	defer m.mu.RUnlock()

	maxCode := 0
	for _, code := range m.codes {
		maxCode = max(maxCode, code)
	}

	return maxCode
}

// ResetSuccess clears all exit codes from the map.
func (m *DetailedExitCodeMap) ResetSuccess() {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.codes = make(map[string]int)
}
