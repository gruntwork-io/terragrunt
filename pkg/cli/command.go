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
	SkipRunning bool
	// The function to call when checking for command completions
	Complete CompleteFunc
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
		if n == name && name != "" {
			return true
		}
	}
	return false
}

// Subcommand returns a subcommand that matches the given name.
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

// VisibleSubCommands returns a slice of the Commands with Hidden=false.
// Used by `urfave/cli` package to generate help.
func (cmd Command) VisibleSubcommands() []*cli.Command {
	if cmd.Subcommands == nil {
		return nil
	}
	return cmd.Subcommands.VisibleCommands()
}

// Run parses the given args for the presence of flags as well as subcommands.
// If this is the final command, starts its execution.
func (cmd *Command) Run(ctx *Context, args Args) (err error) {
	args, err = cmd.parseFlags(args.Slice())
	if err != nil {
		return err
	}

	ctx = ctx.Clone(cmd, args)

	subCmdName := ctx.Args().CommandName()
	subCmdArgs := ctx.Args().Tail()
	subCmd := cmd.Subcommand(subCmdName)

	if ctx.shellComplete {
		if cmd := ctx.Command.Subcommand(args.CommandName()); cmd == nil {
			return ShowCompletions(ctx)
		}

		if subCmd != nil {
			return subCmd.Run(ctx, subCmdArgs)
		}
	}

	if err := cmd.Flags.RunActions(ctx); err != nil {
		return ctx.App.handleExitCoder(err)
	}

	defer func() {
		if cmd.After != nil && err == nil {
			err = cmd.After(ctx)
			err = ctx.App.handleExitCoder(err)
		}
	}()

	if cmd.Before != nil {
		if err := cmd.Before(ctx); err != nil {
			return ctx.App.handleExitCoder(err)
		}
	}

	if subCmd != nil && !subCmd.SkipRunning {
		return subCmd.Run(ctx, subCmdArgs)
	}

	if cmd.IsRoot && ctx.App.DefaultCommand != nil {
		err = ctx.App.DefaultCommand.Run(ctx, args)
		return err
	}

	if ctx.App.CommonBefore != nil {
		if err = ctx.App.CommonBefore(ctx); err != nil {
			return ctx.App.handleExitCoder(err)
		}
	}

	if cmd.Action != nil {
		if err = cmd.Action(ctx); err != nil {
			return ctx.App.handleExitCoder(err)
		}
	}

	return nil
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
