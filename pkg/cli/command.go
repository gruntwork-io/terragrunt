package cli

import (
	"flag"
)

type Command struct {
	// The name of the command
	Name string
	// A list of aliases for the command
	Aliases []string
	// A short description of the usage of this command
	Usage string
	// Custom text to show on USAGE section of help
	UsageText string
	// A longer explanation of how the command works
	Description string
	// List of flags to parse
	Flags Flags
	// Full name of command for help, defaults to full command name, including parent commands.
	HelpName string
	// The function to call when this command is invoked
	Action RunFunc
	// if this is a root "special" command
	IsRoot bool
	// Boolean to hide this command from help
	Hidden bool
}

// AddFlags adds new flags.
func (command *Command) AddFlags(flags ...Flag) {
	command.Flags = append(command.Flags, flags...)
}

// Names returns the names including short names and aliases.
func (command *Command) Names() []string {
	return append([]string{command.Name}, command.Aliases...)
}

// HasName returns true if Command.Name matches given name
func (command *Command) HasName(name string) bool {
	for _, n := range command.Names() {
		if n == name {
			return true
		}
	}
	return false
}

// VisibleFlags returns a slice of the Flags, used by `urfave/cli` package to generate help.
func (command *Command) VisibleFlags() Flags {
	return command.Flags
}

func (command *Command) parseArgs(args []string) ([]string, error) {
	var undefined []string

	flagSet, err := command.Flags.newFlagSet(command.Name, flag.ContinueOnError)
	if err != nil {
		return nil, err
	}

	for {
		args, err = command.Flags.parseArgs(flagSet, args)
		if err != nil {
			return nil, err
		}

		if len(args) == 0 {
			break
		}

		undefined = append(undefined, args[0])
		args = args[1:]
	}

	return undefined, nil
}
