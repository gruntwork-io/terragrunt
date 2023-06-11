package cli

import (
	"github.com/urfave/cli/v2"
)

// App is the main structure of app cli application. It should be created with the cli.NewApp() function.
type App struct {
	*cli.App
	// List of commands to execute
	Commands Commands
	// List of flags to parse
	Flags Flags
	// List of all authors who contributed
	Author string
	// An action to execute before any subcommands are run, but after the context is ready
	// If a non-nil error is returned, no subcommands are run
	Before RunFunc
	// An action to execute after any subcommands are run, but after the subcommand has finished
	After RunFunc
	// The action to execute when no subcommands are specified
	Action RunFunc
}

// NewApp returns app new App instance.
func NewApp() *App {
	return &App{
		App: cli.NewApp(),
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
func (app *App) Run(arguments []string) (err error) {
	app.SkipFlagParsing = true
	app.Authors = []*cli.Author{{Name: app.Author}}

	app.App.Action = func(parentCtx *cli.Context) error {
		args := parentCtx.Args().Slice()

		command, args, err := app.newRootCommand().parseArgs(args)
		if err != nil {
			return err
		}

		ctx := NewContext(parentCtx, app, command, args)

		if app.Before != nil {
			if err := app.Before(ctx); err != nil {
				return err
			}
		}

		if err := command.run(ctx); err != nil {
			return err
		}

		if app.After != nil {
			if err := app.After(ctx); err != nil {
				return err
			}
		}

		return nil
	}

	return app.App.Run(arguments)
}

// VisibleFlags returns a slice of the Flags.
func (app *App) VisibleFlags() Flags {
	return app.Flags.VisibleFlags()
}

// VisibleCommands returns a slice of the Commands.
func (app *App) VisibleCommands() []*cli.Command {
	return app.Commands.VisibleCommands()
}

func (app *App) newRootCommand() *Command {
	return &Command{
		Name:        app.Name,
		Action:      app.Action,
		Usage:       app.Usage,
		UsageText:   app.UsageText,
		Description: app.Description,
		Flags:       app.Flags,
		IsRoot:      true,
	}
}

func init() {
	cli.OsExiter = func(exitCode int) {
		// Do nothing. We just need to override this function, as the default value calls os.Exit, which
		// kills the app (or any automated test) dead in its tracks.
	}
}
