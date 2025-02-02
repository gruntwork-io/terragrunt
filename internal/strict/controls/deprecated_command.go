package controls

import (
	"fmt"

	"github.com/gruntwork-io/terragrunt/internal/errors"
)

const (
	DefaultCommandsCategoryName = "Default commands"
	RunAllCommandsCategoryName  = "`*-all` commands"
)

// NewDeprecatedCommand declares the deprecated command.
func NewDeprecatedCommand(command, newCommand string) *Control {
	return &Control{
		Name:        command,
		Description: "replaced with: " + newCommand,
		Error:       errors.Errorf("`%s` commands is no longer supported. Use `%s` instead.", command, newCommand),
		Warning:     fmt.Sprintf("`%s` commands is deprecated and will be removed in a future version. Use `%s` instead.", command, newCommand),
	}
}
