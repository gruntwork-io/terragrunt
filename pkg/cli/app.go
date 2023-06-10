package cli

import (
	"github.com/gruntwork-io/gruntwork-cli/collections"
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
	Before ActionFunc
	// The action to execute when no subcommands are specified
	Action ActionFunc

	rootCommand *Command
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
	app.rootCommand = app.newRootCommand()
	app.SkipFlagParsing = true
	app.Authors = []*cli.Author{{Name: app.Author}}

	app.App.Action = func(cliCtx *cli.Context) error {
		command, args, err := app.parseArgs(cliCtx.Args().Slice())
		if err != nil {
			return err
		}

		ctx := &Context{
			Context: cliCtx,
			App:     app,
			args:    args,
			Command: command,
		}

		if app.Before != nil {
			if err := app.Before(ctx); err != nil {
				return err
			}
		}

		if command.Action != nil {
			return command.Action(ctx)
		}
		return app.Action(ctx)
	}

	return app.App.Run(arguments)
}

// VisibleFlags returns app slice of the Flags.
func (app *App) VisibleFlags() Flags {
	return app.Flags
}

// VisibleCommands returns a slice of the Commands.
func (app *App) VisibleCommands() []*cli.Command {
	var commands []*cli.Command

	for _, command := range app.Commands {
		commands = append(commands, &cli.Command{
			Name:        command.Name,
			Aliases:     command.Aliases,
			Usage:       command.Usage,
			UsageText:   command.UsageText,
			Description: command.Description,
		})
	}

	return commands
}

func (app *App) parseArgs(args []string) (*Command, []string, error) {
	var err error
	lastCommand := app.rootCommand

	args, err = app.rootCommand.parseArgs(args)
	if err != nil {
		return nil, nil, err
	}

	if len(args) == 0 {
		return lastCommand, args, nil
	}

	for _, command := range app.Commands {
		if args[0] == "*" {
			continue
		}
		if !collections.ListContainsElement(command.Names(), args[0]) {
			continue
		}
		lastCommand = command

		args, err = command.parseArgs(args[1:])
		if err != nil {
			return nil, nil, err
		}
		break
	}

	return lastCommand, args, nil
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
