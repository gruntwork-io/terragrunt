package cli

import (
	"errors"
	libflag "flag"
	"strings"
)

type Command struct {
	// Category is the category the command belongs to.
	Category *Category

	// Before is an action to execute before the command is invoked.
	// If a non-nil error is returned, no further processing is done.
	Before ActionFunc

	// CustomHelp is a custom function to display help text.
	CustomHelp HelpFunc

	// After is the function to call after the command is invoked.
	After ActionFunc

	// Complete is the function to call for shell completion.
	Complete CompleteFunc

	// Action is the function to execute when the command is invoked.
	// Runs after subcommands are finished.
	Action ActionFunc

	// Description is a longer explanation of how the command works.
	Description string

	// HelpName is the full name of the command for help.
	// Defaults to the full command name, including parent commands.
	HelpName string

	// Name is the command name.
	Name string

	// UsageText is custom text to show on the `Usage` section of the help.
	UsageText string

	// CustomHelpTemplate is a custom text template for the help topic.
	CustomHelpTemplate string

	// Usage is a short description of the usage for the command.
	Usage string

	// Flags is a list of flags to parse.
	Flags Flags

	// Examples is a list of examples for using the command in help.
	Examples []string

	// Subcommands is a list of subcommands.
	Subcommands Commands

	// Aliases is a list of aliases for the command.
	Aliases []string

	// IsRoot is true if this is a root "special" command.
	// NOTE: The author of this comment doesn't know what this means.
	IsRoot bool

	// SkipRunning disables the parsing command, but it will
	// still be shown in help.
	SkipRunning bool

	// SkipFlagParsing treats all flags as normal arguments.
	SkipFlagParsing bool

	// Hidden hides the command from help.
	Hidden bool

	// DisabledErrorOnUndefinedFlag prevents the application to exit and return an error on any undefined flag.
	DisabledErrorOnUndefinedFlag bool

	// DisabledErrorOnMultipleSetFlag prevents the application to exit and return an error if any flag is set multiple times.
	DisabledErrorOnMultipleSetFlag bool
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
	return cmd.Flags.VisibleFlags()
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
	if err != nil {
		return NewExitError(err, ExitCodeGeneralError)
	}

	ctx = ctx.NewCommandContext(cmd, args)

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
		return ctx.handleExitCoder(ctx, err)
	}

	defer func() {
		if cmd.After != nil && err == nil {
			err = cmd.After(ctx)
			err = ctx.handleExitCoder(ctx, err)
		}
	}()

	if cmd.Before != nil {
		if err := cmd.Before(ctx); err != nil {
			return ctx.handleExitCoder(ctx, err)
		}
	}

	if subCmd != nil && !subCmd.SkipRunning {
		return subCmd.Run(ctx, subCmdArgs)
	}

	if cmd.Action != nil {
		if err = cmd.Action(ctx); err != nil {
			return ctx.handleExitCoder(ctx, err)
		}
	}

	return nil
}

func (cmd *Command) parseFlags(ctx *Context, args Args) ([]string, error) {
	var undefArgs Args

	errHandler := func(err error) error {
		if err == nil {
			return nil
		}

		if cmd.DisabledErrorOnMultipleSetFlag && IsMultipleTimesSettingError(err) {
			return nil
		}

		if flagErrHandler := ctx.FlagErrHandler; flagErrHandler != nil {
			err = flagErrHandler(ctx.NewCommandContext(cmd, args), err)
		}

		return err
	}

	flagSet, err := cmd.Flags.NewFlagSet(cmd.Name, errHandler)
	if err != nil {
		return nil, err
	}

	flagSetWithSubcommandScope, err := cmd.Flags.WithSubcommandScope().NewFlagSet(cmd.Name, errHandler)
	if err != nil {
		return nil, err
	}

	if cmd.SkipFlagParsing {
		return args, nil
	}

	args, builtinCmd := args.Split(BuiltinCmdSep)

	for i := 0; len(args) > 0; i++ {
		if i == 0 {
			args, err = cmd.flagSetParse(ctx, flagSet, args)
		} else {
			args, err = cmd.flagSetParse(ctx, flagSetWithSubcommandScope, args)
		}

		if len(args) != 0 {
			undefArgs = append(undefArgs, args[0])
			args = args[1:]
		}

		if err != nil {
			if !errors.As(err, new(UndefinedFlagError)) ||
				(cmd.Subcommands.Get(undefArgs.Get(0)) == nil) {
				if err = errHandler(err); err != nil {
					return undefArgs, err
				}
			}
		}
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

		if err = flagSet.Parse(args); err == nil {
			break
		}

		if errStr := err.Error(); strings.HasPrefix(errStr, ErrMsgFlagUndefined) {
			undefArg = strings.Trim(strings.TrimPrefix(errStr, ErrMsgFlagUndefined), " -")
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

		if !cmd.DisabledErrorOnUndefinedFlag && !ctx.shellComplete {
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

// DisableErrorOnMultipleSetFlag returns cloned commands with disabled the check for multiple values set for the same flag.
func (cmd *Command) DisableErrorOnMultipleSetFlag() *Command {
	newCmd := *cmd
	newCmd.DisabledErrorOnMultipleSetFlag = true
	newCmd.Subcommands = newCmd.Subcommands.DisableErrorOnMultipleSetFlag()

	return &newCmd
}
