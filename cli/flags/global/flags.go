// Package global provides CLI global flags.
package global

import (
	"fmt"

	"slices"

	"github.com/gruntwork-io/go-commons/collections"
	"github.com/gruntwork-io/terragrunt/cli/commands"
	"github.com/gruntwork-io/terragrunt/cli/commands/help"
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

	// App flags.

	HelpFlagName    = "help"
	VersionFlagName = "version"

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

// NewFlagsWithDeprecatedMovedFlags returns global flags along with flags that have been moved to other commands and hidden from CLI help.
func NewFlagsWithDeprecatedMovedFlags(opts *options.TerragruntOptions) cli.Flags {
	globalFlags := NewFlags(opts, nil)

	// Since the flags we take from the command may already have deprecated flags,
	// and we create new strict controls each time we call `commands.New` which will then be evaluated,
	// we need to clear `Strict Controls` to avoid them being displayed and causing duplicate warnings, for example, log output:
	//
	// WARN The global `--terragrunt-no-auto-init` flag has moved to the `run` command and will not be accessible as a global flag in a future version of Terragrunt. Use `run --no-auto-init` instead.
	// WARN The `--terragrunt-no-auto-init` flag is deprecated and will be removed in a future version of Terragrunt. Use `--no-auto-init` instead.
	cmdOpts := opts.Clone()
	cmdOpts.StrictControls = nil

	commands := commands.New(cmdOpts)

	var seen []string

	for _, cmd := range commands {
		for _, flag := range cmd.Flags {
			flagName := util.FirstElement(util.RemoveEmptyElements(flag.Names()))

			if slices.Contains(seen, flagName) {
				continue
			}

			seen = append(seen, flagName)
			globalFlags = append(globalFlags, flags.NewMovedFlag(flag, cmd.Name, flags.StrictControlsByMovedGlobalFlags(opts.StrictControls, cmd.Name)))
		}
	}

	return globalFlags
}

// NewFlags creates and returns global flags common for all commands.
func NewFlags(opts *options.TerragruntOptions, prefix flags.Prefix) cli.Flags {
	tgPrefix := prefix.Prepend(flags.TgPrefix)
	terragruntPrefix := prefix.Prepend(flags.TerragruntPrefix)
	terragruntPrefixControl := flags.StrictControlsByGlobalFlags(opts.StrictControls)
	legacyLogsControl := flags.StrictControlsByGlobalFlags(opts.StrictControls, controls.LegacyLogs)

	flags := cli.Flags{
		NewLogLevelFlag(opts, prefix),

		flags.NewFlag(&cli.GenericFlag[string]{
			Name:        WorkingDirFlagName,
			EnvVars:     tgPrefix.EnvVars(WorkingDirFlagName),
			Destination: &opts.WorkingDir,
			Usage:       "The path to the directory of Terragrunt configurations. Default is current directory.",
		},
			flags.WithDeprecatedNames(terragruntPrefix.FlagNames(DeprecatedWorkingDirFlagName), terragruntPrefixControl)),

		flags.NewFlag(&cli.BoolFlag{
			Name:    LogDisableFlagName,
			EnvVars: tgPrefix.EnvVars(LogDisableFlagName),
			Usage:   "Disable logging.",
			Setter: func(val bool) error {
				opts.Logger.Formatter().SetDisabledOutput(val)
				opts.ForwardTFStdout = true
				return nil
			},
		},
			flags.WithDeprecatedNames(terragruntPrefix.FlagNames(DeprecatedLogDisableFlagName), terragruntPrefixControl)),

		flags.NewFlag(&cli.BoolFlag{
			Name:        ShowLogAbsPathsFlagName,
			EnvVars:     tgPrefix.EnvVars(ShowLogAbsPathsFlagName),
			Destination: &opts.LogShowAbsPaths,
			Usage:       "Show absolute paths in logs.",
		},
			flags.WithDeprecatedNames(terragruntPrefix.FlagNames(DeprecatedShowLogAbsPathsFlagName), terragruntPrefixControl)),

		flags.NewFlag(&cli.BoolFlag{
			Name:    NoColorFlagName,
			EnvVars: tgPrefix.EnvVars(NoColorFlagName),
			Usage:   "Disable color output.",
			Setter: func(val bool) error {
				opts.Logger.Formatter().SetDisabledColors(val)
				return nil
			},
		},
			flags.WithDeprecatedNames(terragruntPrefix.FlagNames(DeprecatedNoColorFlagName), terragruntPrefixControl)),

		flags.NewFlag(&cli.GenericFlag[string]{
			Name:    LogFormatFlagName,
			EnvVars: tgPrefix.EnvVars(LogFormatFlagName),
			Usage:   "Set the log format.",
			Setter:  opts.Logger.Formatter().SetFormat,
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
			flags.WithDeprecatedNames(terragruntPrefix.FlagNames(DeprecatedLogFormatFlagName), terragruntPrefixControl),
			flags.WithDeprecatedFlag(&cli.BoolFlag{
				Name:        terragruntPrefix.FlagName(DeprecatedDisableLogFormattingFlagName),
				EnvVars:     terragruntPrefix.EnvVars(DeprecatedDisableLogFormattingFlagName),
				Destination: &opts.DisableLogFormatting,
				Usage:       "If specified, logs will be displayed in key/value format. By default, logs are formatted in a human readable format.",
			}, flags.NewValue(format.KeyValueFormatName), legacyLogsControl),
			flags.WithDeprecatedFlag(&cli.BoolFlag{
				Name:        terragruntPrefix.FlagName(DeprecatedJSONLogFlagName),
				EnvVars:     terragruntPrefix.EnvVars(DeprecatedJSONLogFlagName),
				Destination: &opts.JSONLogFormat,
				Usage:       "If specified, Terragrunt will output its logs in JSON format.",
			}, flags.NewValue(format.JSONFormatName), legacyLogsControl),
			flags.WithDeprecatedFlag(&cli.BoolFlag{
				Name:    terragruntPrefix.FlagName(DeprecatedTfLogJSONFlagName),
				EnvVars: terragruntPrefix.EnvVars(DeprecatedTfLogJSONFlagName),
				Usage:   "If specified, Terragrunt will wrap Terraform stdout and stderr in JSON.",
			}, flags.NewValue(format.JSONFormatName), legacyLogsControl)),

		flags.NewFlag(&cli.GenericFlag[string]{
			Name:    LogCustomFormatFlagName,
			EnvVars: tgPrefix.EnvVars(LogCustomFormatFlagName),
			Usage:   "Set the custom log formatting.",
			Setter:  opts.Logger.Formatter().SetCustomFormat,
		},
			flags.WithDeprecatedNames(terragruntPrefix.FlagNames(DeprecatedLogCustomFormatFlagName), terragruntPrefixControl)),

		flags.NewFlag(&cli.BoolFlag{
			Name:        NonInteractiveFlagName,
			EnvVars:     tgPrefix.EnvVars(NonInteractiveFlagName),
			Destination: &opts.NonInteractive,
			Usage:       `Assume "yes" for all prompts.`,
		},
			flags.WithDeprecatedNames(terragruntPrefix.FlagNames(DeprecatedNonInteractiveFlagName), terragruntPrefixControl),
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
			flags.WithDeprecatedNames(terragruntPrefix.FlagNames(DeprecatedExperimentModeFlagName), terragruntPrefixControl)),

		flags.NewFlag(&cli.SliceFlag[string]{
			Name:    ExperimentFlagName,
			EnvVars: tgPrefix.EnvVars(ExperimentFlagName),
			Usage:   "Enables specific experiments. For a list of available experiments, see https://terragrunt.gruntwork.io/docs/reference/experiment-mode .",
			Setter:  opts.Experiments.EnableExperiment,
			Action: func(_ *cli.Context, val []string) error {
				opts.Experiments.NotifyCompletedExperiments(opts.Logger)

				return nil
			},
		},
			flags.WithDeprecatedNames(terragruntPrefix.FlagNames(DeprecatedExperimentFlagName), terragruntPrefixControl)),

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
				opts.StrictControls.LogEnabled(opts.Logger)

				return nil
			},
		},
			flags.WithDeprecatedNames(terragruntPrefix.FlagNames(DeprecatedStrictModeFlagName), terragruntPrefixControl)),

		flags.NewFlag(&cli.SliceFlag[string]{
			Name:    StrictControlFlagName,
			EnvVars: tgPrefix.EnvVars(StrictControlFlagName),
			Usage:   "Enables specific strict controls. For a list of available controls, run 'terragrunt info strict'.",
			Setter: func(val string) error {
				return opts.StrictControls.EnableControl(val)
			},
			Action: func(_ *cli.Context, _ []string) error {
				opts.StrictControls.LogEnabled(opts.Logger)

				return nil
			},
		},
			flags.WithDeprecatedNames(terragruntPrefix.FlagNames(DeprecatedStrictControlFlagName), terragruntPrefixControl)),
	}

	flags = flags.Sort()
	flags = flags.Add(NewHelpVersionFlags(opts)...)

	return flags
}

func NewLogLevelFlag(opts *options.TerragruntOptions, prefix flags.Prefix) *flags.Flag {
	tgPrefix := prefix.Prepend(flags.TgPrefix)
	terragruntPrefix := prefix.Prepend(flags.TerragruntPrefix)
	terragruntPrefixControl := flags.StrictControlsByGlobalFlags(opts.StrictControls)

	return flags.NewFlag(&cli.GenericFlag[string]{
		Name:        LogLevelFlagName,
		EnvVars:     tgPrefix.EnvVars(LogLevelFlagName),
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
	}, flags.WithDeprecatedNames(terragruntPrefix.FlagNames(DeprecatedLogLevelFlagName), terragruntPrefixControl))
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
