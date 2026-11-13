// Package commands represents CLI commands.
package commands

import (
	"context"
	"path/filepath"
	"runtime/debug"
	"slices"
	"strings"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
	"golang.org/x/sync/errgroup"

	"github.com/gruntwork-io/terragrunt/internal/cache"
	rewrite "github.com/gruntwork-io/terragrunt/internal/config/rewrite"
	"github.com/gruntwork-io/terragrunt/internal/engine"
	"github.com/gruntwork-io/terragrunt/internal/experiment"
	"github.com/gruntwork-io/terragrunt/internal/filter"
	"github.com/gruntwork-io/terragrunt/internal/panicreport"
	"github.com/gruntwork-io/terragrunt/internal/spinner"
	"github.com/gruntwork-io/terragrunt/internal/tf"
	"github.com/gruntwork-io/terragrunt/internal/util"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/pkg/options"

	awsproviderpatch "github.com/gruntwork-io/terragrunt/internal/cli/commands/aws-provider-patch"
	"github.com/gruntwork-io/terragrunt/internal/cli/commands/backend"
	"github.com/gruntwork-io/terragrunt/internal/cli/commands/browse"
	"github.com/gruntwork-io/terragrunt/internal/cli/commands/catalog"
	"github.com/gruntwork-io/terragrunt/internal/cli/commands/dag"
	execcmd "github.com/gruntwork-io/terragrunt/internal/cli/commands/exec"
	"github.com/gruntwork-io/terragrunt/internal/cli/commands/find"
	"github.com/gruntwork-io/terragrunt/internal/cli/commands/hcl"
	helpcmd "github.com/gruntwork-io/terragrunt/internal/cli/commands/help"
	"github.com/gruntwork-io/terragrunt/internal/cli/commands/info"
	"github.com/gruntwork-io/terragrunt/internal/cli/commands/list"
	"github.com/gruntwork-io/terragrunt/internal/cli/commands/login"
	mcpcmd "github.com/gruntwork-io/terragrunt/internal/cli/commands/mcp"
	"github.com/gruntwork-io/terragrunt/internal/cli/commands/render"
	runcmd "github.com/gruntwork-io/terragrunt/internal/cli/commands/run"
	"github.com/gruntwork-io/terragrunt/internal/cli/commands/scaffold"
	"github.com/gruntwork-io/terragrunt/internal/cli/commands/stack"
	versioncmd "github.com/gruntwork-io/terragrunt/internal/cli/commands/version"
	"github.com/gruntwork-io/terragrunt/internal/clihelper"
	"github.com/gruntwork-io/terragrunt/internal/iacargs"
	"github.com/gruntwork-io/terragrunt/internal/os/exec"
	semver "github.com/gruntwork-io/terragrunt/internal/semver"
	"github.com/gruntwork-io/terragrunt/internal/telemetry"
	"github.com/gruntwork-io/terragrunt/internal/tips"
	"github.com/gruntwork-io/terragrunt/pkg/config"
	"github.com/gruntwork-io/terragrunt/pkg/log/format/placeholders"
)

// Command category names.
const (
	// MainCommandsCategoryName represents primary Terragrunt operations like run, exec.
	MainCommandsCategoryName = "Main commands"
	// CatalogCommandsCategoryName represents commands for managing Terragrunt catalogs.
	CatalogCommandsCategoryName = "Catalog commands"
	// PortalCommandsCategoryName represents commands for the Gruntwork Developer Portal.
	PortalCommandsCategoryName = "Gruntwork Developer Portal commands"
	// DiscoveryCommandsCategoryName represents commands for discovering Terragrunt configurations.
	DiscoveryCommandsCategoryName = "Discovery commands"
	// ConfigurationCommandsCategoryName represents commands for managing Terragrunt configurations.
	ConfigurationCommandsCategoryName = "Configuration commands"
	// ShortcutsCommandsCategoryName represents OpenTofu-specific shortcut commands.
	ShortcutsCommandsCategoryName = "OpenTofu shortcuts"
)

// Command categories appear in help in the order declared here.
const (
	mainCommandsOrder = iota + 1
	catalogCommandsOrder
	portalCommandsOrder
	discoveryCommandsOrder
	configurationCommandsOrder
	shortcutsCommandsOrder
)

// New returns the set of Terragrunt commands, grouped into categories.
func New(l log.Logger, opts *options.TerragruntOptions, v *venv.Venv, s *rewrite.Store) clihelper.Commands {
	mainCommands := clihelper.Commands{
		runcmd.NewCommand(l, opts, v, s),  // run
		stack.NewCommand(l, opts, v, s),   // stack
		execcmd.NewCommand(l, opts, v),    // exec
		backend.NewCommand(l, opts, v, s), // backend
	}.SetCategory(
		&clihelper.Category{
			Name:  MainCommandsCategoryName,
			Order: mainCommandsOrder,
		},
	)

	catalogCommands := clihelper.Commands{
		catalog.NewCommand(l, opts, v),  // catalog
		scaffold.NewCommand(l, opts, v), // scaffold
	}.SetCategory(
		&clihelper.Category{
			Name:  CatalogCommandsCategoryName,
			Order: catalogCommandsOrder,
		},
	)

	portalCommands := clihelper.Commands{
		login.NewCommand(l, opts, v), // login
	}.SetCategory(
		&clihelper.Category{
			Name:  PortalCommandsCategoryName,
			Order: portalCommandsOrder,
		},
	)

	discoveryCommands := clihelper.Commands{
		find.NewCommand(l, opts, v, s),   // find
		list.NewCommand(l, opts, v, s),   // list
		browse.NewCommand(l, opts, v, s), // browse
	}.SetCategory(
		&clihelper.Category{
			Name:  DiscoveryCommandsCategoryName,
			Order: discoveryCommandsOrder,
		},
	)

	configurationCommands := clihelper.Commands{
		hcl.NewCommand(l, opts, v, s),              // hcl
		info.NewCommand(l, opts, v, s),             // info
		dag.NewCommand(l, opts, v, s),              // dag
		render.NewCommand(l, opts, v, s),           // render
		mcpcmd.NewCommand(l, opts, v),              // mcp
		helpcmd.NewCommand(l, opts),                // help (hidden)
		versioncmd.NewCommand(),                    // version (hidden)
		awsproviderpatch.NewCommand(l, opts, v, s), // aws-provider-patch (hidden)
	}.SetCategory(
		&clihelper.Category{
			Name:  ConfigurationCommandsCategoryName,
			Order: configurationCommandsOrder,
		},
	)

	shortcutsCommands := NewShortcutsCommands(l, opts, v, s).SetCategory(
		&clihelper.Category{
			Name:  ShortcutsCommandsCategoryName,
			Order: shortcutsCommandsOrder,
		},
	)

	allCommands := mainCommands.
		Merge(catalogCommands...).
		Merge(portalCommands...).
		Merge(discoveryCommands...).
		Merge(configurationCommands...).
		Merge(shortcutsCommands...)

	return allCommands
}

// telemetryShutdownTimeout limits how long the telemetry flush can delay exit.
const telemetryShutdownTimeout = 30 * time.Second

// WrapWithTelemetry wraps CLI command execution with telemetry initialization,
// context setting and labels. If telemetry is disabled, just runs the command.
//
// The telemeter is created here rather than at app startup because command
// actions run after the full CLI parse, so telemetry options and experiments
// (such as otel-logs) are honored whether they were set via flags or env vars.
func WrapWithTelemetry(
	l log.Logger,
	opts *options.TerragruntOptions,
	v *venv.Venv,
) func(ctx context.Context, cliCtx *clihelper.Context, action clihelper.ActionFunc) error {
	return func(
		ctx context.Context,
		cliCtx *clihelper.Context,
		action clihelper.ActionFunc,
	) error {
		telemeter, err := telemetry.NewTelemeter(
			ctx,
			l,
			cliCtx.App.Name,
			cliCtx.App.Version,
			cliCtx.App.Writer,
			opts.Telemetry,
			opts.Experiments.Evaluate(experiment.OtelLogs),
		)
		if err != nil {
			return err
		}

		// Flush even after an interrupt cancels ctx, but do not wait forever.
		defer func() {
			shutdownCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), telemetryShutdownTimeout)
			defer cancel()

			if err := telemeter.Shutdown(shutdownCtx); err != nil {
				if _, writeErr := cliCtx.App.ErrWriter.Write([]byte(err.Error())); writeErr != nil {
					l.Debugf("Failed to report telemetry shutdown error %v: %v", err, writeErr)
				}
			}
		}()

		ctx = telemetry.ContextWithTelemeter(ctx, telemeter)

		resolveErr := resolveWorkingDir(v, opts)

		return telemeter.Collect(ctx, l, cliCtx.Command.Name, map[string]any{
			"dir": opts.WorkingDir,
		}, func(childCtx context.Context, l log.Logger) error {
			// Engines emit telemetry during shutdown, so stop them while the
			// command span is still open (and before the telemeter's deferred
			// Shutdown flushes) to keep their records correlated with it.
			defer func() {
				if err := engine.Shutdown(
					childCtx,
					l,
					opts.Experiments,
					opts.EngineOptions.NoEngine,
				); err != nil {
					if _, writeErr := cliCtx.App.ErrWriter.Write([]byte(err.Error())); writeErr != nil {
						l.Debugf("Failed to report engine shutdown error %v: %v", err, writeErr)
					}
				}
			}()

			if resolveErr != nil {
				return resolveErr
			}

			if err := initialSetup(cliCtx, l, v, opts); err != nil {
				return err
			}

			setCommandSpanAttributes(childCtx, cliCtx, opts)

			if err := RunAction(childCtx, cliCtx, l, opts, v, action); err != nil {
				opts.Tips.Find(tips.DebuggingDocs).Evaluate(l)
				return err
			}

			return nil
		})
	}
}

// RunAction wires up cancellation and run-scoped caches, then invokes action
// with the resulting context.
func RunAction(
	ctx context.Context,
	cliCtx *clihelper.Context,
	l log.Logger,
	opts *options.TerragruntOptions,
	v *venv.Venv,
	action clihelper.ActionFunc,
) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	errGroup, ctx := errgroup.WithContext(ctx)

	// Install run-scoped caches on actionCtx so memoized helpers like
	// [github.com/gruntwork-io/terragrunt/internal/git.GoRepoRoot] and
	// the version probes share state across the whole action.
	actionCtx := cache.ContextWithCache(ctx)

	if opts.Experiments.Evaluate(experiment.SlowTaskReporting) {
		reporter := newProgressReporter(l, opts, v)
		actionCtx = spinner.ContextWithReporter(actionCtx, reporter)

		if reporter.Animated() {
			defer guardTerminal(l, v, reporter)()
		}
	}

	// Run command action
	errGroup.Go(func() (err error) {
		defer cancel()
		defer func() {
			if rec := recover(); rec != nil {
				err = panicreport.NewRecoveredError(rec, debug.Stack())
			}
		}()

		if action != nil {
			return action(actionCtx, cliCtx)
		}

		return nil
	})

	return errGroup.Wait()
}

// resolveWorkingDir makes opts.WorkingDir a clean absolute path, using the
// current directory when it is unset.
func resolveWorkingDir(v *venv.Venv, opts *options.TerragruntOptions) error {
	if opts.WorkingDir == "" {
		currentDir, err := v.Platform.Getwd()
		if err != nil {
			return err
		}

		opts.WorkingDir = filepath.Clean(currentDir)

		return nil
	}

	if filepath.IsAbs(opts.WorkingDir) {
		opts.WorkingDir = filepath.Clean(opts.WorkingDir)

		return nil
	}

	workingDir, err := filepath.Abs(opts.WorkingDir)
	if err != nil {
		return err
	}

	opts.WorkingDir = filepath.Clean(workingDir)

	return nil
}

// mostly preparing terragrunt options
func initialSetup(
	cliCtx *clihelper.Context,
	l log.Logger,
	v *venv.Venv,
	opts *options.TerragruntOptions,
) error {
	// convert the rest flags (intended for terraform) to one dash, e.g. `--input=true` to `-input=true`
	args := cliCtx.Args().WithoutBuiltinCmdSep().Normalize(clihelper.SingleDashFlag)
	cmdName := cliCtx.Command.Name
	isRunCommand := cmdName == runcmd.CommandName

	if isRunCommand {
		cmdName = args.CommandName()
	}

	if !isRunCommand {
		args = append([]string{cmdName}, args...)
	}

	// Terragrunt's `--no-color` propagates to tofu unless the user already passed it after `--`.
	if l.Formatter().DisabledColors() && !slices.Contains(args, tf.FlagNameNoColor) {
		args = append(args, tf.FlagNameNoColor)
	}

	opts.TofuCommand = cmdName
	opts.TerraformCliArgs = iacargs.New(args...)

	l = l.WithField(placeholders.WorkDirKeyName, opts.WorkingDir)

	opts.RootWorkingDir = opts.WorkingDir

	if err := l.Formatter().SetBaseDir(opts.RootWorkingDir); err != nil {
		return err
	}

	if opts.LogShowAbsPaths {
		l.Formatter().DisableRelativePaths()
	}

	// --- Download Dir
	if opts.DownloadDir == "" {
		opts.DownloadDir = filepath.Join(opts.WorkingDir, util.TerragruntCacheDir)
	} else if !filepath.IsAbs(opts.DownloadDir) {
		opts.DownloadDir = filepath.Join(opts.RootWorkingDir, opts.DownloadDir)
	}

	opts.DownloadDir = filepath.Clean(opts.DownloadDir)

	// --- Terragrunt ConfigPath
	if opts.TerragruntConfigPath == "" {
		opts.TerragruntConfigPath = config.GetDefaultConfigPath(v.FS, opts.WorkingDir)
	} else if !filepath.IsAbs(opts.TerragruntConfigPath) &&
		(cliCtx.Command.Name == runcmd.CommandName || slices.Contains(tf.CommandNames, cliCtx.Command.Name)) {
		opts.TerragruntConfigPath = filepath.Join(opts.WorkingDir, opts.TerragruntConfigPath)
	}

	opts.TerragruntConfigPath = filepath.Clean(opts.TerragruntConfigPath)

	if !filepath.IsAbs(opts.TFPath) && strings.Contains(opts.TFPath, string(filepath.Separator)) {
		opts.TFPath = filepath.Join(opts.WorkingDir, opts.TFPath)
	}

	var fileFilterStrings []string

	excludeFiltersFromFile, err := util.ExcludeFiltersFromFile(
		v.FS,
		opts.WorkingDir,
		opts.ExcludesFile,
	)
	if err != nil {
		return err
	}

	fileFilterStrings = append(fileFilterStrings, excludeFiltersFromFile...)

	// Process filters file if the filters file is not disabled
	if !opts.NoFiltersFile {
		filtersFromFile, filtersFromFileErr := util.GetFiltersFromFile(
			v.FS,
			opts.WorkingDir,
			opts.FiltersFile,
		)
		if filtersFromFileErr != nil {
			return filtersFromFileErr
		}

		fileFilterStrings = append(fileFilterStrings, filtersFromFile...)
	}

	if len(fileFilterStrings) > 0 {
		parsed, parseErr := filter.ParseFilterQueries(l, fileFilterStrings)
		if parseErr != nil {
			return parseErr
		}

		opts.Filters = append(opts.Filters, parsed...)
	}

	// Deduplicate filters by their string representation
	seen := make(map[string]struct{}, len(opts.Filters))
	deduped := make(filter.Filters, 0, len(opts.Filters))

	for _, f := range opts.Filters {
		key := f.String()
		if _, ok := seen[key]; ok {
			continue
		}

		seen[key] = struct{}{}

		deduped = append(deduped, f)
	}

	opts.Filters = deduped

	// --- Terragrunt Version
	terragruntVersion, err := semver.Parse(cliCtx.Version)
	if err != nil {
		// Malformed Terragrunt version; set the version to 0.0
		if terragruntVersion, err = semver.Parse("0.0"); err != nil {
			return err
		}
	}

	opts.TerragruntVersion = terragruntVersion
	// Log the terragrunt version in debug mode. This helps with
	// debugging issues and ensuring a specific version is used.
	l.Debugf("Terragrunt Version: %s", opts.TerragruntVersion)

	// --- Others
	if !opts.RunAllAutoApprove {
		// When running in no-auto-approve mode, set parallelism to 1 so that interactive prompts work.
		opts.Parallelism = 1
	}

	opts.OriginalTerragruntConfigPath = opts.TerragruntConfigPath
	opts.OriginalTofuCommand = opts.TerraformCliArgs.CommandName(opts.TofuCommand)
	opts.OriginalIAMRoleOptions = opts.IAMRoleOptions

	if !exec.PrepareConsole(l) {
		l.Debugf("Virtual terminal processing not available, disabling colors")
		l.Formatter().SetDisabledColors(true)
	}

	return nil
}

// newProgressReporter returns a reporter that draws only next to text logs at info level or lower.
func newProgressReporter(l log.Logger, opts *options.TerragruntOptions, v *venv.Venv) *spinner.Reporter {
	textLogs := !opts.JSONLogFormat && !l.Formatter().DisabledOutput()

	return spinner.New(v, spinner.Options{
		Threshold:     spinner.DefaultThreshold,
		LogsForHumans: textLogs && l.Level() >= log.InfoLevel,
	})
}

// guardTerminal wraps the logger, stderr and stdout around the progress line and returns a function that unwraps them.
func guardTerminal(l log.Logger, v *venv.Venv, reporter *spinner.Reporter) func() {
	writers := v.Writers
	restoreLogger := reporter.GuardLogger(l)

	v.Writers = writers.WithErrWriter(reporter.Guard(writers.ErrWriter))
	if v.Terminal.StdoutIsTTY() {
		v.Writers = v.Writers.WithWriter(reporter.Guard(writers.Writer))
	}

	return func() {
		v.Writers = writers

		restoreLogger()
	}
}

// setCommandSpanAttributes records on the command span the tofu command and
// arguments that initialSetup resolves, for a command that runs OpenTofu/Terraform.
func setCommandSpanAttributes(ctx context.Context, cliCtx *clihelper.Context, opts *options.TerragruntOptions) {
	span := trace.SpanFromContext(ctx)
	if !span.IsRecording() {
		return
	}

	if !runsTofu(cliCtx) || opts.TofuCommand == "" {
		return
	}

	span.SetAttributes(
		attribute.String("tofu_command", opts.TofuCommand),
		attribute.StringSlice("args", opts.TerraformCliArgs.Slice()),
	)
}

// runsTofu reports whether cliCtx is a command that passes its arguments to
// OpenTofu/Terraform: run, stack run, a subcommand of run, or a shortcut.
func runsTofu(cliCtx *clihelper.Context) bool {
	if cliCtx.Command.Name == runcmd.CommandName {
		return true
	}

	if cliCtx.Command.Category != nil && cliCtx.Command.Category.Name == ShortcutsCommandsCategoryName {
		return true
	}

	parent := cliCtx.Parent().Command

	return parent != nil && parent.Name == runcmd.CommandName
}
