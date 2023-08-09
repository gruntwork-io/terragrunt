package runall

import "fmt"

type RunAllDisabledErr struct {
	command string
	reason  string
}

func (err RunAllDisabledErr) Error() string {
	return fmt.Sprintf("%s with run-all is disabled: %s", err.command, err.reason)
}

type MissingCommand struct{}

func (err MissingCommand) Error() string {
	return "Missing run-all command argument (Example: terragrunt run-all plan)"
}
