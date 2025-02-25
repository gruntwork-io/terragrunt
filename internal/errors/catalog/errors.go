// Package catalog provides error types specific to the catalog functionality
package catalog

import (
	"fmt"
)

// ErrModuleNotFound is returned when a module cannot be found in a repository
type ErrModuleNotFound struct {
	RepoURL    string
	ModulePath string
}

func (e ErrModuleNotFound) Error() string {
	return fmt.Sprintf("module not found at path '%s' in repository '%s'", e.ModulePath, e.RepoURL)
}

// ErrRepositoryNotFound is returned when a repository cannot be found or accessed
type ErrRepositoryNotFound struct {
	RepoURL string
	Cause   error
}

func (e ErrRepositoryNotFound) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("repository not found or cannot be accessed: %s (cause: %s)", e.RepoURL, e.Cause)
	}

	return "repository not found or cannot be accessed: " + e.RepoURL
}

// ErrInvalidURL is returned when a URL cannot be parsed
type ErrInvalidURL struct {
	URL   string
	Cause error
}

func (e ErrInvalidURL) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("invalid URL format: %s (cause: %s)", e.URL, e.Cause)
	}

	return "invalid URL format: " + e.URL
}

// ErrExperimentRequired is returned when an operation requires an experiment that is not enabled
type ErrExperimentRequired struct {
	ExperimentName string
	Operation      string
}

func (e ErrExperimentRequired) Error() string {
	return fmt.Sprintf("operation '%s' requires the %s experiment to be enabled", e.Operation, e.ExperimentName)
}

// ErrCloneFailure is returned when a repository clone operation fails
type ErrCloneFailure struct {
	RepoURL string
	Cause   error
}

func (e ErrCloneFailure) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("failed to clone repository '%s': %s", e.RepoURL, e.Cause)
	}

	return "failed to clone repository '" + e.RepoURL + "'"
}

// ErrScaffoldFailure is returned when scaffolding a module fails
type ErrScaffoldFailure struct {
	ModulePath string
	TargetDir  string
	Cause      error
}

func (e ErrScaffoldFailure) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("failed to scaffold module '%s' to '%s': %s", e.ModulePath, e.TargetDir, e.Cause)
	}

	return "failed to scaffold module '" + e.ModulePath + "' to '" + e.TargetDir + "'"
}

// Wrap wraps the given error with additional context
func Wrap(err error, format string, args ...interface{}) error {
	if err == nil {
		return nil
	}

	return fmt.Errorf(format+": %w", append(args, err)...)
}
