package runnerpool

import (
	"fmt"

	"github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/gruntwork-io/terragrunt/internal/queue"
)

// UnitEarlyExitError is an error type for units that didn't run due to dependency failure.
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
	return errors.New(UnitEarlyExitError{
		UnitPath:         unitPath,
		FailedDependency: failedDep,
	})
}

// UnitFailedError is an error type for units that failed during execution.
type UnitFailedError struct {
	UnitPath string
}

func (e UnitFailedError) Error() string {
	return fmt.Sprintf("Unit '%s' encountered an error during its run", e.UnitPath)
}

// NewUnitFailedError creates a new UnitFailedError.
func NewUnitFailedError(unitPath string) error {
	return errors.New(UnitFailedError{UnitPath: unitPath})
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
