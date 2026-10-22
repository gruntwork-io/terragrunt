package runner

import (
	"errors"
	"fmt"

	"github.com/gruntwork-io/terragrunt/internal/queue"
)

// ErrRunnerNotSet is returned when a [Controller] is run without a [UnitRunnerFunc].
var ErrRunnerNotSet = errors.New("runner pool controller: runner is not set, cannot run")

// UnitEarlyExitError reports a unit that never ran because a dependency failed.
type UnitEarlyExitError struct {
	UnitPath         string
	FailedDependency string // The dependency that caused the early exit (optional)
}

func (e UnitEarlyExitError) Error() string {
	if e.FailedDependency != "" {
		return fmt.Sprintf("Unit '%s' did not run due to a failure in '%s'",
			e.UnitPath, e.FailedDependency)
	}

	return fmt.Sprintf("Unit '%s' did not run due to an earlier failure", e.UnitPath)
}

// NewUnitEarlyExitError creates a new UnitEarlyExitError.
func NewUnitEarlyExitError(unitPath, failedDep string) error {
	return UnitEarlyExitError{
		UnitPath:         unitPath,
		FailedDependency: failedDep,
	}
}

// UnitNotDiscoveredError reports a queue entry with no matching discovered unit.
type UnitNotDiscoveredError struct {
	UnitPath string
}

func (e UnitNotDiscoveredError) Error() string {
	return fmt.Sprintf("unit for path %s not found in discovered units", e.UnitPath)
}

// NewUnitNotDiscoveredError creates a new UnitNotDiscoveredError.
func NewUnitNotDiscoveredError(unitPath string) error {
	return UnitNotDiscoveredError{UnitPath: unitPath}
}

// UnitNotParsedError reports a unit that reached the runner without the configuration discovery parses for every
// unit. This indicates a bug in Terragrunt.
type UnitNotParsedError struct {
	UnitPath string
}

func (e UnitNotParsedError) Error() string {
	return fmt.Sprintf(
		"Unit at path '%s' reached the runner without a parsed configuration. "+
			"This is a bug in Terragrunt. "+
			"Please open a bug report at https://github.com/gruntwork-io/terragrunt/issues "+
			"with details about how you encountered this error.",
		e.UnitPath,
	)
}

// NewUnitNotParsedError creates a new UnitNotParsedError.
func NewUnitNotParsedError(unitPath string) error {
	return UnitNotParsedError{UnitPath: unitPath}
}

// UnitFailedError reports a unit that failed during execution.
type UnitFailedError struct {
	UnitPath string
}

func (e UnitFailedError) Error() string {
	return fmt.Sprintf("Unit '%s' encountered an error during its run", e.UnitPath)
}

// NewUnitFailedError creates a new UnitFailedError.
func NewUnitFailedError(unitPath string) error {
	return UnitFailedError{UnitPath: unitPath}
}

// findFailedDependency finds the first failed dependency for a given entry.
func findFailedDependency(entry *queue.Entry, q *queue.Queue) string {
	for _, dep := range entry.Component.Dependencies() {
		for _, e := range q.Entries {
			if e.Component.Path() == dep.Path() {
				if e.Status == queue.StatusFailed {
					return dep.Path()
				}
			}
		}
	}

	return ""
}
