// Package global provides CLI global flags.
package global

import (
	"context"
	"fmt"

	"slices"

	"github.com/gruntwork-io/go-commons/collections"
	"github.com/gruntwork-io/terragrunt/cli/commands"
	"github.com/gruntwork-io/terragrunt/cli/commands/catalog"
	"github.com/gruntwork-io/terragrunt/cli/commands/help"
	"github.com/gruntwork-io/terragrunt/cli/commands/run"
	"github.com/gruntwork-io/terragrunt/cli/commands/scaffold"
	"github.com/gruntwork-io/terragrunt/cli/commands/version"
	"github.com/gruntwork-io/terragrunt/cli/flags"
	"github.com/gruntwork-io/terragrunt/internal/cli"
	"github.com/gruntwork-io/terragrunt/internal/strict"
	"github.com/gruntwork-io/terragrunt/internal/strict/controls"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/pkg/log/format"
	"github.com/gruntwork-io/terragrunt/util"
)

const (
	// Logs related flags.

	LogLevelFlagName        = "log-level"
	LogDisableFlagName      = "log-disable"
	ShowLogAbsPathsFlagName = "log-show-abs-paths"
	LogFormatFlagName       = "log-format"
	LogCustomFormatFlagName = "log-custom-format"
	NoColorFlagName         = "no-color"

	NonInteractiveFlagName = "non-interactive"
	WorkingDirFlagName     = "working-dir"

	// Strict Mode related flags.

	StrictModeFlagName    = "strict-mode"
	StrictControlFlagName = "strict-control"

	// Experiment Mode related flags/envs.

	ExperimentModeFlagName = "experiment-mode"
	ExperimentFlagName     = "experiment"

	// CLI config flag.
	CLIConfigFileFlagName = "cli-config-file"

	// App flags.

	HelpFlagName    = "help"
	VersionFlagName = "version"

	// Telemetry flags.

	TelemetryTraceExporterFlagName                  = "telemetry-trace-exporter"
	TelemetryTraceExporterInsecureEndpointFlagName  = "telemetry-trace-exporter-insecure-endpoint"
	TelemetryTraceExporterHTTPEndpointFlagName      = "telemetry-trace-exporter-http-endpoint"
	TraceparentFlagName                             = "traceparent"
	TelemetryMetricExporterFlagName                 = "telemetry-metric-exporter"
	TelemetryMetricExporterInsecureEndpointFlagName = "telemetry-metric-exporter-insecure-endpoint"

	// Renamed flags.

	DeprecatedLogLevelFlagName        = "log-level"
	DeprecatedLogDisableFlagName      = "log-disable"
	DeprecatedShowLogAbsPathsFlagName = "log-show-abs-paths"
	DeprecatedLogFormatFlagName       = "log-format"
	DeprecatedLogCustomFormatFlagName = "log-custom-format"
	DeprecatedNoColorFlagName         = "no-color"
	DeprecatedNonInteractiveFlagName  = "non-interactive"
	DeprecatedTFInputFlagName         = "tf-input"
	DeprecatedWorkingDirFlagName      = "working-dir"
	DeprecatedStrictModeFlagName      = "strict-mode"
	DeprecatedStrictControlFlagName   = "strict-control"
	DeprecatedExperimentModeFlagName  = "experiment-mode"
	DeprecatedExperimentFlagName      = "experiment"

	// Deprecated flags.

	DeprecatedDisableLogFormattingFlagName = "disable-log-formatting"
	DeprecatedJSONLogFlagName              = "json-log"
	DeprecatedTfLogJSONFlagName            = "tf-logs-to-json"
)

// experimentalCommands is a list of experimental commands for which the deprecated messages about moved global flags should not be displayed unless the `cli-redesign` experiment is enabled.
var experimentalCommands = []string{
	run.CommandName,
	catalog.CommandName,
	scaffold.CommandName,
}

// NewFlagsWithDeprecatedMovedFlags returns global flags along with flags that have been moved to other commands and hidden from CLI help.
func NewFlagsWithDeprecatedMovedFlags(opts *options.TerragruntOptions) cli.Flags {
	globalFlags := NewFlags(opts)
	commands := commands.New(opts).FilterByNames(experimentalCommands...)

	var seen []string

	for _, cmd := range commands {
		for _, flag := range cmd.Flags {
			flagName := util.FirstElement(util.RemoveEmptyElements(flag.Names()))

			if slices.Contains(seen, flagName) {
				continue
			}

			// Disable strcit control evaluation of moves global flags for the experimental `run` command if the `cli-redesign` experiment is not enabled.
			evaluateWrapper := func(ctx context.Context, evalFn func(ctx context.Context) error) error {
				return evalFn(ctx)
			}

			seen = append(seen, flagName)
			globalFlags = append(globalFlags, flags.NewMovedFlag(
				flag,
				cmd.Name,
				flags.StrictControlsByMovedGlobalFlags(opts.StrictControls, cmd.Name),
				flags.WithEvaluateWrapper(evaluateWrapper),
			))
		}
	}

	return globalFlags
}

// NewFlags creates and returns global flags common for all commands.
func NewFlags(opts *options.TerragruntOptions) cli.Flags {
	strictControl := flags.StrictControlsByGlobalFlags(opts.StrictControls)
	legacyLogsControl := flags.StrictControlsByGlobalFlags(opts.StrictControls, controls.LegacyLogs)

	flags := cli.Flags{
		flags.NewFlag(&cli.GenericFlag[string]{
			Name:        LogLevelFlagName,
			EnvVars:     flags.EnvVarsWithTgPrefix(LogLevelFlagName),
			ConfigKey:   flags.ConfigKey(LogLevelFlagName),
			DefaultText: opts.Logger.Level().String(),
			Setter:      opts.Logger.SetLevel,
			Usage:       fmt.Sprintf("Sets the logging level for Terragrunt. Supported levels: %s.", log.AllLevels),
			Action: func(_ *cli.Context, val string) error {
				// Before the release of v0.67.0, these levels actually disabled logs, since we do not use these levels for logging.
				// For backward compatibility we simulate the same behavior.
				removedLevels := []string{
					"panic",
					"fatal",
				}

				if collections.ListContainsElement(removedLevels, val) {
					opts.ForwardTFStdout = true
					opts.Logger.Formatter().SetDisabledOutput(true)
				}

				return nil
			},
		}, flags.WithDeprecatedNames(flags.FlagNamesWithTerragruntPrefix(DeprecatedLogLevelFlagName), strictControl)),
		flags.NewFlag(&cli.GenericFlag[string]{
			Name:        WorkingDirFlagName,
			EnvVars:     flags.EnvVarsWithTgPrefix(WorkingDirFlagName),
			Destination: &opts.WorkingDir,
			Usage:       "The path to the directory of Terragrunt configurations. Default is current directory.",
		},
			flags.WithDeprecatedNames(flags.FlagNamesWithTerragruntPrefix(DeprecatedWorkingDirFlagName), strictControl),
		),
		flags.NewFlag(&cli.GenericFlag[string]{
			Name:        CLIConfigFileFlagName,
			EnvVars:     flags.EnvVarsWithTgPrefix(CLIConfigFileFlagName),
			Destination: &opts.CLIConfigFile,
			Hidden:      true,
			Usage:       "The path to CLI configuration file.",
		}),
		flags.NewFlag(&cli.BoolFlag{
			Name:      LogDisableFlagName,
			EnvVars:   flags.EnvVarsWithTgPrefix(LogDisableFlagName),
			ConfigKey: flags.ConfigKey(LogDisableFlagName),
			Usage:     "Disable logging.",
			Setter: func(val bool) error {
				opts.Logger.Formatter().SetDisabledOutput(val)
				opts.ForwardTFStdout = true
				return nil
			},
		},
			flags.WithDeprecatedNames(flags.FlagNamesWithTerragruntPrefix(DeprecatedLogDisableFlagName), strictControl)),

		flags.NewFlag(&cli.BoolFlag{
			Name:        ShowLogAbsPathsFlagName,
			EnvVars:     flags.EnvVarsWithTgPrefix(ShowLogAbsPathsFlagName),
			ConfigKey:   flags.ConfigKey(ShowLogAbsPathsFlagName),
			Destination: &opts.LogShowAbsPaths,
			Usage:       "Show absolute paths in logs.",
		},
			flags.WithDeprecatedNames(flags.FlagNamesWithTerragruntPrefix(DeprecatedShowLogAbsPathsFlagName), strictControl)),

		flags.NewFlag(&cli.BoolFlag{
			Name:      NoColorFlagName,
			EnvVars:   flags.EnvVarsWithTgPrefix(NoColorFlagName),
			ConfigKey: flags.ConfigKey(NoColorFlagName),
			Usage:     "Disable color output.",
			Setter: func(val bool) error {
				opts.Logger.Formatter().SetDisabledColors(val)
				return nil
			},
		},
			flags.WithDeprecatedNames(flags.FlagNamesWithTerragruntPrefix(DeprecatedNoColorFlagName), strictControl)),

		flags.NewFlag(&cli.GenericFlag[string]{
			Name:      LogFormatFlagName,
			EnvVars:   flags.EnvVarsWithTgPrefix(LogFormatFlagName),
			ConfigKey: flags.ConfigKey(LogFormatFlagName),
			Usage:     "Set the log format.",
			Setter:    opts.Logger.Formatter().SetFormat,
			Action: func(_ *cli.Context, val string) error {
				switch val {
				case format.BareFormatName:
					opts.ForwardTFStdout = true
				case format.JSONFormatName:
					opts.JSONLogFormat = true
				}

				return nil
			},
		},
			flags.WithDeprecatedNames(flags.FlagNamesWithTerragruntPrefix(DeprecatedLogFormatFlagName), strictControl),
			flags.WithDeprecatedFlag(&cli.BoolFlag{
				Name:        flags.FlagNameWithTerragruntPrefix(DeprecatedDisableLogFormattingFlagName),
				EnvVars:     flags.EnvVarsWithTerragruntPrefix(DeprecatedDisableLogFormattingFlagName),
				Destination: &opts.DisableLogFormatting,
				Usage:       "If specified, logs will be displayed in key/value format. By default, logs are formatted in a human readable format.",
			}, flags.NewValue(format.KeyValueFormatName), legacyLogsControl),
			flags.WithDeprecatedFlag(&cli.BoolFlag{
				Name:        flags.FlagNameWithTerragruntPrefix(DeprecatedJSONLogFlagName),
				EnvVars:     flags.EnvVarsWithTerragruntPrefix(DeprecatedJSONLogFlagName),
				Destination: &opts.JSONLogFormat,
				Usage:       "If specified, Terragrunt will output its logs in JSON format.",
			}, flags.NewValue(format.JSONFormatName), legacyLogsControl),
			flags.WithDeprecatedFlag(&cli.BoolFlag{
				Name:    flags.FlagNameWithTerragruntPrefix(DeprecatedTfLogJSONFlagName),
				EnvVars: flags.EnvVarsWithTerragruntPrefix(DeprecatedTfLogJSONFlagName),
				Usage:   "If specified, Terragrunt will wrap Terraform stdout and stderr in JSON.",
			}, flags.NewValue(format.JSONFormatName), legacyLogsControl)),

		flags.NewFlag(&cli.GenericFlag[string]{
			Name:      LogCustomFormatFlagName,
			EnvVars:   flags.EnvVarsWithTgPrefix(LogCustomFormatFlagName),
			ConfigKey: flags.ConfigKey(LogCustomFormatFlagName),
			Usage:     "Set the custom log formatting.",
			Setter:    opts.Logger.Formatter().SetCustomFormat,
		},
			flags.WithDeprecatedNames(flags.FlagNamesWithTerragruntPrefix(DeprecatedLogCustomFormatFlagName), strictControl)),

		flags.NewFlag(&cli.BoolFlag{
			Name:        NonInteractiveFlagName,
			EnvVars:     flags.EnvVarsWithTgPrefix(NonInteractiveFlagName),
			ConfigKey:   flags.ConfigKey(NonInteractiveFlagName),
			Destination: &opts.NonInteractive,
			Usage:       `Assume "yes" for all prompts.`,
		},
			flags.WithDeprecatedNames(flags.FlagNamesWithTerragruntPrefix(DeprecatedNonInteractiveFlagName), strictControl),
			flags.WithDeprecatedFlag(&cli.BoolFlag{
				Negative: true,
				EnvVars:  flags.Name{}.EnvVars(DeprecatedTFInputFlagName),
			}, nil, strictControl)),

		// Experiment Mode flags.

		flags.NewFlag(&cli.BoolFlag{
			Name:      ExperimentModeFlagName,
			EnvVars:   flags.EnvVarsWithTgPrefix(ExperimentModeFlagName),
			ConfigKey: flags.ConfigKey(ExperimentModeFlagName),
			Usage:     "Enables experiment mode for Terragrunt. For more information, see https://terragrunt.gruntwork.io/docs/reference/experiment-mode .",
			Setter: func(_ bool) error {
				opts.Experiments.ExperimentMode()

				return nil
			},
		},
			flags.WithDeprecatedNames(flags.FlagNamesWithTerragruntPrefix(DeprecatedExperimentModeFlagName), strictControl)),

		flags.NewFlag(&cli.SliceFlag[string]{
			Name:      ExperimentFlagName,
			EnvVars:   flags.EnvVarsWithTgPrefix(ExperimentFlagName),
			ConfigKey: flags.ConfigKey(ExperimentFlagName),
			Usage:     "Enables specific experiments. For a list of available experiments, see https://terragrunt.gruntwork.io/docs/reference/experiment-mode .",
			Setter:    opts.Experiments.EnableExperiment,
			Action: func(_ *cli.Context, val []string) error {
				opts.Experiments.NotifyCompletedExperiments(opts.Logger)

				return nil
			},
		},
			flags.WithDeprecatedNames(flags.FlagNamesWithTerragruntPrefix(DeprecatedExperimentFlagName), strictControl)),

		// Strict Mode flags.

		flags.NewFlag(&cli.BoolFlag{
			Name:      StrictModeFlagName,
			EnvVars:   flags.EnvVarsWithTgPrefix(StrictModeFlagName),
			ConfigKey: flags.ConfigKey(StrictModeFlagName),
			Usage:     "Enables strict mode for Terragrunt. For more information, run 'terragrunt info strict'.",
			Setter: func(_ bool) error {
				opts.StrictControls.FilterByStatus(strict.ActiveStatus).Enable()

				return nil
			},
			Action: func(_ *cli.Context, _ bool) error {
				opts.StrictControls.LogEnabled(opts.Logger)

				return nil
			},
		},
			flags.WithDeprecatedNames(flags.FlagNamesWithTerragruntPrefix(DeprecatedStrictModeFlagName), strictControl)),

		flags.NewFlag(&cli.SliceFlag[string]{
			Name:      StrictControlFlagName,
			EnvVars:   flags.EnvVarsWithTgPrefix(StrictControlFlagName),
			ConfigKey: flags.ConfigKey(StrictControlFlagName),
			Usage:     "Enables specific strict controls. For a list of available controls, run 'terragrunt info strict'.",
			Setter: func(val string) error {
				return opts.StrictControls.EnableControl(val)
			},
			Action: func(_ *cli.Context, _ []string) error {
				opts.StrictControls.LogEnabled(opts.Logger)

				return nil
			},
		},
			flags.WithDeprecatedNames(flags.FlagNamesWithTerragruntPrefix(DeprecatedStrictControlFlagName), strictControl),
		),

		// Telemetry related flags.

		flags.NewFlag(&cli.GenericFlag[string]{
			EnvVars:     flags.EnvVarsWithTgPrefix(TelemetryTraceExporterFlagName),
			ConfigKey:   flags.ConfigKey(TelemetryTraceExporterFlagName),
			Destination: &opts.Telemetry.TraceExporter,
		},
			flags.WithDeprecatedNames(flags.FlagNamesWithTerragruntPrefix("telemetry-trace-exporter"), strictControl),
			flags.WithDeprecatedNames(flags.FlagNamesWithTerragruntPrefix("telemerty-trace-exporter"), strictControl)),

		flags.NewFlag(&cli.BoolFlag{
			EnvVars:     flags.EnvVarsWithTgPrefix(TelemetryTraceExporterInsecureEndpointFlagName),
			ConfigKey:   flags.ConfigKey(TelemetryTraceExporterInsecureEndpointFlagName),
			Destination: &opts.Telemetry.TraceExporterInsecureEndpoint,
		},
			flags.WithDeprecatedNames(flags.FlagNamesWithTerragruntPrefix("telemetry-trace-exporter-insecure-endpoint"), strictControl),
			flags.WithDeprecatedNames(flags.FlagNamesWithTerragruntPrefix("telemerty-trace-exporter-insecure-endpoint"), strictControl)),

		flags.NewFlag(&cli.GenericFlag[string]{
			EnvVars:     flags.EnvVarsWithTgPrefix(TelemetryTraceExporterHTTPEndpointFlagName),
			ConfigKey:   flags.ConfigKey(TelemetryTraceExporterHTTPEndpointFlagName),
			Destination: &opts.Telemetry.TraceExporterHTTPEndpoint,
		},
			flags.WithDeprecatedNames(flags.FlagNamesWithTerragruntPrefix("telemetry-trace-exporter-http-endpoint"), strictControl),
			flags.WithDeprecatedNames(flags.FlagNamesWithTerragruntPrefix("telemerty-trace-exporter-http-endpoint"), strictControl)),

		flags.NewFlag(&cli.GenericFlag[string]{
			EnvVars:     flags.Name{}.EnvVars(TraceparentFlagName),
			ConfigKey:   flags.ConfigKey(TraceparentFlagName),
			Destination: &opts.Telemetry.TraceParent,
		}),
		flags.NewFlag(&cli.GenericFlag[string]{
			EnvVars:     flags.EnvVarsWithTgPrefix(TelemetryMetricExporterFlagName),
			ConfigKey:   flags.ConfigKey(TelemetryMetricExporterFlagName),
			Destination: &opts.Telemetry.MetricExporter,
		},
			flags.WithDeprecatedNames(flags.FlagNamesWithTerragruntPrefix("telemetry-metric-exporter"), strictControl),
			flags.WithDeprecatedNames(flags.FlagNamesWithTerragruntPrefix("telemerty-metric-exporter"), strictControl)),

		flags.NewFlag(&cli.BoolFlag{
			EnvVars:     flags.EnvVarsWithTgPrefix(TelemetryMetricExporterInsecureEndpointFlagName),
			ConfigKey:   flags.ConfigKey(TelemetryMetricExporterInsecureEndpointFlagName),
			Destination: &opts.Telemetry.MetricExporterInsecureEndpoint,
		},
			flags.WithDeprecatedNames(flags.FlagNamesWithTerragruntPrefix("telemetry-metric-exporter-insecure-endpoint"), strictControl),
			flags.WithDeprecatedNames(flags.FlagNamesWithTerragruntPrefix("telemerty-metric-exporter-insecure-endpoint"), strictControl)),
	}

	flags = flags.Sort()
	flags = flags.Add(NewHelpVersionFlags(opts)...)

	return flags
}

func NewHelpVersionFlags(opts *options.TerragruntOptions) cli.Flags {
	return cli.Flags{
		&cli.BoolFlag{
			Name:    HelpFlagName,  // --help, -help
			Aliases: []string{"h"}, //  -h
			Usage:   "Show help.",
			Action: func(ctx *cli.Context, _ bool) error {
				return help.Action(ctx, opts)
			},
		},
		&cli.BoolFlag{
			Name:    VersionFlagName, // --version, -version
			Aliases: []string{"v"},   //  -v
			Usage:   "Show terragrunt version.",
			Action: func(ctx *cli.Context, _ bool) (err error) {
				return version.Action(ctx)
			},
		},
	}
}
