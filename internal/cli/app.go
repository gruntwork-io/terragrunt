// Package cli configures the Terragrunt CLI app and its commands.
package cli

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/gruntwork-io/terragrunt/internal/runner/run"
	"github.com/gruntwork-io/terragrunt/pkg/log"

	"github.com/gruntwork-io/terragrunt/internal/engine"
	"github.com/gruntwork-io/terragrunt/internal/os/signal"
	"golang.org/x/text/cases"
	"golang.org/x/text/language"

	"github.com/gruntwork-io/terragrunt/internal/cli/commands"
	"github.com/gruntwork-io/terragrunt/internal/cli/flags"
	"github.com/gruntwork-io/terragrunt/internal/cli/flags/global"

	"errors"

	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/internal/version"
	"github.com/gruntwork-io/terragrunt/pkg/config"

	"github.com/gruntwork-io/terragrunt/internal/clihelper"
	"github.com/gruntwork-io/terragrunt/pkg/options"
)

const (
	AppName = "terragrunt"
)

func init() {
	clihelper.AppVersionTemplate = AppVersionTemplate
	clihelper.AppHelpTemplate = AppHelpTemplate
	clihelper.CommandHelpTemplate = CommandHelpTemplate
}

type App struct {
	*clihelper.App
	opts *options.TerragruntOptions
}

// NewApp creates the Terragrunt CLI App. The supplied [venv.Venv] is the
// root virtualized environment; it is threaded through to the command
// constructors and captured by their Action closures rather than held on
// the App, so virtualized handlers stay function parameters. Its environment
// map is what env-var-backed flags resolve against.
func NewApp(l log.Logger, opts *options.TerragruntOptions, v *venv.Venv) *App {
	terragruntCommands := commands.New(l, opts, v)

	app := clihelper.NewApp(v.Env)
	app.Name = AppName
	app.Usage = "Terragrunt is a flexible orchestration tool that allows Infrastructure as Code written in OpenTofu/Terraform to scale.\nFor documentation, see https://docs.terragrunt.com/."
	app.Author = "Gruntwork <www.gruntwork.io>"
	app.Version = version.GetVersion()
	app.Writer = v.Writers.Writer
	app.ErrWriter = v.Writers.ErrWriter
	app.Flags = global.NewFlags(l, opts, nil)
	app.Commands = terragruntCommands.
		WrapAction(commands.WrapWithTelemetry(l, opts, v)).
		WrapAction(commands.WrapWithProfiling(l, opts, v))
	app.Before = beforeAction(opts)
	app.OsExiter = OSExiter
	app.ExitErrHandler = ExitErrHandler
	app.FlagErrHandler = flags.ErrorHandler(terragruntCommands)
	app.Action = clihelper.ShowAppHelp

	return &App{app, opts}
}

func (app *App) Run(l log.Logger, v *venv.Venv, args []string) error {
	return app.RunContext(context.Background(), l, v, args)
}

func (app *App) registerGracefullyShutdown(ctx context.Context, l log.Logger, v *venv.Venv) context.Context {
	v.RequireSignals()

	ctx, cancel := context.WithCancelCause(ctx)

	v.Signals(ctx, func(sig os.Signal) {
		// Carriage return helps prevent "^C" from being printed
		if _, err := fmt.Fprint(app.Writer, "\r"); err != nil {
			l.Debugf("Failed to write to the output on %s: %v", sig, err)
		}

		l.Infof(
			"%s signal received. Gracefully shutting down...",
			cases.Title(language.English).String(sig.String()),
		)

		cancel(signal.NewContextCanceledError(sig))
	}, signal.InterruptSignals...)

	return ctx
}

func (app *App) RunContext(
	ctx context.Context,
	l log.Logger,
	v *venv.Venv,
	args []string,
) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	ctx = app.registerGracefullyShutdown(ctx, l, v)

	ctx = config.WithConfigValues(ctx)
	// configure engine context
	ctx = engine.WithEngineValues(ctx)

	ctx = run.WithRunVersionCache(ctx)
	ctx = run.WithModuleVersionResolver(ctx, v)

	args = RemoveNoColorFlagDuplicates(args)

	if err := app.App.RunContext(ctx, args); err != nil && !errors.Is(err, context.Canceled) {
		return err
	}

	return nil
}

// RemoveNoColorFlagDuplicates keeps only the first `--no-color` argument, since the parser rejects
// a flag that is set more than once. Arguments from the `--` terminator onward belong to tofu and
// pass through as written.
func RemoveNoColorFlagDuplicates(args []string) []string {
	var (
		foundNoColor bool
		filteredArgs = make([]string, 0, len(args))
	)

	for i, arg := range args {
		if arg == "--" {
			return append(filteredArgs, args[i:]...)
		}

		if isNoColorFlag(arg) {
			if foundNoColor {
				continue
			}

			foundNoColor = true
		}

		filteredArgs = append(filteredArgs, arg)
	}

	return filteredArgs
}

func isNoColorFlag(arg string) bool {
	if !strings.HasPrefix(arg, "-") {
		return false
	}

	name, _, _ := strings.Cut(strings.TrimPrefix(arg[1:], "-"), "=")

	return name == global.NoColorFlagName
}

func beforeAction(_ *options.TerragruntOptions) clihelper.ActionFunc {
	return func(ctx context.Context, cliCtx *clihelper.Context) error {
		// setting current context to the options
		// show help if the args are not specified.
		if !cliCtx.Args().Present() {
			err := clihelper.ShowAppHelp(ctx, cliCtx)
			// exit the app
			return clihelper.NewExitError(err, 0)
		}

		// If args are present but the first non-flag token is not a known
		// top-level command, fail fast with guidance to use `run --`.
		// This removes the legacy behavior of implicitly forwarding unknown
		// commands to OpenTofu/Terraform.
		cmdName := cliCtx.Args().CommandName()
		if cmdName != "" {
			if cliCtx.Command == nil || cliCtx.Command.Subcommand(cmdName) == nil {
				return clihelper.NewExitError(
					UnknownCommandError(cmdName),
					clihelper.ExitCodeGeneralError,
				)
			}
		}

		return nil
	}
}

// OSExiter is an empty function that overrides the default behavior.
func OSExiter(exitCode int) {
	// Do nothing. We just need to override this function, as the default value calls os.Exit, which
	// kills the app (or any automated test) dead in its tracks.
}

// ExitErrHandler is an empty function that overrides the default behavior.
func ExitErrHandler(_ *clihelper.Context, err error) error {
	return err
}
