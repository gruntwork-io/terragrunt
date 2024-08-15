package runall

import "fmt"

// DisabledError is an error type that is returned when a run-all command is disabled.
type DisabledError struct {
	command string
	reason  string
}

// Error returns the error message as a string.
func (err DisabledError) Error() string {
	return fmt.Sprintf("%s with run-all is disabled: %s", err.command, err.reason)
}

// MissingCommandError is an error type that is returned when a run-all command is missing.
type MissingCommandError struct{}

// Error returns the error message as a string.
func (err MissingCommandError) Error() string {
	return "Missing run-all command argument (Example: terragrunt run-all plan)"
}
