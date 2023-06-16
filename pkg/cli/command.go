package cli

import (
	libflag "flag"

	"github.com/urfave/cli/v2"
)

type Command struct {
	// The name of the cmd
	Name string
	// A list of aliases for the cmd
	Aliases []string
	// A short description of the usage of this cmd
	Usage string
	// Custom text to show on USAGE section of help
	UsageText string
	// A longer explanation of how the cmd works
	Description string
	// List of flags to parse
	Flags Flags
	// Full name of cmd for help, defaults to full cmd name, including parent commands.
	HelpName string
	// if this is a root "special" cmd
	IsRoot bool
	// Boolean to hide this cmd from help
	Hidden bool
	// CustomHelpTemplate the text template for the cmd help topic.
	// cli.go uses text/template to render templates. You can
	// render custom help text by setting this variable.
	CustomHelpTemplate string
	// List of child commands
	Subcommands Commands
	// Treat all flags as normal arguments if true
	SkipFlagParsing bool
	// Boolean to disable the parsing command, but it will still be shown in the help.
	SkipRun bool
	// An action to execute before any subcommands are run, but after the context is ready
	// If a non-nil error is returned, no subcommands are run
	Before ActionFunc
	// An action to execute after any subcommands are run, but after the subcommand has finished
	After ActionFunc
	// The action to execute when no subcommands are specified
	Action ActionFunc
}

// Names returns the names including short names and aliases.
func (cmd *Command) Names() []string {
	return append([]string{cmd.Name}, cmd.Aliases...)
}

// HasName returns true if Command.Name matches given name
func (cmd *Command) HasName(name string) bool {
	for _, n := range cmd.Names() {
		if n == name {
			return true
		}
	}
	return false
}

func (cmd *Command) Subcommand(name string) *Command {
	for _, c := range cmd.Subcommands {
		if c.HasName(name) {
			return c
		}
	}

	return nil
}

// VisibleFlags returns a slice of the Flags, used by `urfave/cli` package to generate help.
func (cmd *Command) VisibleFlags() Flags {
	return cmd.Flags
}

// VisibleCommands returns a slice of the Commands with Hidden=false.
// Used by `urfave/cli` package to generate help.
func (cmd Command) VisibleCommands() []*cli.Command {
	if cmd.Subcommands == nil {
		return nil
	}
	return cmd.Subcommands.VisibleCommands()
}

func (cmd *Command) Run(ctx *Context, args []string) error {
	args, err := cmd.parseFlags(args)
	if err != nil {
		return err
	}

	ctx = ctx.Clone(cmd, args)

	subCmdName := ctx.Args().CommandName()
	if subCmd := cmd.Subcommand(subCmdName); subCmd != nil && !subCmd.SkipRun {
		args := ctx.Args().Tail()
		err := subCmd.Run(ctx, args)
		return err
	}

	if cmd.IsRoot && ctx.App.DefaultCommand != nil {
		err := ctx.App.DefaultCommand.Run(ctx, args)
		return err
	}

	err = cmd.runAction(ctx)
	return err
}

func (cmd *Command) parseFlags(args []string) ([]string, error) {
	var undefArgs []string

	flagSet, err := cmd.Flags.newFlagSet(cmd.Name, libflag.ContinueOnError)
	if err != nil {
		return nil, err
	}

	if cmd.SkipFlagParsing {
		return args, nil
	}

	for {
		args, err = cmd.Flags.parseFlags(flagSet, args)
		if err != nil {
			return nil, err
		}

		if len(args) == 0 {
			break
		}

		undefArgs = append(undefArgs, args[0])
		args = args[1:]
	}

	return undefArgs, nil
}

func (cmd *Command) runAction(ctx *Context) error {
	if cmd.Before != nil {
		if err := cmd.Before(ctx); err != nil {
			return err
		}
	}

	if cmd.Action != nil {
		if err := cmd.Action(ctx); err != nil {
			return err
		}
	}

	if cmd.After != nil {
		if err := cmd.After(ctx); err != nil {
			return err
		}
	}

	return nil
}
