package cli

import (
	"os"
	"strings"

	"github.com/urfave/cli/v2"
)

// App is a wrapper for `urfave`'s `cli.App` struct. It should be created with the cli.NewApp() function.
// The main purpose of this wrapper is to parse commands and flags in the way we need, namely,
// if during parsing we find undefined commands or flags, instead of returning an error, we consider them as arguments,
// regardless of their position among the others registered commands and flags.
//
// For example, CLI command:
// `terragrunt run-all apply --terragrunt-log-level debug --auto-approve --terragrunt-non-interactive`
// The `App` will runs the registered command `run-all`, define the registered flags `--terragrunt-log-level`,
// `--terragrunt-non-interactive`, and define args `apply --auto-approve` which can be obtained from the App context,
// ctx.Args().Slice()
type App struct {
	*cli.App
	// List of commands to execute
	Commands Commands
	// List of flags to parse
	Flags Flags
	// CustomAppVersionTemplate text template for app version topic.
	CustomAppVersionTemplate string
	// Contributor
	Author string
	// An action to execute before running the `Action` of the target command.
	// The difference between `Before` is that `CommonBefore` runs only once for the target command, while `Before` is different for each command and is performed by each command.
	// Useful when some steps need to to performed for all commands without exception, when all flags are parsed and the context contains the target command.
	CommonBefore ActionFunc
	// An action to execute before any subcommands are run, but after the context is ready
	// If a non-nil error is returned, no subcommands are run
	Before ActionFunc
	// An action to execute after any subcommands are run, but after the subcommand has finished
	After ActionFunc
	// The action to execute when no subcommands are specified
	Action ActionFunc
	// DefaultCommand is the (optional)  command
	// to run if no command names are passed as CLI arguments.
	DefaultCommand *Command
	// OsExiter is the function used when the app exits. If not set defaults to os.Exit.
	OsExiter func(code int)
}

// NewApp returns app new App instance.
func NewApp() *App {
	return &App{
		App:      cli.NewApp(),
		OsExiter: os.Exit,
	}
}

// AddFlags adds new flags.
func (app *App) AddFlags(flags ...Flag) {
	app.Flags = append(app.Flags, flags...)
}

// AddCommands adds new commands.
func (app *App) AddCommands(cmds ...*Command) {
	app.Commands = append(app.Commands, cmds...)
}

// Run is the entry point to the cli app. Parses the arguments slice and routes to the proper flag/args combination.
func (app *App) Run(arguments []string) error {
	// remove empty args
	filteredArguments := []string{}
	for _, arg := range arguments {
		if trimmedArg := strings.TrimSpace(arg); len(trimmedArg) > 0 {
			filteredArguments = append(filteredArguments, trimmedArg)
		}
	}
	arguments = filteredArguments

	app.SkipFlagParsing = true
	app.Authors = []*cli.Author{{Name: app.Author}}

	app.App.Action = func(parentCtx *cli.Context) error {
		args := parentCtx.Args().Slice()
		ctx := newContext(parentCtx.Context, app)

		cmd := ctx.App.newRootCommand()
		err := cmd.Run(ctx, args...)
		return err
	}

	return app.App.Run(arguments)
}

// VisibleFlags returns a slice of the Flags used for help.
func (app *App) VisibleFlags() Flags {
	return app.Flags.VisibleFlags()
}

// VisibleCommands returns a slice of the Commands used for help.
func (app *App) VisibleCommands() []*cli.Command {
	if app.Commands == nil {
		return nil
	}
	return app.Commands.VisibleCommands()
}

func (app *App) newRootCommand() *Command {
	return &Command{
		Name:        app.Name,
		Before:      app.Before,
		After:       app.After,
		Action:      app.Action,
		Usage:       app.Usage,
		UsageText:   app.UsageText,
		Description: app.Description,
		Flags:       app.Flags,
		Subcommands: app.Commands,
		IsRoot:      true,
	}
}

func (app *App) handleExitCoder(err error) error {
	return handleExitCoder(err, app.OsExiter)
}
