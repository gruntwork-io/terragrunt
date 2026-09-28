package cli

import "fmt"

// UnknownCommandError is returned when the first non-flag argument names no
// command. The message points at `terragrunt run --`, the explicit form for
// handing a command to the wrapped binary.
type UnknownCommandError string

func (err UnknownCommandError) Error() string {
	return fmt.Sprintf(
		"unknown command: %[1]q. Terragrunt no longer forwards unknown commands by default."+
			" Use 'terragrunt run -- %[1]s ...' or a supported shortcut."+
			" Learn more: https://docs.terragrunt.com/migrate/cli-redesign/#use-the-new-run-command",
		string(err),
	)
}
