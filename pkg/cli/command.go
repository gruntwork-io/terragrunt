package cli

import (
	libflag "flag"
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
	// if this is a root "special" command
	IsRoot bool
	// Boolean to hide this command from help
	Hidden bool
	// CustomHelpTemplate the text template for the command help topic.
	// cli.go uses text/template to render templates. You can
	// render custom help text by setting this variable.
	CustomHelpTemplate string
	// List of child commands
	Subcommands Commands
	// An action to execute before any subcommands are run, but after the context is ready
	// If a non-nil error is returned, no subcommands are run
	Before RunFunc
	// An action to execute after any subcommands are run, but after the subcommand has finished
	After RunFunc
	// The action to execute when no subcommands are specified
	Action RunFunc
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

func (command *Command) parseArgs(args []string) (*Command, []string, error) {
	var undefArgs []string
	var undefArg string

	flagSet, err := command.Flags.newFlagSet(command.Name, libflag.ContinueOnError)
	if err != nil {
		return nil, nil, err
	}

	for {
		args, err = command.Flags.parseArgs(flagSet, args)
		if err != nil {
			return nil, nil, err
		}

		if len(args) == 0 {
			break
		}

		undefArg, args = args[0], args[1:]
		undefArgs = append(undefArgs, undefArg)
	}

	if command, undefArgs, err := command.Subcommands.parseArgs(undefArgs, false); command != nil || err != nil {
		return command, undefArgs, err
	}

	return command, undefArgs, nil
}

func (command *Command) Run(ctx *Context) error {
	if command.Before != nil {
		if err := command.Before(ctx); err != nil {
			return err
		}
	}

	if command.Action != nil {
		if err := command.Action(ctx); err != nil {
			return err
		}
	}

	if command.After != nil {
		if err := command.After(ctx); err != nil {
			return err
		}
	}

	return nil
}
