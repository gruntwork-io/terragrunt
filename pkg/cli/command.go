package cli

import (
	libflag "flag"
	"io"
	"strings"

	"github.com/gruntwork-io/go-commons/errors"
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
	// if DisallowUndefinedFlags is true, any undefined flag will cause the application to exit and return an error.
	DisallowUndefinedFlags bool
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
func (c *Command) Names() []string {
	return append([]string{c.Name}, c.Aliases...)
}

// HasName returns true if Command.Name matches given name
func (c *Command) HasName(name string) bool {
	for _, n := range c.Names() {
		if n == name && name != "" {
			return true
		}
	}

	return false
}

// Subcommand returns a subcommand that matches the given name.
func (c *Command) Subcommand(name string) *Command {
	for _, c := range c.Subcommands {
		if c.HasName(name) {
			return c
		}
	}

	return nil
}

// VisibleFlags returns a slice of the Flags, used by `urfave/cli` package to generate help.
func (c *Command) VisibleFlags() Flags {
	return c.Flags
}

// VisibleSubcommands returns a slice of the Commands with Hidden=false.
// Used by `urfave/cli` package to generate help.
func (c Command) VisibleSubcommands() []*cli.Command {
	if c.Subcommands == nil {
		return nil
	}

	return c.Subcommands.VisibleCommands()
}

// Run parses the given args for the presence of flags as well as subcommands.
// If this is the final command, starts its execution.
func (c *Command) Run(ctx *Context, args Args) (err error) {
	args, err = c.parseFlags(args.Slice())
	if err != nil {
		return err
	}

	ctx = ctx.Clone(c, args)

	subCmdName := ctx.Args().CommandName()
	subCmdArgs := ctx.Args().Tail()
	subCmd := c.Subcommand(subCmdName)

	if ctx.shellComplete {
		if cmd := ctx.Command.Subcommand(args.CommandName()); cmd == nil {
			return ShowCompletions(ctx)
		}

		if subCmd != nil {
			return subCmd.Run(ctx, subCmdArgs)
		}
	}

	if err := c.Flags.RunActions(ctx); err != nil {
		return ctx.App.handleExitCoder(err)
	}

	defer func() {
		if c.After != nil && err == nil {
			err = c.After(ctx)
			err = ctx.App.handleExitCoder(err)
		}
	}()

	if c.Before != nil {
		if err := c.Before(ctx); err != nil {
			return ctx.App.handleExitCoder(err)
		}
	}

	if subCmd != nil && !subCmd.SkipRunning {
		return subCmd.Run(ctx, subCmdArgs)
	}

	if c.IsRoot && ctx.App.DefaultCommand != nil {
		err = ctx.App.DefaultCommand.Run(ctx, args)
		return err
	}

	if c.Action != nil {
		if err = c.Action(ctx); err != nil {
			return ctx.App.handleExitCoder(err)
		}
	}

	return nil
}

func (c *Command) parseFlags(args []string) ([]string, error) {
	var undefArgs []string

	flagSet, err := c.newFlagSet(libflag.ContinueOnError)
	if err != nil {
		return nil, err
	}

	if c.SkipFlagParsing {
		return args, nil
	}

	for {
		args, err = c.flagSetParse(flagSet, args)
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

func (c *Command) newFlagSet(errorHandling libflag.ErrorHandling) (*libflag.FlagSet, error) {
	flagSet := libflag.NewFlagSet(c.Name, errorHandling)
	flagSet.SetOutput(io.Discard)

	for _, flag := range c.Flags {
		if err := flag.Apply(flagSet); err != nil {
			return nil, err
		}
	}

	return flagSet, nil
}

func (c *Command) flagSetParse(flagSet *libflag.FlagSet, args []string) ([]string, error) {
	var undefArgs []string

	if len(args) == 0 {
		return undefArgs, nil
	}

	for {
		err := flagSet.Parse(args)
		if err == nil {
			break
		}

		// check if the error is due to an undefArgs flag
		var undefArg string

		errStr := err.Error()

		if c.DisallowUndefinedFlags || !strings.HasPrefix(errStr, errFlagUndefined) {
			return nil, errors.WithStackTrace(err)
		}

		undefArg = strings.Trim(strings.TrimPrefix(errStr, errFlagUndefined), " -")

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

		// This should be an impossible to reach code path, but in case the arg
		// splitting failed to happen, this will prevent infinite loops
		if !notFoundMatch {
			return nil, err
		}
	}

	undefArgs = append(undefArgs, flagSet.Args()...)

	return undefArgs, nil
}

func (c Command) WrapAction(fn func(ctx *Context, action ActionFunc) error) *Command {
	action := c.Action
	c.Action = func(ctx *Context) error {
		return fn(ctx, action)
	}
	c.Subcommands = c.Subcommands.WrapAction(fn)

	return &c
}
