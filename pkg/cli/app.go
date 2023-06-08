package cli

import (
	"github.com/gruntwork-io/terragrunt/errors"
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
	var showHelp bool

	app.SkipFlagParsing = true
	app.Authors = []*cli.Author{{Name: app.Author}}
	app.Flags = append(app.Flags, &Flag{
		Name:        "help",
		Usage:       "Show help",
		Destination: &showHelp,
	})

	app.App.Action = func(cliCtx *cli.Context) error {
		ctx := NewContext(cliCtx, app)

		if err := app.parseFlags(ctx.DoublePrefixedFlags()); err != nil {
			return err
		}

		// If someone calls us with `--help` or no args at all, show the help text and exit
		if showHelp || !ctx.Args().Present() {
			return ShowAppHelp(ctx)
		}

		return app.Action(ctx)
	}

	return app.App.Run(arguments)
}

// VisibleFlags returns a slice of the Flags used by `urfave/cli` package to generate help.
func (app *App) VisibleFlags() []cli.Flag {
	var flags []cli.Flag
	for _, flag := range app.Flags {
		flags = append(flags, flag)
	}
	return flags
}

func (app *App) parseFlags(args []string) error {
	set, err := app.Flags.flagSet("rootCmd")
	if err != nil {
		return err
	}

	if err := set.Parse(args); err != nil {
		return errors.WithStackTrace(err)
	}
	return nil
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
