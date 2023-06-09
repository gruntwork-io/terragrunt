package cli

import (
	"flag"

	"github.com/urfave/cli/v2"
)

// App is the main structure of a cli application. It should be created with the cli.NewApp() function.
type App struct {
	*cli.App
	Flags  Flags
	Author string
	Action ActionFunc
}

// AddFlags adds a new cli flag.
func (app *App) AddFlags(flags ...*Flag) {
	app.Flags = append(app.Flags, flags...)
}

// Run is the entry point to the cli app. Parses the arguments slice and routes to the proper flag/args combination.
func (app *App) Run(arguments []string) (err error) {
	app.SkipFlagParsing = true
	app.Authors = []*cli.Author{{Name: app.Author}}

	app.App.Action = func(cliCtx *cli.Context) error {
		args, err := app.parseArgs(cliCtx.Args().Slice())
		if err != nil {
			return err
		}

		ctx := &Context{
			Context: cliCtx,
			App:     app,
			args:    args,
		}

		return app.Action(ctx)
	}

	return app.App.Run(arguments)
}

// VisibleFlags returns a slice of the Flags, used by `urfave/cli` package to generate help.
func (app *App) VisibleFlags() []cli.Flag {
	var flags []cli.Flag
	for _, flag := range app.Flags {
		flags = append(flags, flag)
	}
	return flags
}

func (app *App) parseArgs(args []string) ([]string, error) {
	var filteredArgs []string

	flagSet, err := app.Flags.newFlagSet("root-command", flag.ContinueOnError)
	if err != nil {
		return nil, err
	}

	for {
		args, err = app.Flags.parseArgs(flagSet, args)
		if err != nil {
			return nil, err
		}

		filteredArgs = append(filteredArgs, args[0])
		args = args[1:]

		if len(args) == 0 {
			break
		}
	}

	return filteredArgs, nil
}

// NewApp returns a new App instance.
func NewApp() *App {
	return &App{
		App: cli.NewApp(),
	}
}

func init() {
	cli.OsExiter = func(exitCode int) {
		// Do nothing. We just need to override this function, as the default value calls os.Exit, which
		// kills the app (or any automated test) dead in its tracks.
	}
}
