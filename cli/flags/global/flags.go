// Package global provides CLI global flags.
package global

import (
	"fmt"

	"github.com/gruntwork-io/go-commons/collections"
	"github.com/gruntwork-io/terragrunt/cli/commands/help"
	"github.com/gruntwork-io/terragrunt/cli/commands/version"
	"github.com/gruntwork-io/terragrunt/cli/flags"
	"github.com/gruntwork-io/terragrunt/internal/cli"
	"github.com/gruntwork-io/terragrunt/internal/strict"
	"github.com/gruntwork-io/terragrunt/internal/strict/controls"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/pkg/log/format"
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

// NewFlags creates and returns global flags common for all commands.
func NewFlags(l log.Logger, opts *options.TerragruntOptions, prefix flags.Prefix) cli.Flags {
	tgPrefix := prefix.Prepend(flags.TgPrefix)
	terragruntPrefix := prefix.Prepend(flags.TerragruntPrefix)
	terragruntPrefixControl := flags.StrictControlsByGlobalFlags(opts.StrictControls)
	legacyLogsControl := flags.StrictControlsByGlobalFlags(opts.StrictControls, controls.LegacyLogs)

	flags := cli.Flags{
		NewLogLevelFlag(l, opts, prefix),

		flags.NewFlag(&cli.GenericFlag[string]{
			Name:        WorkingDirFlagName,
			EnvVars:     tgPrefix.EnvVars(WorkingDirFlagName),
			Destination: &opts.WorkingDir,
			Usage:       "The path to the directory of Terragrunt configurations. Default is current directory.",
		},
			flags.WithDeprecatedEnvVars(terragruntPrefix.EnvVars(DeprecatedWorkingDirFlagName), terragruntPrefixControl)),

		flags.NewFlag(&cli.BoolFlag{
			Name:    LogDisableFlagName,
			EnvVars: tgPrefix.EnvVars(LogDisableFlagName),
			Usage:   "Disable logging.",
			Setter: func(val bool) error {
				l.Formatter().SetDisabledOutput(val)
				opts.ForwardTFStdout = true
				return nil
			},
		},
			flags.WithDeprecatedEnvVars(terragruntPrefix.EnvVars(DeprecatedLogDisableFlagName), terragruntPrefixControl)),

		flags.NewFlag(&cli.BoolFlag{
			Name:        ShowLogAbsPathsFlagName,
			EnvVars:     tgPrefix.EnvVars(ShowLogAbsPathsFlagName),
			Destination: &opts.LogShowAbsPaths,
			Usage:       "Show absolute paths in logs.",
		},
			flags.WithDeprecatedEnvVars(terragruntPrefix.EnvVars(DeprecatedShowLogAbsPathsFlagName), terragruntPrefixControl)),

		flags.NewFlag(&cli.BoolFlag{
			Name:    NoColorFlagName,
			EnvVars: tgPrefix.EnvVars(NoColorFlagName),
			Usage:   "Disable color output.",
			Setter: func(val bool) error {
				l.Formatter().SetDisabledColors(val)
				return nil
			},
		},
			flags.WithDeprecatedEnvVars(terragruntPrefix.EnvVars(DeprecatedNoColorFlagName), terragruntPrefixControl)),

		flags.NewFlag(&cli.GenericFlag[string]{
			Name:    LogFormatFlagName,
			EnvVars: tgPrefix.EnvVars(LogFormatFlagName),
			Usage:   "Set the log format.",
			Setter:  l.Formatter().SetFormat,
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
			flags.WithDeprecatedEnvVars(terragruntPrefix.EnvVars(DeprecatedLogFormatFlagName), terragruntPrefixControl),
			flags.WithDeprecatedEnvVars(terragruntPrefix.EnvVars(DeprecatedDisableLogFormattingFlagName), legacyLogsControl),
			flags.WithDeprecatedEnvVars(terragruntPrefix.EnvVars(DeprecatedJSONLogFlagName), legacyLogsControl),
			flags.WithDeprecatedEnvVars(terragruntPrefix.EnvVars(DeprecatedTfLogJSONFlagName), legacyLogsControl)),

		flags.NewFlag(&cli.GenericFlag[string]{
			Name:    LogCustomFormatFlagName,
			EnvVars: tgPrefix.EnvVars(LogCustomFormatFlagName),
			Usage:   "Set the custom log formatting.",
			Setter:  l.Formatter().SetCustomFormat,
		},
			flags.WithDeprecatedEnvVars(terragruntPrefix.EnvVars(DeprecatedLogCustomFormatFlagName), terragruntPrefixControl)),

		flags.NewFlag(&cli.BoolFlag{
			Name:        NonInteractiveFlagName,
			EnvVars:     tgPrefix.EnvVars(NonInteractiveFlagName),
			Destination: &opts.NonInteractive,
			Usage:       `Assume "yes" for all prompts.`,
		},
			flags.WithDeprecatedEnvVars(terragruntPrefix.EnvVars(DeprecatedNonInteractiveFlagName), terragruntPrefixControl),
			flags.WithDeprecatedFlag(&cli.BoolFlag{
				Negative: true,
				EnvVars:  flags.Prefix{}.EnvVars(DeprecatedTFInputFlagName),
			}, nil, terragruntPrefixControl)),

		// Experiment Mode flags.

		flags.NewFlag(&cli.BoolFlag{
			Name:    ExperimentModeFlagName,
			EnvVars: tgPrefix.EnvVars(ExperimentModeFlagName),
			Usage:   "Enables experiment mode for Terragrunt. For more information, see https://terragrunt.gruntwork.io/docs/reference/experiment-mode .",
			Setter: func(_ bool) error {
				opts.Experiments.ExperimentMode()

				return nil
			},
		},
			flags.WithDeprecatedEnvVars(terragruntPrefix.EnvVars(DeprecatedExperimentModeFlagName), terragruntPrefixControl)),

		flags.NewFlag(&cli.SliceFlag[string]{
			Name:    ExperimentFlagName,
			EnvVars: tgPrefix.EnvVars(ExperimentFlagName),
			Usage:   "Enables specific experiments. For a list of available experiments, see https://terragrunt.gruntwork.io/docs/reference/experiment-mode .",
			Setter:  opts.Experiments.EnableExperiment,
			Action: func(_ *cli.Context, val []string) error {
				opts.Experiments.NotifyCompletedExperiments(l)

				return nil
			},
		},
			flags.WithDeprecatedEnvVars(terragruntPrefix.EnvVars(DeprecatedExperimentFlagName), terragruntPrefixControl)),

		// Strict Mode flags.

		flags.NewFlag(&cli.BoolFlag{
			Name:    StrictModeFlagName,
			EnvVars: tgPrefix.EnvVars(StrictModeFlagName),
			Usage:   "Enables strict mode for Terragrunt. For more information, run 'terragrunt info strict'.",
			Setter: func(_ bool) error {
				opts.StrictControls.FilterByStatus(strict.ActiveStatus).Enable()

				return nil
			},
			Action: func(_ *cli.Context, _ bool) error {
				opts.StrictControls.LogEnabled(l)

				return nil
			},
		},
			flags.WithDeprecatedEnvVars(terragruntPrefix.EnvVars(DeprecatedStrictModeFlagName), terragruntPrefixControl)),

		flags.NewFlag(&cli.SliceFlag[string]{
			Name:    StrictControlFlagName,
			EnvVars: tgPrefix.EnvVars(StrictControlFlagName),
			Usage:   "Enables specific strict controls. For a list of available controls, run 'terragrunt info strict'.",
			Setter: func(val string) error {
				return opts.StrictControls.EnableControl(val)
			},
			Action: func(_ *cli.Context, _ []string) error {
				opts.StrictControls.LogEnabled(l)

				return nil
			},
		},
			flags.WithDeprecatedEnvVars(terragruntPrefix.EnvVars(DeprecatedStrictControlFlagName), terragruntPrefixControl)),
	}

	flags = flags.Add(NewTelemetryFlags(opts, nil)...)
	flags = flags.Sort()
	flags = flags.Add(NewHelpVersionFlags(l, opts)...)

	return flags
}

// NewTelemetryFlags creates telemetry related flags.
func NewTelemetryFlags(opts *options.TerragruntOptions, prefix flags.Prefix) cli.Flags {
	tgPrefix := prefix.Prepend(flags.TgPrefix)
	terragruntPrefix := prefix.Prepend(flags.TerragruntPrefix)
	terragruntPrefixControl := flags.StrictControlsByGlobalFlags(opts.StrictControls)

	return cli.Flags{
		flags.NewFlag(&cli.GenericFlag[string]{
			EnvVars:     tgPrefix.EnvVars(TelemetryTraceExporterFlagName),
			Destination: &opts.Telemetry.TraceExporter,
		},
			flags.WithDeprecatedEnvVars(terragruntPrefix.EnvVars("telemetry-trace-exporter"), terragruntPrefixControl),
			flags.WithDeprecatedEnvVars(terragruntPrefix.EnvVars("telemerty-trace-exporter"), terragruntPrefixControl)),

		flags.NewFlag(&cli.BoolFlag{
			EnvVars:     tgPrefix.EnvVars(TelemetryTraceExporterInsecureEndpointFlagName),
			Destination: &opts.Telemetry.TraceExporterInsecureEndpoint,
		},
			flags.WithDeprecatedEnvVars(terragruntPrefix.EnvVars("telemetry-trace-exporter-insecure-endpoint"), terragruntPrefixControl),
			flags.WithDeprecatedEnvVars(terragruntPrefix.EnvVars("telemerty-trace-exporter-insecure-endpoint"), terragruntPrefixControl)),

		flags.NewFlag(&cli.GenericFlag[string]{
			EnvVars:     tgPrefix.EnvVars(TelemetryTraceExporterHTTPEndpointFlagName),
			Destination: &opts.Telemetry.TraceExporterHTTPEndpoint,
		},
			flags.WithDeprecatedEnvVars(terragruntPrefix.EnvVars("telemetry-trace-exporter-http-endpoint"), terragruntPrefixControl),
			flags.WithDeprecatedEnvVars(terragruntPrefix.EnvVars("telemerty-trace-exporter-http-endpoint"), terragruntPrefixControl)),

		flags.NewFlag(&cli.GenericFlag[string]{
			EnvVars:     flags.Prefix{}.EnvVars(TraceparentFlagName),
			Destination: &opts.Telemetry.TraceParent,
		}),
		flags.NewFlag(&cli.GenericFlag[string]{
			EnvVars:     tgPrefix.EnvVars(TelemetryMetricExporterFlagName),
			Destination: &opts.Telemetry.MetricExporter,
		},
			flags.WithDeprecatedEnvVars(terragruntPrefix.EnvVars("telemetry-metric-exporter"), terragruntPrefixControl),
			flags.WithDeprecatedEnvVars(terragruntPrefix.EnvVars("telemerty-metric-exporter"), terragruntPrefixControl)),

		flags.NewFlag(&cli.BoolFlag{
			EnvVars:     tgPrefix.EnvVars(TelemetryMetricExporterInsecureEndpointFlagName),
			Destination: &opts.Telemetry.MetricExporterInsecureEndpoint,
		},
			flags.WithDeprecatedEnvVars(terragruntPrefix.EnvVars("telemetry-metric-exporter-insecure-endpoint"), terragruntPrefixControl),
			flags.WithDeprecatedEnvVars(terragruntPrefix.EnvVars("telemerty-metric-exporter-insecure-endpoint"), terragruntPrefixControl)),
	}
}

func NewLogLevelFlag(l log.Logger, opts *options.TerragruntOptions, prefix flags.Prefix) *flags.Flag {
	tgPrefix := prefix.Prepend(flags.TgPrefix)
	terragruntPrefix := prefix.Prepend(flags.TerragruntPrefix)
	terragruntPrefixControl := flags.StrictControlsByGlobalFlags(opts.StrictControls)

	return flags.NewFlag(&cli.GenericFlag[string]{
		Name:        LogLevelFlagName,
		EnvVars:     tgPrefix.EnvVars(LogLevelFlagName),
		DefaultText: l.Level().String(),
		Setter:      l.SetLevel,
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
				l.Formatter().SetDisabledOutput(true)
			}

			return nil
		},
	}, flags.WithDeprecatedEnvVars(terragruntPrefix.EnvVars(DeprecatedLogLevelFlagName), terragruntPrefixControl))
}

func NewHelpVersionFlags(l log.Logger, opts *options.TerragruntOptions) cli.Flags {
	return cli.Flags{
		&cli.BoolFlag{
			Name:    HelpFlagName,  // --help, -help
			Aliases: []string{"h"}, //  -h
			Usage:   "Show help.",
			Action: func(ctx *cli.Context, _ bool) error {
				return help.Action(ctx, l, opts)
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
