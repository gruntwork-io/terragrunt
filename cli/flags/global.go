// Package flags provides Terragrunt command flags.
package flags

import (
	"fmt"

	"github.com/gruntwork-io/go-commons/collections"
	"github.com/gruntwork-io/terragrunt/internal/cli"
	"github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/gruntwork-io/terragrunt/internal/strict"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/pkg/log/format"
	"github.com/gruntwork-io/terragrunt/tf"
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

	// Deprecated flags.

	TerragruntDisableLogFormattingFlagName = DeprecatedFlagNamePrefix + "disable-log-formatting"
	TerragruntJSONLogFlagName              = DeprecatedFlagNamePrefix + "json-log"
)

// NewGlobalFlags creates and returns flags common to all commands.
func NewGlobalFlags(opts *options.TerragruntOptions) cli.Flags {
	flags := cli.Flags{
		NewLogLevelFlag(opts),
		GenericWithDeprecatedFlag(opts, &cli.GenericFlag[string]{
			Name:        WorkingDirFlagName,
			EnvVars:     EnvVars(WorkingDirFlagName),
			Destination: &opts.WorkingDir,
			Usage:       "The path to the directory of Terragrunt configurations. Default is current directory.",
		}),
		BoolWithDeprecatedFlag(opts, &cli.BoolFlag{
			Name:    LogDisableFlagName,
			EnvVars: EnvVars(LogDisableFlagName),
			Usage:   "Disable logging.",
			Setter: func(val bool) error {
				opts.Logger.Formatter().SetDisabledOutput(val)
				opts.ForwardTFStdout = true
				return nil
			},
		}),
		BoolWithDeprecatedFlag(opts, &cli.BoolFlag{
			Name:        ShowLogAbsPathsFlagName,
			EnvVars:     EnvVars(ShowLogAbsPathsFlagName),
			Destination: &opts.LogShowAbsPaths,
			Usage:       "Show absolute paths in logs.",
		}),
		BoolWithDeprecatedFlag(opts, &cli.BoolFlag{
			Name:    NoColorFlagName,
			EnvVars: EnvVars(NoColorFlagName),
			Usage:   "Disable color output.",
			Setter: func(val bool) error {
				opts.Logger.Formatter().SetDisabledColors(val)
				return nil
			},
		}),
		GenericWithDeprecatedFlag(opts, &cli.GenericFlag[string]{
			Name:    LogFormatFlagName,
			EnvVars: EnvVars(LogFormatFlagName),
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
		}),
		GenericWithDeprecatedFlag(opts, &cli.GenericFlag[string]{
			Name:    LogCustomFormatFlagName,
			EnvVars: EnvVars(LogCustomFormatFlagName),
			Usage:   "Set the custom log formatting.",
			Setter:  opts.Logger.Formatter().SetCustomFormat,
		}),
		BoolWithDeprecatedFlag(opts, &cli.BoolFlag{
			Name:        NonInteractiveFlagName,
			EnvVars:     EnvVars(NonInteractiveFlagName),
			Destination: &opts.NonInteractive,
			Usage:       `Assume "yes" for all prompts.`,
		}),

		// Experiment Mode flags

		BoolWithDeprecatedFlag(opts, &cli.BoolFlag{
			Name:    ExperimentModeFlagName,
			EnvVars: EnvVars(ExperimentModeFlagName),
			Usage:   "Enables experiment mode for Terragrunt. For more information, see https://terragrunt.gruntwork.io/docs/reference/experiment-mode .",
			Setter: func(_ bool) error {
				opts.Experiments.ExperimentMode()

				return nil
			},
		}),
		SliceWithDeprecatedFlag(opts, &cli.SliceFlag[string]{
			Name:    ExperimentFlagName,
			EnvVars: EnvVars(ExperimentFlagName),
			Usage:   "Enables specific experiments. For a list of available experiments, see https://terragrunt.gruntwork.io/docs/reference/experiment-mode .",
			Setter:  opts.Experiments.EnableExperiment,
			Action: func(_ *cli.Context, val []string) error {
				opts.Experiments.NotifyCompletedExperiments(opts.Logger)

				return nil
			},
		}),

		// Strict Mode flags.

		BoolWithDeprecatedFlag(opts, &cli.BoolFlag{
			Name:    StrictModeFlagName,
			EnvVars: EnvVars(StrictModeFlagName),
			Usage:   "Enables strict mode for Terragrunt. For more information, see https://terragrunt.gruntwork.io/docs/reference/strict-mode .",
			Setter: func(_ bool) error {
				opts.StrictControls.EnableStrictMode()

				return nil
			},
		}),
		SliceWithDeprecatedFlag(opts, &cli.SliceFlag[string]{
			Name:    StrictControlFlagName,
			EnvVars: EnvVars(StrictControlFlagName),
			Usage:   "Enables specific strict controls. For a list of available controls, see https://terragrunt.gruntwork.io/docs/reference/strict-mode .",
			Setter:  opts.StrictControls.EnableControl,
			Action: func(_ *cli.Context, _ []string) error {
				opts.StrictControls.NotifyCompletedControls(opts.Logger)

				return nil
			},
		}),

		// Deprecated flags.

		&cli.BoolFlag{
			Name:        TerragruntDisableLogFormattingFlagName,
			EnvVars:     EnvVars(TerragruntDisableLogFormattingFlagName),
			Destination: &opts.DisableLogFormatting,
			Usage:       "If specified, logs will be displayed in key/value format. By default, logs are formatted in a human readable format.",
			Hidden:      true,
			Action: func(_ *cli.Context, _ bool) error {
				if err := opts.Logger.Formatter().SetFormat(format.KeyValueFormatName); err != nil {
					return err
				}

				newFlagName := LogCustomFormatFlagName + "=" + format.KeyValueFormatName

				if err := opts.StrictControls.Evaluate(opts.Logger, strict.DeprecatedFlags, TerragruntDisableLogFormattingFlagName, newFlagName); err != nil {
					return cli.NewExitError(err, cli.ExitCodeGeneralError)
				}

				return nil
			},
		},
		&cli.BoolFlag{
			Name:        TerragruntJSONLogFlagName,
			EnvVars:     EnvVars(TerragruntJSONLogFlagName),
			Destination: &opts.JSONLogFormat,
			Usage:       "If specified, Terragrunt will output its logs in JSON format.",
			Hidden:      true,
			Action: func(_ *cli.Context, _ bool) error {
				if err := opts.Logger.Formatter().SetFormat(format.JSONFormatName); err != nil {
					return err
				}

				newFlagName := LogCustomFormatFlagName + "=" + format.JSONFormatName

				if err := opts.StrictControls.Evaluate(opts.Logger, strict.DeprecatedFlags, TerragruntJSONLogFlagName, newFlagName); err != nil {
					return cli.NewExitError(err, cli.ExitCodeGeneralError)
				}

				return nil
			},
		},
	}

	flags = flags.Sort()
	flags = flags.Add(NewHelpVersionFlags(opts)...)

	return flags
}

func NewLogLevelFlag(opts *options.TerragruntOptions) cli.Flag {
	return GenericWithDeprecatedFlag(opts, &cli.GenericFlag[string]{
		Name:        LogLevelFlagName,
		EnvVars:     EnvVars(LogLevelFlagName),
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
	})
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
	// If the app command is specified, show help for the command
	if cmdName := ctx.Args().CommandName(); cmdName != "" {
		err := cli.ShowSubcommandHelp(ctx, cmdName)

		// If the command name is not found, it is most likely a terraform command, show Terraform help.
		var invalidCommandNameError cli.InvalidCommandNameError
		if ok := errors.As(err, &invalidCommandNameError); ok {
			terraformHelpCmd := append([]string{cmdName, "-help"}, ctx.Args().Tail()...)

			return cli.NewExitError(tf.RunCommand(ctx, opts, terraformHelpCmd...), cli.ExitCodeSuccess)
		}

		return err
	}

	// In other cases, show the App help.
	return cli.ShowAppHelp(ctx)
}

func VersionAction(ctx *cli.Context, _ *options.TerragruntOptions) error {
	return cli.ShowVersion(ctx)
}
