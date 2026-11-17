// Package global provides CLI global flags.
package global

import (
	"context"
	"fmt"
	"slices"

	"github.com/gruntwork-io/terragrunt/internal/cli/commands/help"
	"github.com/gruntwork-io/terragrunt/internal/cli/commands/version"
	"github.com/gruntwork-io/terragrunt/internal/cli/flags"
	"github.com/gruntwork-io/terragrunt/internal/clihelper"
	"github.com/gruntwork-io/terragrunt/internal/experiment"
	"github.com/gruntwork-io/terragrunt/internal/strict"
	"github.com/gruntwork-io/terragrunt/internal/strict/controls"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/pkg/log/format"
	"github.com/gruntwork-io/terragrunt/pkg/options"
)

const (
	// Logs related flags.

	// LogLevelFlagName is the flag name for `--log-level`, the minimum level Terragrunt logs at.
	LogLevelFlagName = "log-level"
	// LogDisableFlagName is the flag name for `--log-disable`, which turns off Terragrunt logging.
	LogDisableFlagName = "log-disable"
	// ShowLogAbsPathsFlagName is the flag name for `--log-show-abs-paths`, which logs absolute paths.
	ShowLogAbsPathsFlagName = "log-show-abs-paths"
	// LogFormatFlagName is the flag name for `--log-format`, which picks a preset log format.
	LogFormatFlagName = "log-format"
	// LogCustomFormatFlagName is the flag name for `--log-custom-format`, a custom log format string.
	LogCustomFormatFlagName = "log-custom-format"
	// NoColorFlagName is the flag name for `--no-color`, which turns off colored output.
	NoColorFlagName = "no-color"

	// NonInteractiveFlagName is the flag name for `--non-interactive`, which answers "yes" to every prompt.
	NonInteractiveFlagName = "non-interactive"
	// WorkingDirFlagName is the flag name for `--working-dir`, the directory Terragrunt runs in.
	WorkingDirFlagName = "working-dir"

	// Strict Mode related flags.

	// StrictModeFlagName is the flag name for `--strict-mode`, which enables every active strict control.
	StrictModeFlagName = "strict-mode"
	// StrictControlFlagName is the flag name for `--strict-control`, which enables the named strict controls.
	StrictControlFlagName = "strict-control"

	// Experiment Mode related flags/envs.

	// ExperimentModeFlagName is the flag name for `--experiment-mode`, which enables every ongoing experiment.
	ExperimentModeFlagName = "experiment-mode"
	// ExperimentFlagName is the flag name for `--experiment`, which enables the named experiments.
	ExperimentFlagName = "experiment"

	// Tips related flags.

	// NoTipsFlagName is the flag name for `--no-tips`, which hides every tip.
	NoTipsFlagName = "no-tips"
	// NoTipFlagName is the flag name for `--no-tip`, which hides the named tips.
	NoTipFlagName = "no-tip"

	// App flags.

	// HelpFlagName is the flag name for `--help`, which prints help for the app or command.
	HelpFlagName = "help"
	// VersionFlagName is the flag name for `--version`, which prints the Terragrunt version.
	VersionFlagName = "version"

	// Telemetry flags.

	// TelemetryTraceExporterFlagName is the flag name behind the `TG_TELEMETRY_TRACE_EXPORTER` env var,
	// which selects the trace exporter.
	TelemetryTraceExporterFlagName = "telemetry-trace-exporter"
	// TelemetryTraceExporterInsecureEndpointFlagName is the flag name behind the
	// `TG_TELEMETRY_TRACE_EXPORTER_INSECURE_ENDPOINT` env var, which sends traces without TLS.
	TelemetryTraceExporterInsecureEndpointFlagName = "telemetry-trace-exporter-insecure-endpoint"
	// TelemetryTraceExporterHTTPEndpointFlagName is the flag name behind the
	// `TG_TELEMETRY_TRACE_EXPORTER_HTTP_ENDPOINT` env var, the endpoint the HTTP trace exporter sends to.
	TelemetryTraceExporterHTTPEndpointFlagName = "telemetry-trace-exporter-http-endpoint"
	// TraceparentFlagName is the flag name behind the unprefixed `TRACEPARENT` env var, the W3C trace
	// context that Terragrunt's spans join.
	TraceparentFlagName = "traceparent"
	// TelemetryMetricExporterFlagName is the flag name behind the `TG_TELEMETRY_METRIC_EXPORTER` env var,
	// which selects the metric exporter.
	TelemetryMetricExporterFlagName = "telemetry-metric-exporter"
	// TelemetryMetricExporterInsecureEndpointFlagName is the flag name behind the
	// `TG_TELEMETRY_METRIC_EXPORTER_INSECURE_ENDPOINT` env var, which sends metrics without TLS.
	TelemetryMetricExporterInsecureEndpointFlagName = "telemetry-metric-exporter-insecure-endpoint"
	// TelemetryLogsExporterFlagName is the flag name behind the `TG_TELEMETRY_LOGS_EXPORTER` env var,
	// which selects the logs exporter.
	TelemetryLogsExporterFlagName = "telemetry-logs-exporter"
	// TelemetryLogsExporterInsecureEndpointFlagName is the flag name behind the
	// `TG_TELEMETRY_LOGS_EXPORTER_INSECURE_ENDPOINT` env var, which sends logs without TLS.
	TelemetryLogsExporterInsecureEndpointFlagName = "telemetry-logs-exporter-insecure-endpoint"

	// Profiling flags.

	// ProfileCPUFlagName is the flag name for `--profile-cpu`, the path to write a CPU profile to.
	ProfileCPUFlagName = "profile-cpu"
	// ProfileMemFlagName is the flag name for `--profile-mem`, the path to write a heap profile to.
	ProfileMemFlagName = "profile-mem"
	// ProfileGoroutineFlagName is the flag name for `--profile-goroutine`, the path to write a goroutine
	// profile to.
	ProfileGoroutineFlagName = "profile-goroutine"
	// ProfileDirFlagName is the flag name for `--profile-dir`, the directory to write the CPU, heap, and
	// goroutine profiles to.
	ProfileDirFlagName = "profile-dir"

	// Renamed flags.

	// DeprecatedLogLevelFlagName is the flag name behind the deprecated `TERRAGRUNT_LOG_LEVEL` env var.
	DeprecatedLogLevelFlagName = "log-level"
	// DeprecatedLogDisableFlagName is the flag name behind the deprecated `TERRAGRUNT_LOG_DISABLE` env var.
	DeprecatedLogDisableFlagName = "log-disable"
	// DeprecatedShowLogAbsPathsFlagName is the flag name behind the deprecated
	// `TERRAGRUNT_LOG_SHOW_ABS_PATHS` env var.
	DeprecatedShowLogAbsPathsFlagName = "log-show-abs-paths"
	// DeprecatedLogFormatFlagName is the flag name behind the deprecated `TERRAGRUNT_LOG_FORMAT` env var.
	DeprecatedLogFormatFlagName = "log-format"
	// DeprecatedLogCustomFormatFlagName is the flag name behind the deprecated
	// `TERRAGRUNT_LOG_CUSTOM_FORMAT` env var.
	DeprecatedLogCustomFormatFlagName = "log-custom-format"
	// DeprecatedNoColorFlagName is the flag name behind the deprecated `TERRAGRUNT_NO_COLOR` env var.
	DeprecatedNoColorFlagName = "no-color"
	// DeprecatedNonInteractiveFlagName is the flag name behind the deprecated
	// `TERRAGRUNT_NON_INTERACTIVE` env var.
	DeprecatedNonInteractiveFlagName = "non-interactive"
	// DeprecatedTFInputFlagName is the flag name behind the deprecated, unprefixed `TF_INPUT` env var,
	// read as the negation of `--non-interactive`.
	DeprecatedTFInputFlagName = "tf-input"
	// DeprecatedWorkingDirFlagName is the flag name behind the deprecated `TERRAGRUNT_WORKING_DIR` env var.
	DeprecatedWorkingDirFlagName = "working-dir"
	// DeprecatedStrictModeFlagName is the flag name behind the deprecated `TERRAGRUNT_STRICT_MODE` env var.
	DeprecatedStrictModeFlagName = "strict-mode"
	// DeprecatedStrictControlFlagName is the flag name behind the deprecated
	// `TERRAGRUNT_STRICT_CONTROL` env var.
	DeprecatedStrictControlFlagName = "strict-control"
	// DeprecatedExperimentModeFlagName is the flag name behind the deprecated
	// `TERRAGRUNT_EXPERIMENT_MODE` env var.
	DeprecatedExperimentModeFlagName = "experiment-mode"
	// DeprecatedExperimentFlagName is the flag name behind the deprecated `TERRAGRUNT_EXPERIMENT` env var.
	DeprecatedExperimentFlagName = "experiment"

	// Deprecated flags.

	// DeprecatedDisableLogFormattingFlagName is the flag name behind the deprecated
	// `TERRAGRUNT_DISABLE_LOG_FORMATTING` env var, superseded by `--log-format`.
	DeprecatedDisableLogFormattingFlagName = "disable-log-formatting"
	// DeprecatedJSONLogFlagName is the flag name behind the deprecated `TERRAGRUNT_JSON_LOG` env var,
	// superseded by `--log-format`.
	DeprecatedJSONLogFlagName = "json-log"
	// DeprecatedTfLogJSONFlagName is the flag name behind the deprecated `TERRAGRUNT_TF_LOGS_TO_JSON`
	// env var, superseded by `--log-format`.
	DeprecatedTfLogJSONFlagName = "tf-logs-to-json"
)

// NewFlags creates and returns global flags common for all commands.
func NewFlags(l log.Logger, opts *options.TerragruntOptions, prefix flags.Prefix) clihelper.Flags {
	tgPrefix := prefix.Prepend(flags.TgPrefix)
	terragruntPrefix := prefix.Prepend(flags.TerragruntPrefix)
	flags := clihelper.Flags{
		NewLogLevelFlag(l, opts, prefix),

		flags.NewFlag(
			&clihelper.GenericFlag[string]{
				Name:        WorkingDirFlagName,
				EnvVars:     tgPrefix.EnvVars(WorkingDirFlagName),
				Destination: &opts.WorkingDir,
				Usage:       "The path to the directory of Terragrunt configurations. Default is current directory.",
			},
			flags.WithDeprecatedEnvVars(
				terragruntPrefix.EnvVars(DeprecatedWorkingDirFlagName),
				opts.StrictControls,
			),
		),

		flags.NewFlag(
			&clihelper.BoolFlag{
				Name:    LogDisableFlagName,
				EnvVars: tgPrefix.EnvVars(LogDisableFlagName),
				Usage:   "Disable logging.",
				Setter: func(val bool) error {
					l.Formatter().SetDisabledOutput(val)

					if val {
						opts.ForwardTFStdout = true
					}

					return nil
				},
			},
			flags.WithDeprecatedEnvVars(
				terragruntPrefix.EnvVars(DeprecatedLogDisableFlagName),
				opts.StrictControls,
			),
		),

		flags.NewFlag(
			&clihelper.BoolFlag{
				Name:        ShowLogAbsPathsFlagName,
				EnvVars:     tgPrefix.EnvVars(ShowLogAbsPathsFlagName),
				Destination: &opts.LogShowAbsPaths,
				Usage:       "Show absolute paths in logs.",
			},
			flags.WithDeprecatedEnvVars(
				terragruntPrefix.EnvVars(DeprecatedShowLogAbsPathsFlagName),
				opts.StrictControls,
			),
		),

		flags.NewFlag(
			&clihelper.BoolFlag{
				Name:    NoColorFlagName,
				EnvVars: tgPrefix.EnvVars(NoColorFlagName),
				Usage:   "Disable color output.",
				Setter: func(val bool) error {
					l.Formatter().SetDisabledColors(val)
					return nil
				},
			},
			flags.WithDeprecatedEnvVars(
				terragruntPrefix.EnvVars(DeprecatedNoColorFlagName),
				opts.StrictControls,
			),
		),

		flags.NewFlag(
			&clihelper.GenericFlag[string]{
				Name:    LogFormatFlagName,
				EnvVars: tgPrefix.EnvVars(LogFormatFlagName),
				Usage:   "Set the log format.",
				Setter:  l.Formatter().SetFormat,
				Action: func(_ context.Context, _ *clihelper.Context, val string) error {
					switch val {
					case format.BareFormatName:
						opts.ForwardTFStdout = true
					case format.JSONFormatName:
						opts.JSONLogFormat = true
					}

					return nil
				},
			},
			flags.WithDeprecatedEnvVars(
				terragruntPrefix.EnvVars(DeprecatedLogFormatFlagName),
				opts.StrictControls,
			),
			flags.WithDeprecatedEnvVars(
				terragruntPrefix.EnvVars(DeprecatedDisableLogFormattingFlagName),
				opts.StrictControls,
				controls.LegacyLogs,
			),
			flags.WithDeprecatedEnvVars(
				terragruntPrefix.EnvVars(DeprecatedJSONLogFlagName),
				opts.StrictControls,
				controls.LegacyLogs,
			),
			flags.WithDeprecatedEnvVars(
				terragruntPrefix.EnvVars(DeprecatedTfLogJSONFlagName),
				opts.StrictControls,
				controls.LegacyLogs,
			),
		),

		flags.NewFlag(
			&clihelper.GenericFlag[string]{
				Name:    LogCustomFormatFlagName,
				EnvVars: tgPrefix.EnvVars(LogCustomFormatFlagName),
				Usage:   "Set the custom log formatting.",
				Setter:  l.Formatter().SetCustomFormat,
			},
			flags.WithDeprecatedEnvVars(
				terragruntPrefix.EnvVars(DeprecatedLogCustomFormatFlagName),
				opts.StrictControls,
			),
		),

		flags.NewFlag(
			&clihelper.BoolFlag{
				Name:        NonInteractiveFlagName,
				EnvVars:     tgPrefix.EnvVars(NonInteractiveFlagName),
				Destination: &opts.NonInteractive,
				Usage:       `Assume "yes" for all prompts.`,
			},
			flags.WithDeprecatedEnvVars(
				terragruntPrefix.EnvVars(DeprecatedNonInteractiveFlagName),
				opts.StrictControls,
			),
			flags.WithDeprecatedFlag(&clihelper.BoolFlag{
				Negative: true,
				EnvVars:  flags.Prefix{}.EnvVars(DeprecatedTFInputFlagName),
			}, nil, opts.StrictControls),
		),

		// Experiment Mode flags.

		flags.NewFlag(
			&clihelper.BoolFlag{
				Name:    ExperimentModeFlagName,
				EnvVars: tgPrefix.EnvVars(ExperimentModeFlagName),
				Usage:   "Enables experiment mode for Terragrunt. For more information, see https://docs.terragrunt.com/reference/experiment-mode .",
				Setter: func(_ bool) error {
					opts.Experiments.ExperimentMode()

					return nil
				},
			},
			flags.WithDeprecatedEnvVars(
				terragruntPrefix.EnvVars(DeprecatedExperimentModeFlagName),
				opts.StrictControls,
			),
		),

		flags.NewFlag(
			&clihelper.SliceFlag[string]{
				Name:    ExperimentFlagName,
				EnvVars: tgPrefix.EnvVars(ExperimentFlagName),
				Usage:   "Enables specific experiments. For a list of available experiments, see https://docs.terragrunt.com/reference/experiment-mode .",
				Setter:  opts.Experiments.EnableExperiment,
				Action: func(_ context.Context, _ *clihelper.Context, _ []string) error {
					opts.Experiments.NotifyCompletedExperiments(l)

					return nil
				},
			},
			flags.WithDeprecatedEnvVars(
				terragruntPrefix.EnvVars(DeprecatedExperimentFlagName),
				opts.StrictControls,
			),
		),

		// Tips Mode flags.

		flags.NewFlag(&clihelper.BoolFlag{
			Name:    NoTipsFlagName,
			EnvVars: tgPrefix.EnvVars(NoTipsFlagName),
			Usage:   "Disable all tips from being displayed.",
			Setter: func(v bool) error {
				if v {
					opts.Tips.DisableAll()
				}

				return nil
			},
		}),

		flags.NewFlag(&clihelper.SliceFlag[string]{
			Name:    NoTipFlagName,
			EnvVars: tgPrefix.EnvVars(NoTipFlagName),
			Usage:   "Disable specific tips from being displayed.",
			Setter:  opts.Tips.DisableTip,
		}),

		// Strict Mode flags.

		flags.NewFlag(
			&clihelper.BoolFlag{
				Name:    StrictModeFlagName,
				EnvVars: tgPrefix.EnvVars(StrictModeFlagName),
				Usage:   "Enables strict mode for Terragrunt. For more information, run 'terragrunt info strict'.",
				Setter: func(_ bool) error {
					opts.StrictControls.FilterByStatus(strict.ActiveStatus).Enable()

					return nil
				},
				Action: func(_ context.Context, _ *clihelper.Context, _ bool) error {
					opts.StrictControls.LogEnabled(l)

					return nil
				},
			},
			flags.WithDeprecatedEnvVars(
				terragruntPrefix.EnvVars(DeprecatedStrictModeFlagName),
				opts.StrictControls,
			),
		),

		flags.NewFlag(
			&clihelper.SliceFlag[string]{
				Name:    StrictControlFlagName,
				EnvVars: tgPrefix.EnvVars(StrictControlFlagName),
				Usage:   "Enables specific strict controls. For a list of available controls, run 'terragrunt info strict'.",
				Setter: func(val string) error {
					return opts.StrictControls.EnableControl(val)
				},
				Action: func(_ context.Context, _ *clihelper.Context, vals []string) error {
					opts.StrictControls.LogEnabled(l)
					opts.StrictControls.LogCompletedControls(l, vals)

					return nil
				},
			},
			flags.WithDeprecatedEnvVars(
				terragruntPrefix.EnvVars(DeprecatedStrictControlFlagName),
				opts.StrictControls,
			),
		),
	}

	flags = flags.Add(NewTelemetryFlags(opts, nil)...)
	flags = flags.Add(NewProfileFlags(opts, nil)...)
	flags = flags.Sort()
	flags = flags.Add(NewHelpVersionFlags(l, opts)...)

	return flags
}

// NewTelemetryFlags creates telemetry related flags.
func NewTelemetryFlags(opts *options.TerragruntOptions, prefix flags.Prefix) clihelper.Flags {
	tgPrefix := prefix.Prepend(flags.TgPrefix)
	terragruntPrefix := prefix.Prepend(flags.TerragruntPrefix)

	return clihelper.Flags{
		flags.NewFlag(
			&clihelper.GenericFlag[string]{
				EnvVars:     tgPrefix.EnvVars(TelemetryTraceExporterFlagName),
				Destination: &opts.Telemetry.TraceExporter,
			},
			flags.WithDeprecatedEnvVars(
				terragruntPrefix.EnvVars("telemetry-trace-exporter"),
				opts.StrictControls,
			),
			flags.WithDeprecatedEnvVars(
				terragruntPrefix.EnvVars("telemerty-trace-exporter"),
				opts.StrictControls,
			),
		),

		flags.NewFlag(
			&clihelper.BoolFlag{
				EnvVars:     tgPrefix.EnvVars(TelemetryTraceExporterInsecureEndpointFlagName),
				Destination: &opts.Telemetry.TraceExporterInsecureEndpoint,
			},
			flags.WithDeprecatedEnvVars(
				terragruntPrefix.EnvVars("telemetry-trace-exporter-insecure-endpoint"),
				opts.StrictControls,
			),
			flags.WithDeprecatedEnvVars(
				terragruntPrefix.EnvVars("telemerty-trace-exporter-insecure-endpoint"),
				opts.StrictControls,
			),
		),

		flags.NewFlag(
			&clihelper.GenericFlag[string]{
				EnvVars:     tgPrefix.EnvVars(TelemetryTraceExporterHTTPEndpointFlagName),
				Destination: &opts.Telemetry.TraceExporterHTTPEndpoint,
			},
			flags.WithDeprecatedEnvVars(
				terragruntPrefix.EnvVars("telemetry-trace-exporter-http-endpoint"),
				opts.StrictControls,
			),
			flags.WithDeprecatedEnvVars(
				terragruntPrefix.EnvVars("telemerty-trace-exporter-http-endpoint"),
				opts.StrictControls,
			),
		),

		flags.NewFlag(&clihelper.GenericFlag[string]{
			EnvVars:     flags.Prefix{}.EnvVars(TraceparentFlagName),
			Destination: &opts.Telemetry.TraceParent,
		}),
		flags.NewFlag(
			&clihelper.GenericFlag[string]{
				EnvVars:     tgPrefix.EnvVars(TelemetryMetricExporterFlagName),
				Destination: &opts.Telemetry.MetricExporter,
			},
			flags.WithDeprecatedEnvVars(
				terragruntPrefix.EnvVars("telemetry-metric-exporter"),
				opts.StrictControls,
			),
			flags.WithDeprecatedEnvVars(
				terragruntPrefix.EnvVars("telemerty-metric-exporter"),
				opts.StrictControls,
			),
		),

		flags.NewFlag(
			&clihelper.BoolFlag{
				EnvVars:     tgPrefix.EnvVars(TelemetryMetricExporterInsecureEndpointFlagName),
				Destination: &opts.Telemetry.MetricExporterInsecureEndpoint,
			},
			flags.WithDeprecatedEnvVars(
				terragruntPrefix.EnvVars("telemetry-metric-exporter-insecure-endpoint"),
				opts.StrictControls,
			),
			flags.WithDeprecatedEnvVars(
				terragruntPrefix.EnvVars("telemerty-metric-exporter-insecure-endpoint"),
				opts.StrictControls,
			),
		),

		flags.NewFlag(&clihelper.GenericFlag[string]{
			EnvVars:     tgPrefix.EnvVars(TelemetryLogsExporterFlagName),
			Destination: &opts.Telemetry.LogsExporter,
		}),

		flags.NewFlag(&clihelper.BoolFlag{
			EnvVars:     tgPrefix.EnvVars(TelemetryLogsExporterInsecureEndpointFlagName),
			Destination: &opts.Telemetry.LogsExporterInsecureEndpoint,
		}),
	}
}

// NewProfileFlags creates the profiling flags backing the opts.Profile* fields.
func NewProfileFlags(opts *options.TerragruntOptions, prefix flags.Prefix) clihelper.Flags {
	tgPrefix := prefix.Prepend(flags.TgPrefix)

	return clihelper.Flags{
		flags.NewFlag(&clihelper.GenericFlag[string]{
			Name:        ProfileCPUFlagName,
			EnvVars:     tgPrefix.EnvVars(ProfileCPUFlagName),
			Destination: &opts.ProfileCPU,
			Usage:       profileFlagUsage(opts, "Write a CPU profile to the given path."),
		}),

		flags.NewFlag(&clihelper.GenericFlag[string]{
			Name:        ProfileMemFlagName,
			EnvVars:     tgPrefix.EnvVars(ProfileMemFlagName),
			Destination: &opts.ProfileMem,
			Usage:       profileFlagUsage(opts, "Write a memory (heap) profile to the given path."),
		}),

		flags.NewFlag(&clihelper.GenericFlag[string]{
			Name:        ProfileGoroutineFlagName,
			EnvVars:     tgPrefix.EnvVars(ProfileGoroutineFlagName),
			Destination: &opts.ProfileGoroutine,
			Usage:       profileFlagUsage(opts, "Write a goroutine profile to the given path."),
		}),

		flags.NewFlag(&clihelper.GenericFlag[string]{
			Name:        ProfileDirFlagName,
			EnvVars:     tgPrefix.EnvVars(ProfileDirFlagName),
			Destination: &opts.ProfileDir,
			Usage:       profileFlagUsage(opts, "Directory to write profile files (cpu, mem, goroutine)."),
		}),
	}
}

// profileFlagUsage keeps the experiment note on profiling flags until the experiment completes.
func profileFlagUsage(opts *options.TerragruntOptions, usage string) string {
	return flags.ExperimentUsage(opts.Experiments, experiment.Profiling, usage)
}

// NewLogLevelFlag returns the `--log-level` flag, which sets the level of l. The levels "panic"
// and "fatal" turn off Terragrunt logging and forward tofu stdout.
func NewLogLevelFlag(
	l log.Logger,
	opts *options.TerragruntOptions,
	prefix flags.Prefix,
) *flags.Flag {
	tgPrefix := prefix.Prepend(flags.TgPrefix)
	terragruntPrefix := prefix.Prepend(flags.TerragruntPrefix)

	return flags.NewFlag(&clihelper.GenericFlag[string]{
		Name:        LogLevelFlagName,
		EnvVars:     tgPrefix.EnvVars(LogLevelFlagName),
		DefaultText: l.Level().String(),
		Setter:      l.SetLevel,
		Usage: fmt.Sprintf(
			"Sets the logging level for Terragrunt. Supported levels: %s.",
			log.AllLevels,
		),
		Action: func(_ context.Context, _ *clihelper.Context, val string) error {
			// Before the release of v0.67.0, these levels actually disabled logs, since we do not use these levels for logging.
			// For backward compatibility we simulate the same behavior.
			removedLevels := []string{
				"panic",
				"fatal",
			}

			if slices.Contains(removedLevels, val) {
				opts.ForwardTFStdout = true

				l.Formatter().SetDisabledOutput(true)
			}

			return nil
		},
	}, flags.WithDeprecatedEnvVars(terragruntPrefix.EnvVars(DeprecatedLogLevelFlagName), opts.StrictControls))
}

// NewHelpVersionFlags returns the `--help` (`-h`) and `--version` (`-v`) flags.
func NewHelpVersionFlags(l log.Logger, opts *options.TerragruntOptions) clihelper.Flags {
	return clihelper.Flags{
		&clihelper.BoolFlag{
			Name:    HelpFlagName,  // --help, -help
			Aliases: []string{"h"}, //  -h
			Usage:   "Show help.",
			Action: func(ctx context.Context, cliCtx *clihelper.Context, _ bool) error {
				return help.Action(ctx, cliCtx, l, opts)
			},
		},
		&clihelper.BoolFlag{
			Name:    VersionFlagName, // --version, -version
			Aliases: []string{"v"},   //  -v
			Usage:   "Show terragrunt version.",
			Action: func(ctx context.Context, cliCtx *clihelper.Context, _ bool) (err error) {
				return version.Action(ctx, cliCtx)
			},
		},
	}
}
