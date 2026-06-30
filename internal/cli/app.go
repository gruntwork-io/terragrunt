// Package cli configures the Terragrunt CLI app and its commands.
package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"runtime/pprof"
	"strconv"
	"strings"

	"github.com/gruntwork-io/terragrunt/internal/runner/run"
	"github.com/gruntwork-io/terragrunt/internal/tf"
	"github.com/gruntwork-io/terragrunt/pkg/log"

	"github.com/gruntwork-io/terragrunt/internal/engine"
	"github.com/gruntwork-io/terragrunt/internal/os/signal"
	"github.com/gruntwork-io/terragrunt/internal/telemetry"
	"golang.org/x/text/cases"
	"golang.org/x/text/language"

	"github.com/gruntwork-io/terragrunt/internal/cli/commands"
	"github.com/gruntwork-io/terragrunt/internal/cli/flags"
	"github.com/gruntwork-io/terragrunt/internal/cli/flags/global"

	"github.com/gruntwork-io/terragrunt/internal/experiment"
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
	l    log.Logger
}

// NewApp creates the Terragrunt CLI App. The supplied [venv.Venv] is the
// root virtualized environment; it is threaded through to the command
// constructors and captured by their Action closures rather than held on
// the App, so virtualized handlers stay function parameters.
func NewApp(l log.Logger, opts *options.TerragruntOptions, v venv.Venv) *App {
	terragruntCommands := commands.New(l, opts, v)

	app := clihelper.NewApp()
	app.Name = AppName
	app.Usage = "Terragrunt is a flexible orchestration tool that allows Infrastructure as Code written in OpenTofu/Terraform to scale.\nFor documentation, see https://docs.terragrunt.com/."
	app.Author = "Gruntwork <www.gruntwork.io>"
	app.Version = version.GetVersion()
	app.Writer = opts.Writers.Writer
	app.ErrWriter = opts.Writers.ErrWriter
	app.Flags = global.NewFlags(l, opts, nil)
	app.Commands = terragruntCommands.WrapAction(commands.WrapWithTelemetry(l, opts, v))
	app.Before = beforeAction(opts)
	app.OsExiter = OSExiter
	app.ExitErrHandler = ExitErrHandler
	app.FlagErrHandler = flags.ErrorHandler(terragruntCommands)
	app.Action = clihelper.ShowAppHelp

	return &App{app, opts, l}
}

func (app *App) Run(args []string) error {
	return app.RunContext(context.Background(), args)
}

func (app *App) registerGracefullyShutdown(ctx context.Context) context.Context {
	ctx, cancel := context.WithCancelCause(ctx)

	signal.NotifierWithContext(ctx, func(sig os.Signal) {
		// Carriage return helps prevent "^C" from being printed
		fmt.Fprint(app.Writer, "\r") //nolint:errcheck
		app.l.Infof("%s signal received. Gracefully shutting down...", cases.Title(language.English).String(sig.String()))

		cancel(signal.NewContextCanceledError(sig))
	}, signal.InterruptSignals...)

	return ctx
}

func (app *App) RunContext(ctx context.Context, args []string) error {
	// Bind experiment flags early (so the pprof gate below can see --experiment / TG_EXPERIMENT / --experiment-mode).
	if err := bindExperimentsEarly(app.opts, args); err != nil {
		return err
	}

	// Bind profile opts early from provided args / env before reading them for resolution.
	if err := bindProfileFlagsEarly(app.opts, args); err != nil {
		return err
	}

	// Resolve profile paths preferring CLI flags / new TG_PROFILE_* (opts), then legacy env vars.
	// DIR variants provide defaults when direct path not specified.
	cpuProfilePath := app.opts.ProfileCPU
	memProfilePath := app.opts.ProfileMEM
	goroutineProfilePath := app.opts.ProfileGoroutine

	const profileDirMode = 0755

	// Handle ProfileDir for opts-based (CLI or TG_PROFILE_DIR etc).
	if app.opts.ProfileDir != "" {
		if err := os.MkdirAll(app.opts.ProfileDir, profileDirMode); err != nil {
			return fmt.Errorf("could not create profile directory: %w", err)
		}

		if cpuProfilePath == "" {
			cpuProfilePath = filepath.Join(app.opts.ProfileDir, "terragrunt_cpu.prof")
		}

		if memProfilePath == "" {
			memProfilePath = filepath.Join(app.opts.ProfileDir, "terragrunt_mem.prof")
		}

		if goroutineProfilePath == "" {
			goroutineProfilePath = filepath.Join(app.opts.ProfileDir, "terragrunt_goroutine.prof")
		}
	}

	// Legacy env fallbacks (TG_CPU_PROFILE, TG_MEM_PROFILE, and their _DIR variants).
	// These continue to work without requiring the pprof experiment for backward compatibility.
	if cpuProfilePath == "" {
		cpuProfilePath = os.Getenv(tf.EnvNameTGCPUProfile)
	}

	if memProfilePath == "" {
		memProfilePath = os.Getenv(tf.EnvNameTGMemProfile)
	}

	if goroutineProfilePath == "" {
		// No dedicated legacy env for goroutine yet; support TG_GOROUTINE_PROFILE if someone used it.
		goroutineProfilePath = os.Getenv("TG_GOROUTINE_PROFILE")
	}

	if profileDir := os.Getenv(tf.EnvNameTGCPUProfileDir); profileDir != "" {
		if err := os.MkdirAll(profileDir, profileDirMode); err != nil {
			return fmt.Errorf("could not create CPU profile directory: %w", err)
		}

		if cpuProfilePath == "" {
			cpuProfilePath = filepath.Join(profileDir, "terragrunt_cpu.prof")
		}
	}

	if profileDir := os.Getenv(tf.EnvNameTGMemProfileDir); profileDir != "" {
		if err := os.MkdirAll(profileDir, profileDirMode); err != nil {
			return fmt.Errorf("could not create memory profile directory: %w", err)
		}

		if memProfilePath == "" {
			memProfilePath = filepath.Join(profileDir, "terragrunt_mem.prof")
		}
	}

	// If any profile path was provided via the new opts (CLI flags or TG_PROFILE_* envs),
	// require the pprof experiment to be enabled.
	usingNewProfileOpts := app.opts.ProfileCPU != "" || app.opts.ProfileMEM != "" || app.opts.ProfileGoroutine != "" || app.opts.ProfileDir != ""
	// Detect from incoming cmd args and os.Args (tests pass args, real bin uses os)
	allForScan := append([]string{}, os.Args...)
	allForScan = append(allForScan, args...)
	if !usingNewProfileOpts {
		for _, a := range allForScan {
			if a == "--"+global.ProfileCPUFlagName || a == "--terragrunt-"+global.ProfileCPUFlagName ||
				a == "--"+global.ProfileMEMFlagName || a == "--terragrunt-"+global.ProfileMEMFlagName ||
				a == "--"+global.ProfileGoroutineFlagName || a == "--terragrunt-"+global.ProfileGoroutineFlagName ||
				a == "--"+global.ProfileDirFlagName || a == "--terragrunt-"+global.ProfileDirFlagName {
				usingNewProfileOpts = true
				break
			}
		}
	}

	// Early enable pprof experiment if --experiment pprof (or terragrunt- variant) is present in args.
	for i := 0; i < len(allForScan); i++ {
		if allForScan[i] == "--"+global.ExperimentFlagName || allForScan[i] == "--terragrunt-"+global.ExperimentFlagName || allForScan[i] == "-"+global.ExperimentFlagName {
			// next tokens may be values; scan a few
			for j := i + 1; j < len(allForScan) && j < i+4; j++ {
				v := allForScan[j]
				if strings.HasPrefix(v, "-") {
					break
				}
				if v == experiment.Pprof || strings.Contains(v, experiment.Pprof) {
					_ = app.opts.Experiments.EnableExperiment(experiment.Pprof)
					break
				}
			}
		}
	}

	if usingNewProfileOpts && !app.opts.Experiments.Evaluate(experiment.Pprof) {
		return fmt.Errorf("profiling flags require the 'pprof' experiment (use --experiment pprof)")
	}

	// Start CPU profiling if configured.
	if cpuProfilePath != "" {
		f, err := os.Create(cpuProfilePath)
		if err != nil {
			return fmt.Errorf("could not create CPU profile: %w", err)
		}

		if err := pprof.StartCPUProfile(f); err != nil {
			f.Close()

			return fmt.Errorf("could not start CPU profile: %w", err)
		}

		defer func() {
			pprof.StopCPUProfile()
			f.Close()
		}()
	}

	// Write memory (heap) profile at exit if configured.
	if memProfilePath != "" {
		defer func() {
			runtime.GC()

			f, err := os.Create(memProfilePath)
			if err != nil {
				return
			}
			defer f.Close()

			_ = pprof.WriteHeapProfile(f)
		}()
	}

	// Write goroutine profile (memory dump) at exit if configured.
	if goroutineProfilePath != "" {
		defer func() {
			f, err := os.Create(goroutineProfilePath)
			if err != nil {
				return
			}
			defer f.Close()

			if p := pprof.Lookup("goroutine"); p != nil {
				_ = p.WriteTo(f, 0)
			}
		}()
	}

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	ctx = app.registerGracefullyShutdown(ctx)

	if err := global.NewTelemetryFlags(app.opts, nil).Parse(os.Args); err != nil {
		return err
	}

	telemeter, err := telemetry.NewTelemeter(ctx, app.l, app.Name, app.Version, app.Writer, app.opts.Telemetry)
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

	ctx = run.WithRunVersionCache(ctx)

	defer func(ctx context.Context) {
		if err := engine.Shutdown(ctx, app.l, app.opts.Experiments, app.opts.EngineOptions.NoEngine); err != nil {
			_, _ = app.ErrWriter.Write([]byte(err.Error()))
		}
	}(ctx)

	args = removeNoColorFlagDuplicates(args)

	if err := app.App.RunContext(ctx, args); err != nil && !errors.Is(err, context.Canceled) {
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

// bindProfileFlagsEarly constructs a minimal flagset for profile flags and parses os.Args
// plus the command args passed to RunContext (for test helpers) and TG_PROFILE_* envs
// to populate opts.Profile* fields before heavy initialization.
func bindProfileFlagsEarly(opts *options.TerragruntOptions, cmdArgs []string) error {
	profileFlags := global.NewProfileFlags(opts, nil)

	fs, err := profileFlags.NewFlagSet("profile-early", func(e error) error { return e })
	if err != nil {
		return err
	}

	// Apply registers + reads TG_PROFILE_* envs.
	_ = fs.Parse(os.Args)
	if len(cmdArgs) > 0 {
		_ = fs.Parse(cmdArgs)
	}

	// Manual scan over both os.Args and cmdArgs for --profile-* (and terragrunt- variants)
	allArgs := append([]string{}, os.Args...)
	allArgs = append(allArgs, cmdArgs...)
	for i := 0; i < len(allArgs); i++ {
		a := allArgs[i]
		switch a {
		case "--" + global.ProfileCPUFlagName, "--terragrunt-" + global.ProfileCPUFlagName:
			if i+1 < len(allArgs) && opts.ProfileCPU == "" {
				opts.ProfileCPU = allArgs[i+1]
			}
		case "--" + global.ProfileMEMFlagName, "--terragrunt-" + global.ProfileMEMFlagName:
			if i+1 < len(allArgs) && opts.ProfileMEM == "" {
				opts.ProfileMEM = allArgs[i+1]
			}
		case "--" + global.ProfileGoroutineFlagName, "--terragrunt-" + global.ProfileGoroutineFlagName:
			if i+1 < len(allArgs) && opts.ProfileGoroutine == "" {
				opts.ProfileGoroutine = allArgs[i+1]
			}
		case "--" + global.ProfileDirFlagName, "--terragrunt-" + global.ProfileDirFlagName:
			if i+1 < len(allArgs) && opts.ProfileDir == "" {
				opts.ProfileDir = allArgs[i+1]
			}
		}
	}

	return nil
}

// bindExperimentsEarly parses experiment-related flags and env vars as early as possible
// so that features gated by experiments (such as pprof profiling) can decide correctly
// before the main command parsing runs.
func bindExperimentsEarly(opts *options.TerragruntOptions, cmdArgs []string) error {
	// We only care about the two experiment flags here.
	expFlags := clihelper.Flags{
		flags.NewFlag(&clihelper.SliceFlag[string]{
			Name:    global.ExperimentFlagName,
			EnvVars: flags.Prefix{flags.TgPrefix}.EnvVars(global.ExperimentFlagName),
			Setter:  opts.Experiments.EnableExperiment,
		}),
		flags.NewFlag(&clihelper.BoolFlag{
			Name:    global.ExperimentModeFlagName,
			EnvVars: flags.Prefix{flags.TgPrefix}.EnvVars(global.ExperimentModeFlagName),
			Setter: func(_ bool) error {
				opts.Experiments.ExperimentMode()
				return nil
			},
		}),
	}

	fs, err := expFlags.NewFlagSet("exp-early", func(e error) error { return e })
	if err != nil {
		return err
	}

	_ = fs.Parse(os.Args)
	if len(cmdArgs) > 0 {
		_ = fs.Parse(cmdArgs)
	}

	// Also check common deprecated / alternate forms manually (best effort)
	all := append([]string{}, os.Args...)
	all = append(all, cmdArgs...)
	for i := 0; i < len(all); i++ {
		a := all[i]
		if a == "--"+global.ExperimentFlagName || a == "--terragrunt-"+global.ExperimentFlagName {
			for j := i + 1; j < len(all) && j < i+8; j++ {
				v := all[j]
				if strings.HasPrefix(v, "-") {
					break
				}
				_ = opts.Experiments.EnableExperiment(v) // ignore unknown here; full validation happens later
			}
		}
		if a == "--"+global.ExperimentModeFlagName || a == "--terragrunt-"+global.ExperimentModeFlagName {
			opts.Experiments.ExperimentMode()
		}
	}

	// Check raw env for TG_EXPERIMENT (comma or space separated) and TG_EXPERIMENT_MODE
	if val := os.Getenv("TG_EXPERIMENT"); val != "" {
		for _, e := range strings.FieldsFunc(val, func(r rune) bool { return r == ',' || r == ' ' }) {
			e = strings.TrimSpace(e)
			if e != "" {
				_ = opts.Experiments.EnableExperiment(e)
			}
		}
	}
	if val := os.Getenv("TG_EXPERIMENT_MODE"); val != "" {
		if b, err := strconv.ParseBool(val); err == nil && b {
			opts.Experiments.ExperimentMode()
		}
	}
	// Also support the old TERRAGRUNT_ variants
	if val := os.Getenv("TERRAGRUNT_EXPERIMENT"); val != "" {
		for _, e := range strings.FieldsFunc(val, func(r rune) bool { return r == ',' || r == ' ' }) {
			e = strings.TrimSpace(e)
			if e != "" {
				_ = opts.Experiments.EnableExperiment(e)
			}
		}
	}
	if val := os.Getenv("TERRAGRUNT_EXPERIMENT_MODE"); val != "" {
		if b, err := strconv.ParseBool(val); err == nil && b {
			opts.Experiments.ExperimentMode()
		}
	}

	return nil
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
				// Show a clear error pointing users to the explicit run form.
				// Example: `terragrunt workspace ls` -> suggest `terragrunt run -- workspace ls`.
				return clihelper.NewExitError(
					fmt.Errorf("unknown command: %q. Terragrunt no longer forwards unknown commands by default. Use 'terragrunt run -- %s ...' or a supported shortcut. Learn more: https://docs.terragrunt.com/migrate/cli-redesign/#use-the-new-run-command", cmdName, cmdName),
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
