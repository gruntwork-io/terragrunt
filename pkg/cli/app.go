package cli

import (
	"os"

	"github.com/urfave/cli/v2"
)

// App is the main structure of app cli application. It should be created with the cli.NewApp() function.
type App struct {
	*cli.App
	// List of commands to execute
	Commands Commands
	// List of flags to parse
	Flags Flags
	// Contributor
	Author string
	// The action to execute before Action func, when no subcommands are specified, but after the context is ready
	Before ActionFunc
	// The action to execute after Action funcs, when no subcommands are specified
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
	app.SkipFlagParsing = true
	app.Authors = []*cli.Author{{Name: app.Author}}

	app.App.Action = func(parentCtx *cli.Context) error {
		args := parentCtx.Args().Slice()
		ctx := newContext(parentCtx.Context, app)

		cmd := ctx.App.newRootCommand()
		err := cmd.Run(ctx, args)
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
