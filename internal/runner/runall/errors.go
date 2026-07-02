package runall

import (
	"fmt"

	"errors"
)

type RunAllDisabledErr struct {
	command string
	reason  string
}

func (err RunAllDisabledErr) Error() string {
	return fmt.Sprintf("%s with run --all is disabled: %s", err.command, err.reason)
}

type MissingCommand struct{}

func (err MissingCommand) Error() string {
	return "Missing run --all command argument (Example: terragrunt run --all plan)"
}

// ErrUserCancelled signals that the user answered "no" to a destructive
// `run --all` confirmation prompt. The top-level error handler treats
// it as a clean exit.
var ErrUserCancelled = errors.New("run --all cancelled by user")
