// Package cli provides functionality for the Terragrunt CLI.
package cli

import (
	"context"
	"os"
	"strings"

	"github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/urfave/cli/v2"
)

// App is a wrapper for `urfave`'s `cli.App` struct. It should be created with the cli.NewApp() function.
// The main purpose of this wrapper is to parse commands and flags in the way we need, namely,
// if during parsing we find undefined commands or flags, instead of returning an error, we consider them as arguments,
// regardless of their position among the others registered commands and flags.
//
// For example, CLI command:
// `terragrunt run-all apply --terragrunt-log-level trace --auto-approve --terragrunt-non-interactive`
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
	// The function to call when checking for command completions
	Complete CompleteFunc
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

	// ExitErrHandler processes any error encountered while running an App before
	// it is returned to the caller. If no function is provided, HandleExitCoder
	// is used as the default behavior.
	ExitErrHandler ExitErrHandlerFunc

	// Autocomplete enables or disables subcommand auto-completion support.
	// This is enabled by default when NewApp is called. Otherwise, this
	// must enabled explicitly.
	//
	// Autocomplete requires the "Name" option to be set on CLI. This name
	// should be set exactly to the binary name that is autocompleted.
	Autocomplete bool

	// AutocompleteInstallFlag and AutocompleteUninstallFlag are the global flag
	// names for installing and uninstalling the autocompletion handlers
	// for the user's shell. The flag should omit the hyphen(s) in front of
	// the value. Both single and double hyphens will automatically be supported
	// for the flag name. These default to `autocomplete-install` and
	// `autocomplete-uninstall` respectively.
	AutocompleteInstallFlag   string
	AutocompleteUninstallFlag string

	// Autocompletion is supported via the github.com/posener/complete
	// library. This library supports bash, zsh and fish. To add support
	// for other shells, please see that library.
	AutocompleteInstaller AutocompleteInstaller
}

// NewApp returns app new App instance.
func NewApp() *App {
	cliApp := cli.NewApp()
	cliApp.ExitErrHandler = func(_ *cli.Context, _ error) {}

	return &App{
		App:          cliApp,
		OsExiter:     os.Exit,
		Autocomplete: true,
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
	return app.RunContext(context.Background(), arguments)
}

// RunContext is like Run except it takes a Context that will be
// passed to its commands and sub-commands. Through this, you can
// propagate timeouts and cancellation requests
func (app *App) RunContext(ctx context.Context, arguments []string) (err error) {
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
		cmd := app.newRootCommand()

		args := Args(parentCtx.Args().Slice())
		ctx := NewContext(parentCtx.Context, app)

		if app.Autocomplete {
			if err := app.setupAutocomplete(args); err != nil {
				return app.handleExitCoder(ctx, err)
			}

			if compLine := os.Getenv(envCompleteLine); compLine != "" {
				args = strings.Fields(compLine)
				if args[0] == app.Name {
					args = args[1:]
				}

				ctx.shellComplete = true
			}
		}

		return cmd.Run(ctx, args.Normalize(SingleDashFlag))
	}

	return app.App.RunContext(ctx, arguments)
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
		Complete:    app.Complete,
		IsRoot:      true,
	}
}

func (app *App) setupAutocomplete(arguments []string) error {
	var (
		isAutocompleteInstall   bool
		isAutocompleteUninstall bool
	)

	if app.AutocompleteInstallFlag == "" {
		app.AutocompleteInstallFlag = defaultAutocompleteInstallFlag
	}

	if app.AutocompleteUninstallFlag == "" {
		app.AutocompleteUninstallFlag = defaultAutocompleteUninstallFlag
	}

	if app.AutocompleteInstaller == nil {
		app.AutocompleteInstaller = &autocompleteInstaller{}
	}

	for _, arg := range arguments {
		switch {
		// Check for autocomplete flags
		case arg == "-"+app.AutocompleteInstallFlag || arg == "--"+app.AutocompleteInstallFlag:
			isAutocompleteInstall = true

		case arg == "-"+app.AutocompleteUninstallFlag || arg == "--"+app.AutocompleteUninstallFlag:
			isAutocompleteUninstall = true
		}
	}

	// Autocomplete requires the "Name" to be set so that we know what command to setup the autocomplete on.
	if app.Name == "" {
		return errors.Errorf("internal error: App.Name must be specified for autocomplete to work")
	}

	// If both install and uninstall flags are specified, then error
	if isAutocompleteInstall && isAutocompleteUninstall {
		return errors.Errorf("either the autocomplete install or uninstall flag may be specified, but not both")
	}

	// If the install flag is specified, perform the install or uninstall and exit
	if isAutocompleteInstall {
		err := app.AutocompleteInstaller.Install(app.Name)
		return NewExitError(err, 0)
	}

	if isAutocompleteUninstall {
		err := app.AutocompleteInstaller.Uninstall(app.Name)
		return NewExitError(err, 0)
	}

	return nil
}

func (app *App) handleExitCoder(ctx *Context, err error) error {
	if err == nil || err.Error() == "" {
		return nil
	}

	if app.ExitErrHandler != nil {
		return app.ExitErrHandler(ctx, err)
	}

	return handleExitCoder(ctx, err, app.OsExiter)
}
