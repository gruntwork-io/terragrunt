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
// `terragrunt run --all apply --log-level trace --auto-approve --non-interactive`
// The `App` will runs the registered command `run --all`, define the registered flags `--log-level`,
// `--non-interactive`, and define args `apply --auto-approve` which can be obtained from the App context,
// ctx.Args().Slice()
type App struct {
	// AutocompleteInstaller supports autocompletion via the github.com/posener/complete
	// library. This library supports bash, zsh and fish. To add support
	// for other shells, please see that library.
	AutocompleteInstaller AutocompleteInstaller

	// FlagErrHandler processes any error encountered while parsing flags.
	FlagErrHandler FlagErrHandlerFunc

	// ExitErrHandler processes any error encountered while running an App before
	// it is returned to the caller. If no function is provided, HandleExitCoder
	// is used as the default behavior.
	ExitErrHandler ExitErrHandlerFunc

	*cli.App

	// Before is an action to execute before any subcommands are run, but after the context is ready.
	Before ActionFunc

	// After is an action to execute after
	// any subcommands are run, but after the subcommand has finished.
	After ActionFunc

	// Complete is the function to call when checking for command completions.
	Complete CompleteFunc

	// Action is the action to execute when no subcommands are specified.
	Action ActionFunc

	// OsExiter is the function used when the app exits. If not set defaults to os.Exit.
	OsExiter func(code int)

	// Author is the author of the app.
	Author string

	// CustomAppVersionTemplate is a text template for app version topic.
	CustomAppVersionTemplate string

	// AutocompleteInstallFlag is the global flag name for installing the autocompletion handlers for the user's shell.
	AutocompleteInstallFlag string

	// AutocompleteUninstallFlag is the global flag name for uninstalling the autocompletion handlers for the user's shell.
	AutocompleteUninstallFlag string

	// Commands is a list of commands to execute.
	Commands Commands

	// Flags is a list of flags to parse.
	Flags Flags

	// Examples is a list of examples of using the App in the help.
	Examples []string

	// Autocomplete enables or disables subcommand auto-completion support.
	Autocomplete bool

	// DisabledErrorOnUndefinedFlag prevents the application to exit and return an error on any undefined flag.
	DisabledErrorOnUndefinedFlag bool

	// DisabledErrorOnMultipleSetFlag prevents the application to exit and return an error if any flag is set multiple times.
	DisabledErrorOnMultipleSetFlag bool
}

// NewApp returns app new App instance.
func NewApp() *App {
	cliApp := cli.NewApp()
	cliApp.ExitErrHandler = func(_ *cli.Context, _ error) {}
	cliApp.HideHelp = true
	cliApp.HideHelpCommand = true

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
		cmd := app.NewRootCommand()

		args := Args(parentCtx.Args().Slice())
		ctx := NewAppContext(parentCtx.Context, app, args)

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

		return cmd.Run(ctx, args)
	}

	return app.App.RunContext(ctx, arguments)
}

// VisibleFlags returns a slice of the Flags used for help.
func (app *App) VisibleFlags() Flags {
	return app.Flags.VisibleFlags()
}

// VisibleCommands returns a slice of the Commands used for help.
func (app *App) VisibleCommands() Commands {
	if app.Commands == nil {
		return nil
	}

	return app.Commands.Sort().VisibleCommands()
}

func (app *App) NewRootCommand() *Command {
	return &Command{
		Name:                           app.Name,
		Before:                         app.Before,
		After:                          app.After,
		Action:                         app.Action,
		Usage:                          app.Usage,
		UsageText:                      app.UsageText,
		Description:                    app.Description,
		Examples:                       app.Examples,
		Flags:                          app.Flags,
		Subcommands:                    app.Commands,
		Complete:                       app.Complete,
		IsRoot:                         true,
		DisabledErrorOnUndefinedFlag:   app.DisabledErrorOnUndefinedFlag,
		DisabledErrorOnMultipleSetFlag: app.DisabledErrorOnMultipleSetFlag,
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
		switch arg {
		case "-" + app.AutocompleteInstallFlag, "--" + app.AutocompleteInstallFlag:
			isAutocompleteInstall = true
		case "-" + app.AutocompleteUninstallFlag, "--" + app.AutocompleteUninstallFlag:
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
