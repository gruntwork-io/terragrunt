package cli

import (
	"slices"

	"github.com/gruntwork-io/terragrunt/internal/errors"
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

func (cmd *Command) parseFlags(ctx *Context, args Args) (Args, error) {
	if cmd.SkipFlagParsing {
		return args, nil
	}

	flagErrHandler := func(err error) error {
		if err == nil || ctx.shellComplete {
			return nil
		}

		var undefErr UndefinedFlagError
		if errors.As(err, &undefErr) {
			if cmd.DisabledErrorOnUndefinedFlag || cmd.Subcommands.Get(undefErr.CmdName) != nil {
				return nil
			}
		}

		if cmd.DisabledErrorOnMultipleSetFlag && IsSetValueMultipleTimesError(err) {
			return nil
		}

		if errHandler := ctx.App.FlagErrHandler; errHandler != nil {
			err = errHandler(ctx.NewCommandContext(cmd, args), err)
		}

		return err
	}

	// The first attempt is to parse flags with scope restrictions, see `flags.AllowedSubcommandScope`.
	args, err := cmd.Flags.WithSubcommandScope().Parse(args, flagErrHandler)
	if err == nil || !args.Present() {
		return args, err
	}

	return cmd.Flags.Parse(args, flagErrHandler)
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

// AllFlags returns all flags, including subcommand flags.
func (cmd *Command) AllFlags() Flags {
	flags := cmd.Flags

	for _, flag := range cmd.Subcommands.AllFlags() {
		if !slices.Contains(flags, flag) {
			flags = append(flags, flag)
		}
	}

	return flags
}

func (cmd *Command) ApplyConfig(cfgGetter FlagConfigGetter, seen *Flags) error {
	for _, flag := range cmd.Flags {
		if slices.Contains(*seen, flag) {
			continue
		}
		*seen = append(*seen, flag)

		if rawKey, val := cfgGetter.Get(cmd, flag); val != nil {
			if err := ApplyConfig(flag, rawKey, val); err != nil {
				return err
			}
		}

	}

	return cmd.Subcommands.ApplyConfig(cfgGetter, seen)
}
