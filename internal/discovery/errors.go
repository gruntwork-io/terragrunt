package discovery

import (
	"fmt"
	"strings"

	"github.com/gruntwork-io/terragrunt/internal/errors"
)

// GitFilterCommandError represents an error that occurs when attempting to use
// Git-based filtering with an unsupported command.
type GitFilterCommandError struct {
	Cmd  string
	Args []string
}

func (e GitFilterCommandError) Error() string {
	command := strings.TrimSpace(
		strings.Join(
			append(
				[]string{e.Cmd},
				e.Args...,
			),
			" ",
		),
	)

	return fmt.Sprintf(
		"Git-based filtering is not supported with the command '%s'. "+
			"Git-based filtering can only be used with 'plan', 'apply', "+
			"or discovery commands (like 'find' or 'list') that don't require additional arguments.",
		command,
	)
}

// NewGitFilterCommandError creates a new GitFilterCommandError with the given command and arguments.
func NewGitFilterCommandError(cmd string, args []string) error {
	return errors.New(GitFilterCommandError{
		Cmd:  cmd,
		Args: args,
	})
}

// MissingDiscoveryContextError represents an error that occurs when a component
// is missing its discovery context during dependency discovery. This indicates
// a bug in Terragrunt.
type MissingDiscoveryContextError struct {
	ComponentPath string
}

func (e MissingDiscoveryContextError) Error() string {
	return fmt.Sprintf(
		"Component at path '%s' is missing its discovery context during dependency discovery. "+
			"This is a bug in Terragrunt. "+
			"Please open a bug report at https://github.com/gruntwork-io/terragrunt/issues "+
			"with details about how you encountered this error.",
		e.ComponentPath,
	)
}

// NewMissingDiscoveryContextError creates a new MissingDiscoveryContextError for the given component path.
func NewMissingDiscoveryContextError(componentPath string) error {
	return errors.New(MissingDiscoveryContextError{
		ComponentPath: componentPath,
	})
}

// MissingWorkingDirectoryError represents an error that occurs when a component's
// discovery context is missing its working directory during dependency discovery.
// This indicates a bug in Terragrunt.
type MissingWorkingDirectoryError struct {
	ComponentPath string
}

func (e MissingWorkingDirectoryError) Error() string {
	return fmt.Sprintf(
		"Component at path '%s' has a discovery context but is missing its working directory during dependency discovery. "+
			"This is a bug in Terragrunt. "+
			"Please open a bug report at https://github.com/gruntwork-io/terragrunt/issues "+
			"with details about how you encountered this error.",
		e.ComponentPath,
	)
}

// NewMissingWorkingDirectoryError creates a new MissingWorkingDirectoryError for the given component path.
func NewMissingWorkingDirectoryError(componentPath string) error {
	return errors.New(MissingWorkingDirectoryError{
		ComponentPath: componentPath,
	})
}

// MaxDepthReachedError represents an error that occurs when the maximum
// discovery depth is reached during traversal.
type MaxDepthReachedError struct {
	CurrentPath string
	Phase       PhaseKind
	MaxDepth    int
}

func (e MaxDepthReachedError) Error() string {
	return fmt.Sprintf(
		"Maximum depth of %d reached during %s phase while processing '%s'. "+
			"Consider increasing the max depth setting or checking for circular references.",
		e.MaxDepth, e.Phase.String(), e.CurrentPath,
	)
}

// NewMaxDepthReachedError creates a new MaxDepthReachedError.
func NewMaxDepthReachedError(phase PhaseKind, maxDepth int, currentPath string) error {
	return errors.New(MaxDepthReachedError{
		Phase:       phase,
		MaxDepth:    maxDepth,
		CurrentPath: currentPath,
	})
}

// ClassificationError represents an error during component classification.
type ClassificationError struct {
	ComponentPath string
	Reason        string
}

func (e ClassificationError) Error() string {
	return fmt.Sprintf(
		"Failed to classify component at '%s': %s",
		e.ComponentPath, e.Reason,
	)
}

// NewClassificationError creates a new ClassificationError.
func NewClassificationError(componentPath, reason string) error {
	return errors.New(ClassificationError{
		ComponentPath: componentPath,
		Reason:        reason,
	})
}

// PhaseError wraps an error that occurred during a specific discovery phase.
type PhaseError struct {
	Err   error
	Phase PhaseKind
}

func (e PhaseError) Error() string {
	return fmt.Sprintf("error during %s phase: %v", e.Phase.String(), e.Err)
}

func (e PhaseError) Unwrap() error {
	return e.Err
}

// NewPhaseError creates a new PhaseError.
func NewPhaseError(phase PhaseKind, err error) error {
	return errors.New(PhaseError{
		Phase: phase,
		Err:   err,
	})
}
