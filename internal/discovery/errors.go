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
			"Git-based filtering can only be used with 'plan', 'apply', or discovery commands (like 'find' or 'list') that don't require additional arguments.",
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
