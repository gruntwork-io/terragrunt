package controls

import (
	"fmt"

	"github.com/gruntwork-io/terragrunt/internal/errors"
)

const (
	DefaultCommandsCategoryName     = "Default commands"
	RunAllCommandsCategoryName      = "`*-all` commands"
	CLIRedesignCommandsCategoryName = "CLI redesign commands"
)

// NewDeprecatedReplacedCommand declares the deprecated command that has an alternative command.
func NewDeprecatedReplacedCommand(command, newCommand string) *Control {
	return &Control{
		Name:        command,
		Description: "replaced with: " + newCommand,
		Error:       errors.Errorf("The `%s` command is no longer supported. Use `%s` instead.", command, newCommand),
		Warning:     fmt.Sprintf("The `%s` command is deprecated and will be removed in a future version of Terragrunt. Use `%s` instead.", command, newCommand),
	}
}

// NewDeprecatedCommand declares the deprecated command.
func NewDeprecatedCommand(command string) *Control {
	return &Control{
		Name:        command,
		Description: "no replaced command",
		Error:       errors.Errorf("The `%s` command is no longer supported.", command),
		Warning:     fmt.Sprintf("The `%s` command is deprecated and will be removed in a future version of Terragrunt.", command),
	}
}
