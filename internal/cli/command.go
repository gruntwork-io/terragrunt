package cli

import (
	"errors"
	libflag "flag"
	"strings"
)

type Command struct {
	// Name is the command name.
	Name string
	// Aliases is a list of aliases for the command.
	Aliases []string
	// Usage is a short description of the usage of the command.
	Usage string
	// UsageText is custom text to show on the `Usage` section of help.
	UsageText string
	// Description is a longer explanation of how the command works.
	Description string
	// Examples is list of examples of using the command in the help.
	Examples []string
	// Category is the category the command belongs to.
	Category *Category
	// Flags is list of flags to parse.
	Flags Flags
	// ErrorOnUndefinedFlag causes the application to exit and return an error on any undefined flag.
	ErrorOnUndefinedFlag bool
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
	// CustomHelp is a func that is executed when help for a command needs to be displayed.
	CustomHelp HelpFunc
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

// VisibleSubcommands returns a slice of the Commands with Hidden=false.
// Used by `urfave/cli` package to generate help.
func (cmd *Command) VisibleSubcommands() Commands {
	if cmd.Subcommands == nil {
		return nil
	}

	return cmd.Subcommands.VisibleCommands()
}

// Run parses the given args for the presence of flags as well as subcommands.
// If this is the final command, starts its execution.
func (cmd *Command) Run(ctx *Context, args Args) (err error) {
	args, err = cmd.parseFlags(ctx, args.Slice())
	ctx = ctx.NewCommandContext(cmd, args)

	if err != nil {
		if flagErrHandler := ctx.App.FlagErrHandler; flagErrHandler != nil {
			err = flagErrHandler(ctx, err)
		}

		return NewExitError(err, ExitCodeGeneralError)
	}

	subCmdName := ctx.Args().CommandName()
	subCmdArgs := ctx.Args().Remove(subCmdName)
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
		return ctx.App.handleExitCoder(ctx, err)
	}

	defer func() {
		if cmd.After != nil && err == nil {
			err = cmd.After(ctx)
			err = ctx.App.handleExitCoder(ctx, err)
		}
	}()

	if cmd.Before != nil {
		if err := cmd.Before(ctx); err != nil {
			return ctx.App.handleExitCoder(ctx, err)
		}
	}

	if subCmd != nil && !subCmd.SkipRunning {
		return subCmd.Run(ctx, subCmdArgs)
	}

	if cmd.Action != nil {
		if err = cmd.Action(ctx); err != nil {
			return ctx.App.handleExitCoder(ctx, err)
		}
	}

	return nil
}

func (cmd *Command) parseFlags(ctx *Context, args Args) ([]string, error) {
	var undefArgs Args

	flagSet, err := cmd.Flags.NewFlagSet(cmd.Name)
	if err != nil {
		return nil, err
	}

	flagSetWithSubcommandScope, err := cmd.Flags.WithSubcommandScope().NewFlagSet(cmd.Name)
	if err != nil {
		return nil, err
	}

	if cmd.SkipFlagParsing {
		return args, nil
	}

	args, builtinCmd := args.Split(BuiltinCmdSep)

	for i := 0; ; i++ {
		if i == 0 {
			args, err = cmd.flagSetParse(ctx, flagSet, args)
		} else {
			args, err = cmd.flagSetParse(ctx, flagSetWithSubcommandScope, args)
		}

		if err != nil {
			if !errors.As(err, new(UndefinedFlagError)) ||
				(cmd.Subcommands.Get(undefArgs.Get(0)) == nil) {
				return nil, err
			}
		}

		if len(args) == 0 {
			break
		}

		undefArgs = append(undefArgs, args[0])
		args = args[1:]
	}

	if len(builtinCmd) > 0 {
		undefArgs = append(undefArgs, BuiltinCmdSep)
		undefArgs = append(undefArgs, builtinCmd...)
	}

	return undefArgs, nil
}

func (cmd *Command) flagSetParse(ctx *Context, flagSet *libflag.FlagSet, args Args) ([]string, error) {
	var (
		undefArgs []string
		err       error
	)

	if len(args) == 0 {
		return undefArgs, nil
	}

	const maxFlagsParse = 1000 // Maximum flags parse

	for range maxFlagsParse {
		// check if the error is due to an undefArgs flag
		var undefArg string

		err = flagSet.Parse(args)
		if err == nil {
			break
		}

		if errStr := err.Error(); strings.HasPrefix(errStr, ErrFlagUndefined) {
			undefArg = strings.Trim(strings.TrimPrefix(errStr, ErrFlagUndefined), " -")
			err = UndefinedFlagError(undefArg)
		} else {
			break
		}

		// cut off the args
		var notFoundMatch bool

		for i, arg := range args {
			// `--var=input=from_env` trims to `var`
			trimmed := strings.SplitN(strings.Trim(arg, "-"), "=", 2)[0] //nolint:mnd
			if trimmed == undefArg {
				undefArgs = append(undefArgs, arg)
				notFoundMatch = true
				args = args[i+1:]

				break
			}
		}

		if cmd.ErrorOnUndefinedFlag && !ctx.shellComplete {
			break
		}

		// This should be an impossible to reach code path, but in case the arg
		// splitting failed to happen, this will prevent infinite loops
		if !notFoundMatch {
			return nil, err
		}
	}

	undefArgs = append(undefArgs, flagSet.Args()...)

	return undefArgs, err
}

func (cmd *Command) WrapAction(fn func(ctx *Context, action ActionFunc) error) *Command {
	clone := *cmd

	action := clone.Action
	clone.Action = func(ctx *Context) error {
		return fn(ctx, action)
	}
	clone.Subcommands = clone.Subcommands.WrapAction(fn)

	return &clone
}
