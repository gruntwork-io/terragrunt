// Package global provides CLI global flags.
package global

import (
	"fmt"
	"os"

	"github.com/gruntwork-io/go-commons/collections"
	"github.com/gruntwork-io/terragrunt/cli/flags"
	"github.com/gruntwork-io/terragrunt/internal/cli"
	"github.com/gruntwork-io/terragrunt/internal/errors"
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

	// Renamed flags.

	DeprecatedLogLevelFlagName        = "log-level"
	DeprecatedLogDisableFlagName      = "log-disable"
	DeprecatedShowLogAbsPathsFlagName = "log-show-abs-paths"
	DeprecatedLogFormatFlagName       = "log-format"
	DeprecatedLogCustomFormatFlagName = "log-custom-format"
	DeprecatedNoColorFlagName         = "no-color"
	DeprecatedNonInteractiveFlagName  = "non-interactive"
	DeprecatedWorkingDirFlagName      = "working-dir"

	// Deprecated flags.

	DeprecatedDisableLogFormattingFlagName = "disable-log-formatting"
	DeprecatedJSONLogFlagName              = "json-log"
	DeprecatedTfLogJSONFlagName            = "tf-logs-to-json"
)

// NewFlags creates and returns flags common to all commands.
func NewFlags(opts *options.TerragruntOptions, prefix flags.Prefix) cli.Flags {
	tgPrefix := prefix.Prepend(flags.TgPrefix)
	terragruntPrefix := prefix.Prepend(flags.TerragruntPrefix)
	cliRedesignControl := flags.StrictControls(opts.StrictControls, controls.CLIRedesign)
	legacyLogsControl := flags.StrictControls(opts.StrictControls, controls.LegacyLogs)

	flags := cli.Flags{
		NewLogLevelFlag(opts, prefix),

		flags.NewFlag(&cli.GenericFlag[string]{
			Name:        WorkingDirFlagName,
			EnvVars:     tgPrefix.EnvVars(WorkingDirFlagName),
			Destination: &opts.WorkingDir,
			Usage:       "The path to the directory of Terragrunt configurations. Default is current directory.",
		},
			flags.WithDeprecatedNames(terragruntPrefix.FlagNames(DeprecatedWorkingDirFlagName), cliRedesignControl)),

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
			flags.WithDeprecatedNames(terragruntPrefix.FlagNames(DeprecatedLogDisableFlagName), cliRedesignControl)),

		flags.NewFlag(&cli.BoolFlag{
			Name:        ShowLogAbsPathsFlagName,
			EnvVars:     tgPrefix.EnvVars(ShowLogAbsPathsFlagName),
			Destination: &opts.LogShowAbsPaths,
			Usage:       "Show absolute paths in logs.",
		},
			flags.WithDeprecatedNames(terragruntPrefix.FlagNames(DeprecatedShowLogAbsPathsFlagName), cliRedesignControl)),

		flags.NewFlag(&cli.BoolFlag{
			Name:    NoColorFlagName,
			EnvVars: tgPrefix.EnvVars(NoColorFlagName),
			Usage:   "Disable color output.",
			Setter: func(val bool) error {
				opts.Logger.Formatter().SetDisabledColors(val)
				return nil
			},
		},
			flags.WithDeprecatedNames(terragruntPrefix.FlagNames(DeprecatedNoColorFlagName), cliRedesignControl)),

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
			flags.WithDeprecatedPrefix(terragruntPrefix, cliRedesignControl),
			flags.WithDeprecatedFlag(&cli.BoolFlag{
				Name:        terragruntPrefix.FlagName(DeprecatedDisableLogFormattingFlagName),
				EnvVars:     terragruntPrefix.EnvVars(DeprecatedDisableLogFormattingFlagName),
				Destination: &opts.DisableLogFormatting,
				Usage:       "If specified, logs will be displayed in key/value format. By default, logs are formatted in a human readable format.",
				Hidden:      true,
			}, flags.NewValue(format.KeyValueFormatName), legacyLogsControl),
			flags.WithDeprecatedFlag(&cli.BoolFlag{
				Name:        terragruntPrefix.FlagName(DeprecatedJSONLogFlagName),
				EnvVars:     terragruntPrefix.EnvVars(DeprecatedJSONLogFlagName),
				Destination: &opts.JSONLogFormat,
				Usage:       "If specified, Terragrunt will output its logs in JSON format.",
				Hidden:      true,
			}, flags.NewValue(format.JSONFormatName), legacyLogsControl),
			flags.WithDeprecatedFlag(&cli.BoolFlag{
				Name:    terragruntPrefix.FlagName(DeprecatedTfLogJSONFlagName),
				EnvVars: terragruntPrefix.EnvVars(DeprecatedTfLogJSONFlagName),
				Usage:   "If specified, Terragrunt will wrap Terraform stdout and stderr in JSON.",
				Hidden:  true,
			}, flags.NewValue(format.JSONFormatName), legacyLogsControl)),

		flags.NewFlag(&cli.GenericFlag[string]{
			Name:    LogCustomFormatFlagName,
			EnvVars: tgPrefix.EnvVars(LogCustomFormatFlagName),
			Usage:   "Set the custom log formatting.",
			Setter:  opts.Logger.Formatter().SetCustomFormat,
		},
			flags.WithDeprecatedNames(terragruntPrefix.FlagNames(DeprecatedLogCustomFormatFlagName), cliRedesignControl)),

		flags.NewFlag(&cli.BoolFlag{
			Name:        NonInteractiveFlagName,
			EnvVars:     tgPrefix.EnvVars(NonInteractiveFlagName),
			Destination: &opts.NonInteractive,
			Usage:       `Assume "yes" for all prompts.`,
		},
			flags.WithDeprecatedNames(terragruntPrefix.FlagNames(DeprecatedNonInteractiveFlagName), cliRedesignControl)),

		// Experiment Mode flags.

		flags.NewFlag(&cli.BoolFlag{
			Name:    ExperimentModeFlagName,
			EnvVars: tgPrefix.EnvVars(ExperimentModeFlagName),
			Usage:   "Enables experiment mode for Terragrunt. For more information, see https://terragrunt.gruntwork.io/docs/reference/experiment-mode .",
			Setter: func(_ bool) error {
				opts.Experiments.ExperimentMode()

				return nil
			},
		}),

		flags.NewFlag(&cli.SliceFlag[string]{
			Name:    ExperimentFlagName,
			EnvVars: tgPrefix.EnvVars(ExperimentFlagName),
			Usage:   "Enables specific experiments. For a list of available experiments, see https://terragrunt.gruntwork.io/docs/reference/experiment-mode .",
			Setter:  opts.Experiments.EnableExperiment,
			Action: func(_ *cli.Context, val []string) error {
				opts.Experiments.NotifyCompletedExperiments(opts.Logger)

				return nil
			},
		}),

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
		}),

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
		}),
	}

	flags = flags.Sort()
	flags = flags.Add(NewHelpVersionFlags(opts)...)

	return flags
}

func NewLogLevelFlag(opts *options.TerragruntOptions, prefix flags.Prefix) cli.Flag {
	tgPrefix := prefix.Prepend(flags.TgPrefix)
	terragruntPrefix := prefix.Prepend(flags.TerragruntPrefix)
	cliRedesignControl := flags.StrictControls(opts.StrictControls, controls.CLIRedesign)

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
	}, flags.WithDeprecatedNames(terragruntPrefix.FlagNames(DeprecatedLogLevelFlagName), cliRedesignControl))
}

func NewHelpVersionFlags(opts *options.TerragruntOptions) cli.Flags {
	return cli.Flags{
		&cli.BoolFlag{
			Name:    HelpFlagName,  // --help, -help
			Aliases: []string{"h"}, //  -h
			Usage:   "Show help.",
			Action: func(ctx *cli.Context, _ bool) error {
				return HelpAction(ctx, opts)
			},
		},
		&cli.BoolFlag{
			Name:    VersionFlagName, // --version, -version
			Aliases: []string{"v"},   //  -v
			Usage:   "Show terragrunt version.",
			Action: func(ctx *cli.Context, _ bool) (err error) {
				return VersionAction(ctx, opts)
			},
		},
	}
}

func HelpAction(ctx *cli.Context, opts *options.TerragruntOptions) error {
	var (
		args = ctx.Args()
		cmds = ctx.App.Commands
	)

	if ctx.App.DefaultCommand != nil {
		cmds = append(cmds, ctx.App.DefaultCommand.Subcommands...)
	}

	if opts.Logger.Level() >= log.DebugLevel {
		// https: //github.com/urfave/cli/blob/f035ffaa3749afda2cd26fb824aa940747297ef1/help.go#L401
		if err := os.Setenv("CLI_TEMPLATE_ERROR_DEBUG", "1"); err != nil {
			return errors.New(err)
		}
	}

	if args.CommandName() == "" {
		return cli.ShowAppHelp(ctx)
	}

	const maxIterations = 1000

	for range maxIterations {
		cmdName := args.CommandName()

		cmd := cmds.Get(cmdName)
		if cmd == nil {
			break
		}

		args = args.Remove(cmdName)
		cmds = cmd.Subcommands
		ctx = ctx.NewCommandContext(cmd, args)
	}

	if ctx.Command != nil {
		return cli.NewExitError(cli.ShowCommandHelp(ctx), cli.ExitCodeGeneralError)
	}

	return cli.NewExitError(errors.New(cli.InvalidCommandNameError(args.First())), cli.ExitCodeGeneralError)
}

func VersionAction(ctx *cli.Context, _ *options.TerragruntOptions) error {
	return cli.ShowVersion(ctx)
}
