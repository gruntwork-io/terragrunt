// Package cli configures the Terragrunt CLI app and its commands.
package cli

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/gruntwork-io/terragrunt/engine"
	"github.com/gruntwork-io/terragrunt/internal/cliconfig"
	"github.com/gruntwork-io/terragrunt/internal/os/signal"
	"github.com/gruntwork-io/terragrunt/telemetry"
	"github.com/gruntwork-io/terragrunt/util"
	"golang.org/x/text/cases"
	"golang.org/x/text/language"

	"github.com/gruntwork-io/terragrunt/cli/commands"
	"github.com/gruntwork-io/terragrunt/cli/flags"
	"github.com/gruntwork-io/terragrunt/cli/flags/global"

	"github.com/gruntwork-io/go-commons/version"
	"github.com/gruntwork-io/terragrunt/config"
	"github.com/gruntwork-io/terragrunt/internal/errors"

	"github.com/gruntwork-io/terragrunt/internal/cli"
	"github.com/gruntwork-io/terragrunt/options"
)

const (
	AppName = "terragrunt"
)

func init() {
	cli.AppVersionTemplate = AppVersionTemplate
	cli.AppHelpTemplate = AppHelpTemplate
	cli.CommandHelpTemplate = CommandHelpTemplate
}

type App struct {
	*cli.App
	opts *options.TerragruntOptions
}

// NewApp creates the Terragrunt CLI App.
func NewApp(opts *options.TerragruntOptions) *App {
	terragruntCommands := commands.New(opts)

	app := cli.NewApp()
	app.Name = AppName
	app.Usage = "Terragrunt is a flexible orchestration tool that allows Infrastructure as Code written in OpenTofu/Terraform to scale.\nFor documentation, see https://terragrunt.gruntwork.io/."
	app.Author = "Gruntwork <www.gruntwork.io>"
	app.Version = version.GetVersion()
	app.Writer = opts.Writer
	app.ErrWriter = opts.ErrWriter
	app.Flags = global.NewFlagsWithDeprecatedMovedFlags(opts)
	app.Commands = terragruntCommands.WrapAction(commands.WrapWithTelemetry(opts))
	app.Before = beforeAction(opts)
	app.OsExiter = OSExiter
	app.ExitErrHandler = ExitErrHandler
	app.FlagErrHandler = flags.ErrorHandler(terragruntCommands)
	app.Action = cli.ShowAppHelp

	return &App{app, opts}
}

func (app *App) registerGracefullyShutdown(ctx context.Context) context.Context {
	ctx, cancel := context.WithCancelCause(ctx)

	signal.NotifierWithContext(ctx, func(sig os.Signal) {
		// Carriage return helps prevent "^C" from being printed
		fmt.Fprint(app.Writer, "\r") //nolint:errcheck
		app.opts.Logger.Infof("%s signal received. Gracefully shutting down...", cases.Title(language.English).String(sig.String()))

		cancel(signal.NewContextCanceledError(sig))
	}, signal.InterruptSignals...)

	return ctx
}

func (app *App) ApplyConfig(ctx context.Context) error {
	if err := app.opts.NormalizeWorkingDir(); err != nil {
		return err
	}

	paths := []string{
		app.opts.CLIConfigFile,
		app.opts.WorkingDir,
	}

	repoDir, err := filepath.Abs(app.opts.WorkingDir)
	if err != nil {
		return errors.New(err)
	}

	const maxWalking = 1000

	for range maxWalking {
		gitDir := filepath.Join(repoDir, ".git")
		if util.FileExists(gitDir) && util.IsDir(gitDir) {
			paths = append(paths, repoDir)

			cfgRepoDir := filepath.Join(repoDir, ".config")
			if util.FileExists(cfgRepoDir) {
				paths = append(paths, cfgRepoDir)
			}

			break
		}

		if newRepoDir := filepath.Dir(repoDir); newRepoDir != repoDir {
			repoDir = newRepoDir
		} else {
			break
		}
	}

	path, err := cliconfig.DiscoveryPath(paths...)
	if err != nil || path == "" {
		return err
	}

	cfg, err := cliconfig.LoadConfig(path)
	if err != nil {
		return errors.Errorf("could not load CLI config %s: %w", path, err)
	}

	if err := app.AllFlags().ApplyConfig(cfg); err != nil {
		return errors.Errorf("could not apply CLI config %s: %w", path, err)
	}

	app.opts.Logger.Debugf("Loaded CLI configuration file %s", cfg.Path())

	if extraKeys := cfg.ExtraKeys(); len(extraKeys) > 0 {
		app.opts.Logger.Warnf("CLI configuration file contains unused keys: %s", strings.Join(extraKeys, ","))
	}

	return nil
}

func (app *App) Run(args []string) error {
	return app.RunContext(context.Background(), args)
}

func (app *App) RunContext(ctx context.Context, args []string) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	ctx = app.registerGracefullyShutdown(ctx)

	// These essential flags we must parse immediately to be able:
	// 1. Log at the user-specified level from the very beginning.
	// 2. Load CLI config file since it can also contain the log level and other settings used from the start.
	// 3. Init telemetery at the very beginning.
	args, err := app.Flags.Filter(
		global.LogLevelFlagName,
		global.WorkingDirFlagName,
		global.CLIConfigFileFlagName,
		// Telemetry flags
		global.TelemetryTraceExporterFlagName,
		global.TelemetryTraceExporterInsecureEndpointFlagName,
		global.TelemetryTraceExporterHTTPEndpointFlagName,
		global.TraceparentFlagName,
		global.TelemetryMetricExporterFlagName,
		global.TelemetryMetricExporterInsecureEndpointFlagName,
	).Parse(args, cli.IgnoringUndefinedFlagErrorHandler)
	if err != nil {
		return err
	}

	if err := app.ApplyConfig(ctx); err != nil {
		return err
	}

	telemeter, err := telemetry.NewTelemeter(ctx, app.Name, app.Version, app.Writer, app.opts.Telemetry)
	if err != nil {
		return err
	}
	defer func(ctx context.Context) {
		if err := telemeter.Shutdown(ctx); err != nil {
			_, _ = app.ErrWriter.Write([]byte(err.Error()))
		}
	}(ctx)

	ctx = telemetry.ContextWithTelemeter(ctx, telemeter)

	ctx = config.WithConfigValues(ctx)
	// configure engine context
	ctx = engine.WithEngineValues(ctx)

	defer func(ctx context.Context) {
		if err := engine.Shutdown(ctx, app.opts); err != nil {
			_, _ = app.ErrWriter.Write([]byte(err.Error()))
		}
	}(ctx)

	args = removeNoColorFlagDuplicates(args)

	if err := app.App.RunContext(ctx, args); err != nil && !errors.IsContextCanceled(err) {
		return err
	}

	return nil
}

// removeNoColorFlagDuplicates removes one of the `--no-color` or `--terragrunt-no-color` arguments if both are present.
// We have to do this because `--terragrunt-no-color` is a deprecated alias for `--no-color`,
// therefore we end up specifying the same flag twice, which causes the `setting the flag multiple times` error.
func removeNoColorFlagDuplicates(args []string) []string {
	var (
		foundNoColor bool
		filteredArgs = make([]string, 0, len(args))
	)

	for _, arg := range args {
		if strings.HasSuffix(arg, "-"+global.NoColorFlagName) {
			if foundNoColor {
				continue
			}

			foundNoColor = true
		}

		filteredArgs = append(filteredArgs, arg)
	}

	return filteredArgs
}

func beforeAction(_ *options.TerragruntOptions) cli.ActionFunc {
	return func(ctx *cli.Context) error {
		// setting current context to the options
		// show help if the args are not specified.
		if !ctx.Args().Present() {
			err := cli.ShowAppHelp(ctx)
			// exit the app
			return cli.NewExitError(err, 0)
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
func ExitErrHandler(_ *cli.Context, err error) error {
	return err
}
