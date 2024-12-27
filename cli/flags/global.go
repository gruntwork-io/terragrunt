// Package flags provides Terragrunt command flags.
package flags

import (
	"fmt"

	"github.com/gruntwork-io/go-commons/collections"
	"github.com/gruntwork-io/terragrunt/internal/cli"
	"github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/gruntwork-io/terragrunt/internal/experiment"
	"github.com/gruntwork-io/terragrunt/internal/strict"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/pkg/log/format"
	"github.com/gruntwork-io/terragrunt/pkg/log/format/placeholders"
	"github.com/gruntwork-io/terragrunt/shell"
)

const EnvVarPrefix = "TG_"

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

// NewGlobalFlags creates and returns common flags.
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
			Name:        LogDisableFlagName,
			EnvVars:     EnvVars(LogDisableFlagName),
			Usage:       "Disable logging.",
			Destination: &opts.DisableLog,
			Action: func(_ *cli.Context, _ bool) error {
				opts.ForwardTFStdout = true
				opts.LogFormatter.SetFormat(nil)

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
			Name:        NoColorFlagName,
			EnvVars:     EnvVars(NoColorFlagName),
			Destination: &opts.DisableLogColors,
			Usage:       "Disable color output.",
			Action: func(_ *cli.Context, _ bool) error {
				opts.LogFormatter.DisableColors()

				return nil
			},
		}),
		GenericWithDeprecatedFlag(opts, &cli.GenericFlag[string]{
			Name:    LogFormatFlagName,
			EnvVars: EnvVars(LogFormatFlagName),
			Usage:   "Set the log format.",
			Action: func(_ *cli.Context, val string) error {
				phs, err := format.ParseFormat(val)
				if err != nil {
					return cli.NewExitError(errors.Errorf("flag --%s, invalid format %q, %v", LogFormatFlagName, val, err), 1)
				}

				if opts.DisableLog || opts.DisableLogFormatting || opts.JSONLogFormat {
					return nil
				}

				switch val {
				case format.BareFormatName:
					opts.ForwardTFStdout = true
				case format.JSONFormatName:
					opts.JSONLogFormat = true
				}

				opts.LogFormatter.SetFormat(phs)

				return nil
			},
		}),
		GenericWithDeprecatedFlag(opts, &cli.GenericFlag[string]{
			Name:    LogCustomFormatFlagName,
			EnvVars: EnvVars(LogCustomFormatFlagName),
			Usage:   "Set the custom log formatting.",
			Action: func(_ *cli.Context, val string) error {
				phs, err := placeholders.Parse(val)
				if err != nil {
					return cli.NewExitError(errors.Errorf("flag --%s, %w", LogCustomFormatFlagName, err), 1)
				}

				opts.LogFormatter.SetFormat(phs)

				return nil
			},
		}),
		// Experiment Mode flags
		&cli.BoolFlag{
			Name:        ExperimentModeFlagName,
			EnvVars:     EnvVars(ExperimentModeFlagName),
			Destination: &opts.ExperimentMode,
			Usage:       "Enables experiment mode for Terragrunt. For more information, see https://terragrunt.gruntwork.io/docs/reference/experiment-mode .",
		},
		&cli.SliceFlag[string]{
			Name:    ExperimentFlagName,
			EnvVars: EnvVars(ExperimentFlagName),
			Usage:   "Enables specific experiments. For a list of available experiments, see https://terragrunt.gruntwork.io/docs/reference/experiment-mode .",
			Action: func(_ *cli.Context, val []string) error {
				experiments := experiment.NewExperiments()
				warning, err := experiments.ValidateExperimentNames(val)
				if err != nil {
					return cli.NewExitError(err, 1)
				}

				if warning != "" {
					log.Warn(warning)
				}

				if err := experiments.EnableExperiments(val); err != nil {
					return cli.NewExitError(err, 1)
				}

				opts.Experiments = experiments

				return nil
			},
		},
		// Strict Mode flags.
		BoolWithDeprecatedFlag(opts, &cli.BoolFlag{
			Name:        StrictModeFlagName,
			EnvVars:     EnvVars(StrictModeFlagName),
			Destination: &opts.StrictMode,
			Usage:       "Enables strict mode for Terragrunt. For more information, see https://terragrunt.gruntwork.io/docs/reference/strict-mode .",
		}),
		SliceWithDeprecatedFlag(opts, &cli.SliceFlag[string]{
			Name:        StrictControlFlagName,
			EnvVars:     EnvVars(StrictControlFlagName),
			Destination: &opts.StrictControls,
			Usage:       "Enables specific strict controls. For a list of available controls, see https://terragrunt.gruntwork.io/docs/reference/strict-mode .",
			Action: func(_ *cli.Context, val []string) error {
				warning, err := strict.StrictControls.ValidateControlNames(val)
				if err != nil {
					return cli.NewExitError(err, 1)
				}

				if warning != "" {
					log.Warn(warning)
				}

				return nil
			},
		}),
		BoolWithDeprecatedFlag(opts, &cli.BoolFlag{
			Name:        NonInteractiveFlagName,
			EnvVars:     EnvVars(NonInteractiveFlagName),
			Destination: &opts.NonInteractive,
			Usage:       `Assume "yes" for all prompts.`,
		}),

		// Deprecated flags.
		&cli.BoolFlag{
			Name:        TerragruntDisableLogFormattingFlagName,
			EnvVars:     EnvVars(TerragruntDisableLogFormattingFlagName),
			Destination: &opts.DisableLogFormatting,
			Usage:       "If specified, logs will be displayed in key/value format. By default, logs are formatted in a human readable format.",
			Hidden:      true,
			Action: func(_ *cli.Context, _ bool) error {
				opts.LogFormatter.SetFormat(format.NewKeyValueFormat())

				if control, ok := strict.GetStrictControl(strict.DisableLogFormatting); ok {
					warn, triggered, err := control.Evaluate(opts)
					if err != nil {
						return err
					}

					if !triggered {
						opts.Logger.Warnf(warn)
					}
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
				opts.LogFormatter.SetFormat(format.NewJSONFormat())

				if control, ok := strict.GetStrictControl(strict.JSONLog); ok {
					warn, triggered, err := control.Evaluate(opts)
					if err != nil {
						return err
					}

					if !triggered {
						opts.Logger.Warnf(warn)
					}
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
		DefaultText: opts.LogLevel.String(),
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
				opts.LogFormatter.SetFormat(nil)

				return nil
			}

			level, err := log.ParseLevel(val)
			if err != nil {
				return cli.NewExitError(errors.Errorf("flag --%s, %w", LogLevelFlagName, err), 1)
			}

			opts.Logger.SetOptions(log.WithLevel(level))
			opts.LogLevel = level

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
			Action: func(ctx *cli.Context, _ bool) (err error) {
				defer func() {
					// exit the app
					err = cli.NewExitError(err, 0)
				}()

				// If the app command is specified, show help for the command
				if cmdName := ctx.Args().CommandName(); cmdName != "" {
					err := cli.ShowCommandHelp(ctx, cmdName)

					// If the command name is not found, it is most likely a terraform command, show Terraform help.
					var invalidCommandNameError cli.InvalidCommandNameError
					if ok := errors.As(err, &invalidCommandNameError); ok {
						terraformHelpCmd := append([]string{cmdName, "-help"}, ctx.Args().Tail()...)

						return shell.RunTerraformCommand(ctx, opts, terraformHelpCmd...)
					}

					return err
				}

				// In other cases, show the App help.
				return cli.ShowAppHelp(ctx)
			},
		},
		&cli.BoolFlag{
			Name:    VersionFlagName, // --version, -version
			Aliases: []string{"v"},   //  -v
			Usage:   "Show terragrunt version.",
			Action: func(ctx *cli.Context, _ bool) (err error) {
				defer func() {
					// exit the app
					err = cli.NewExitError(err, 0)
				}()

				return cli.ShowVersion(ctx)
			},
		},
	}
}
