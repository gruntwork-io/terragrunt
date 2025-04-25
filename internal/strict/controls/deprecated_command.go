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

// NewDeprecatedCommand declares the deprecated command.
func NewDeprecatedCommand(command, newCommand string) *Control {
	return &Control{
		Name:        command,
		Description: "replaced with: " + newCommand,
		Error:       errors.Errorf("The `%s` command is no longer supported. Use `%s` instead.", command, newCommand),
		Warning:     fmt.Sprintf("The `%s` command is deprecated and will be removed in a future version of Terragrunt. Use `%s` instead.", command, newCommand),
	}
}
