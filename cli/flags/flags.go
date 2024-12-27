// Package flags provides Terragrunt command flags.
package flags

import (
	"fmt"

	"github.com/gruntwork-io/go-commons/collections"
	"github.com/gruntwork-io/terragrunt/config"
	"github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/gruntwork-io/terragrunt/internal/experiment"
	"github.com/gruntwork-io/terragrunt/internal/strict"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/pkg/cli"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/pkg/log/format"
	"github.com/gruntwork-io/terragrunt/pkg/log/format/placeholders"
	"github.com/gruntwork-io/terragrunt/shell"
	"github.com/gruntwork-io/terragrunt/util"
)

const EnvVarPrefix = "TG_"

const (
	ConfigFlagName                         = "config"
	NoAutoInitFlagName                     = "no-auto-init"
	NoAutoRetryFlagName                    = "no-auto-retry"
	NoAutoApproveFlagName                  = "no-auto-approve"
	NonInteractiveFlagName                 = "non-interactive"
	WorkingDirFlagName                     = "working-dir"
	DownloadDirFlagName                    = "download-dir"
	TFForwardStdoutFlagName                = "tf-forward-stdout"
	TFPathFlagName                         = "tf-path"
	FeatureMapFlagName                     = "feature"
	ParallelismFlagName                    = "parallelism"
	DebugInputsFlagName                    = "debug-inputs"
	UnitsThatIncludeFlagName               = "units-that-include"
	DependencyFetchOutputFromStateFlagName = "dependency-fetch-output-from-state"
	UsePartialParseConfigCacheFlagName     = "use-partial-parse-config-cache"

	BackendRequireBootstrapFlagName = "backend-require-bootstrap"
	DisableBucketUpdateFlagName     = "disable-bucket-update"

	DisableCommandValidationFlagName   = "disable-command-validation"
	AuthProviderCmdFlagName            = "auth-provider-cmd"
	OutDirFlagName                     = "out-dir"
	JSONOutDirFlagName                 = "json-out-dir"
	NoDestroyDependenciesCheckFlagName = "no-destroy-dependencies-check"

	SourceFlagName       = "source"
	SourceMapFlagName    = "source-map"
	SourceUpdateFlagName = "source-update"

	// Assume IAM Role flags.

	IAMAssumeRoleFlagName                 = "iam-assume-role"
	IAMAssumeRoleDurationFlagName         = "iam-assume-role-duration"
	IAMAssumeRoleSessionNameFlagName      = "iam-assume-role-session-name"
	IAMAssumeRoleWebIdentityTokenFlagName = "iam-assume-role-web-identity-token"

	// Queue related flags.

	QueueIgnoreErrorsFlagName        = "queue-ignore-errors"
	QueueIgnoreDAGOrderFlagName      = "queue-ignore-dag-order"
	QueueExcludeExternalFlagName     = "queue-exclude-external"
	QueueExcludeDirFlagName          = "queue-exclude-dir"
	QueueExcludeFileFlagName         = "queue-exclude-file"
	QueueIncludeDirFlagName          = "queue-include-dir"
	QueueIncludeExternalFlagName     = "queue-include-external"
	QueueStrictIncludeFlagName       = "queue-strict-include"
	QueueIncludeUnitsReadingFlagName = "queue-include-units-reading"

	// Logs related flags.

	LogLevelFlagName        = "log-level"
	LogDisableFlagName      = "log-disable"
	ShowLogAbsPathsFlagName = "log-show-abs-paths"
	LogFormatFlagName       = "log-format"
	LogCustomFormatFlagName = "log-custom-format"
	NoColorFlagName         = "no-color"

	// Strict Mode related flags.

	StrictModeFlagName    = "strict-mode"
	StrictControlFlagName = "strict-control"

	// Experiment Mode related flags/envs.

	ExperimentModeFlagName = "experiment-mode"
	ExperimentFlagName     = "experiment"

	// Terragrunt Provider Cache related flags.

	ProviderCacheFlagName              = "provider-cache"
	ProviderCacheDirFlagName           = "provider-cache-dir"
	ProviderCacheHostnameFlagName      = "provider-cache-hostname"
	ProviderCachePortFlagName          = "provider-cache-port"
	ProviderCacheTokenFlagName         = "provider-cache-token"
	ProviderCacheRegistryNamesFlagName = "provider-cache-registry-names"

	// Engine related environment variables.

	EngineEnableFlagName    = "experimental-engine"
	EngineCachePathFlagName = "engine-cache-path"
	EngineSkipCheckFlagName = "engine-skip-check"
	EngineLogLevelFlagName  = "engine-log-level"

	// Scaffold/Catalog shared flags.

	RootFileNameFlagName  = "root-file-name"
	NoIncludeRootFlagName = "no-include-root"

	// App flags.

	HelpFlagName    = "help"
	VersionFlagName = "version"

	// Renamed flags.

	TerragruntFailOnStateBucketCreationFlagName      = DeprecatedFlagNamePrefix + "fail-on-state-bucket-creation"      // `backend-require-bootstrap`
	TerragruntModulesThatIncludeFlagName             = DeprecatedFlagNamePrefix + "modules-that-include"               // `units-that-include`
	TerragruntForwardTFStdoutFlagName                = DeprecatedFlagNamePrefix + "forward-tf-stdout"                  // `tf-forward-stdout`.
	TerragruntTFPathFlagName                         = DeprecatedFlagNamePrefix + "tfpath"                             // `tf-path`.
	TerragruntDownloadFlagName                       = DeprecatedFlagNamePrefix + "download"                           // `download-dir` for old `TERRAGRUNT_DOWNLOAD` env var.
	TerragruntIAMRoleFlagName                        = DeprecatedFlagNamePrefix + "iam-role"                           // `iam-assume-role`.
	TerragruntIAMWebIdentityTokenFlagName            = DeprecatedFlagNamePrefix + "iam-web-identity-token"             // `iam-assume-role-web-identity-token`.
	TerragruntDebugFlagName                          = DeprecatedFlagNamePrefix + "debug"                              // `debug-inputs`.
	TerragruntFetchDependencyOutputFromStateFlagName = DeprecatedFlagNamePrefix + "fetch-dependency-output-from-state" // `dependency-fetch-output-from-state`.
	TerragruntIgnoreDependencyOrderFlagName          = DeprecatedFlagNamePrefix + "ignore-dependency-order"            // `queue-ignore-dag-order`.
	TerragruntIgnoreExternalDependenciesFlagName     = DeprecatedFlagNamePrefix + "ignore-external-dependencies"       // `queue-exclude-external`.
	TerragruntExcludeDirFlagName                     = DeprecatedFlagNamePrefix + "exclude-dir"                        // `queue-exclude-dir`.
	TerragruntExcludesFileFlagName                   = DeprecatedFlagNamePrefix + "excludes-file"                      // `queue-exclude-file`.
	TerragruntIncludeDirFlagName                     = DeprecatedFlagNamePrefix + "include-dir"                        // `queue-include-dir`.
	TerragruntIncludeExternalDependenciesFlagName    = DeprecatedFlagNamePrefix + "include-external-dependencies"      // `queue-include-external`.
	TerragruntStrictIncludeFlagName                  = DeprecatedFlagNamePrefix + "strict-include"                     // `queue-strict-include`.
	TerragruntUnitsReadingFlagName                   = DeprecatedFlagNamePrefix + "queue-include-units-reading"        // `queue-include-units-reading`.
	TerragruntIgnoreDependencyErrorsFlagName         = DeprecatedFlagNamePrefix + "ignore-dependency-errors"           // `queue-ignore-errors`.

	// Deprecated flags.

	TerragruntIncludeModulePrefixFlagName  = DeprecatedFlagNamePrefix + "include-module-prefix"
	TerragruntDisableLogFormattingFlagName = DeprecatedFlagNamePrefix + "disable-log-formatting"
	TerragruntJSONLogFlagName              = DeprecatedFlagNamePrefix + "json-log"
	TerragruntTfLogJSONFlagName            = DeprecatedFlagNamePrefix + "tf-logs-to-json"
)

func NewHelpFlag(opts *options.TerragruntOptions) cli.Flag {
	return &cli.BoolFlag{
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
	}
}

func NewVersionFlag(opts *options.TerragruntOptions) cli.Flag {
	return &cli.BoolFlag{
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
	}
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
		NewHelpFlag(opts),
		NewVersionFlag(opts),
	}
}

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
			Name:    TerragruntIncludeModulePrefixFlagName,
			EnvVars: EnvVars(TerragruntIncludeModulePrefixFlagName),
			Usage:   "When this flag is set output from Terraform sub-commands is prefixed with module path.",
			Hidden:  true,
			Action: func(_ *cli.Context, _ bool) error {
				opts.Logger.Warnf("The %q flag is deprecated. Use the functionality-inverted %q flag instead. By default, Terraform/OpenTofu output is integrated into the Terragrunt log, which prepends additional data, such as timestamps and prefixes, to log entries.", TerragruntIncludeModulePrefixFlagName, TFForwardStdoutFlagName)

				return nil
			},
		},
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
		&cli.BoolFlag{
			Name:    TerragruntTfLogJSONFlagName,
			EnvVars: EnvVars(TerragruntTfLogJSONFlagName),
			Usage:   "If specified, Terragrunt will wrap Terraform stdout and stderr in JSON.",
			Hidden:  true,
			Action: func(_ *cli.Context, _ bool) error {
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

// NewCommonFlags creates and returns global flags.
func NewCommonFlags(opts *options.TerragruntOptions) cli.Flags {
	flags := cli.Flags{
		GenericWithDeprecatedFlag(opts, &cli.GenericFlag[string]{
			Name:        ConfigFlagName,
			EnvVars:     EnvVars(ConfigFlagName),
			Destination: &opts.TerragruntConfigPath,
			Usage:       "The path to the Terragrunt config file. Default is terragrunt.hcl.",
		}),
		GenericWithDeprecatedFlag(opts, &cli.GenericFlag[string]{
			Name:        TFPathFlagName,
			EnvVars:     EnvVars(TFPathFlagName),
			Destination: &opts.TerraformPath,
			Usage:       "Path to the OpenTofu/Terraform binary. Default is tofu (on PATH).",
		}, TerragruntTFPathFlagName),
		BoolWithDeprecatedFlag(opts, &cli.BoolFlag{
			Name:        NoAutoInitFlagName,
			EnvVars:     EnvVars(NoAutoInitFlagName),
			Usage:       "Don't automatically run 'terraform/tofu init' during other terragrunt commands. You must run 'terragrunt init' manually.",
			Negative:    true,
			Destination: &opts.AutoInit,
		}),
		BoolWithDeprecatedFlag(opts, &cli.BoolFlag{
			Name:        NoAutoRetryFlagName,
			EnvVars:     EnvVars(NoAutoRetryFlagName),
			Destination: &opts.AutoRetry,
			Usage:       "Don't automatically re-run command in case of transient errors.",
			Negative:    true,
		}),
		BoolWithDeprecatedFlag(opts, &cli.BoolFlag{
			Name:        NoAutoApproveFlagName,
			EnvVars:     EnvVars(NoAutoApproveFlagName),
			Destination: &opts.RunAllAutoApprove,
			Usage:       "Don't automatically append `-auto-approve` to the underlying OpenTofu/Terraform commands run with 'run-all'.",
			Negative:    true,
		}),
		GenericWithDeprecatedFlag(opts, &cli.GenericFlag[string]{
			Name:        DownloadDirFlagName,
			EnvVars:     EnvVars(DownloadDirFlagName),
			Destination: &opts.DownloadDir,
			Usage:       "The path to download OpenTofu/Terraform modules into. Default is .cache in the working directory.",
		}, DeprecatedFlagNamePrefix+DownloadDirFlagName, TerragruntDownloadFlagName), // the old flag had `terragrunt-download-dir` name and `TERRAGRUNT_DOWNLOAD` env.
		GenericWithDeprecatedFlag(opts, &cli.GenericFlag[string]{
			Name:        SourceFlagName,
			EnvVars:     EnvVars(SourceFlagName),
			Destination: &opts.Source,
			Usage:       "Download OpenTofu/Terraform configurations from the specified source into a temporary folder, and run Terraform in that temporary folder.",
		}),
		BoolWithDeprecatedFlag(opts, &cli.BoolFlag{
			Name:        SourceUpdateFlagName,
			EnvVars:     EnvVars(SourceUpdateFlagName),
			Destination: &opts.SourceUpdate,
			Usage:       "Delete the contents of the temporary folder to clear out any old, cached source code before downloading new source code into it.",
		}),
		MapWithDeprecatedFlag(opts, &cli.MapFlag[string, string]{
			Name:        SourceMapFlagName,
			EnvVars:     EnvVars(SourceMapFlagName),
			Destination: &opts.SourceMap,
			Usage:       "Replace any source URL (including the source URL of a config pulled in with dependency blocks) that has root source with dest.",
			Splitter:    util.SplitUrls,
		}),

		// Assume IAM Role flags.
		GenericWithDeprecatedFlag(opts, &cli.GenericFlag[string]{
			Name:        IAMAssumeRoleFlagName,
			EnvVars:     EnvVars(IAMAssumeRoleFlagName),
			Destination: &opts.IAMRoleOptions.RoleARN,
			Usage:       "Assume the specified IAM role before executing OpenTofu/Terraform.",
		}, TerragruntIAMRoleFlagName),
		GenericWithDeprecatedFlag(opts, &cli.GenericFlag[int64]{
			Name:        IAMAssumeRoleDurationFlagName,
			EnvVars:     EnvVars(IAMAssumeRoleDurationFlagName),
			Destination: &opts.IAMRoleOptions.AssumeRoleDuration,
			Usage:       "Session duration for IAM Assume Role session.",
		}),
		GenericWithDeprecatedFlag(opts, &cli.GenericFlag[string]{
			Name:        IAMAssumeRoleSessionNameFlagName,
			EnvVars:     EnvVars(IAMAssumeRoleSessionNameFlagName),
			Destination: &opts.IAMRoleOptions.AssumeRoleSessionName,
			Usage:       "Name for the IAM Assumed Role session.",
		}),
		GenericWithDeprecatedFlag(opts, &cli.GenericFlag[string]{
			Name:        IAMAssumeRoleWebIdentityTokenFlagName,
			EnvVars:     EnvVars(IAMAssumeRoleWebIdentityTokenFlagName),
			Destination: &opts.IAMRoleOptions.WebIdentityToken,
			Usage:       "For AssumeRoleWithWebIdentity, the WebIdentity token.",
		}, TerragruntIAMWebIdentityTokenFlagName),

		BoolWithDeprecatedFlag(opts, &cli.BoolFlag{
			Name:        QueueIgnoreErrorsFlagName,
			EnvVars:     EnvVars(QueueIgnoreErrorsFlagName),
			Destination: &opts.IgnoreDependencyErrors,
			Usage:       "Continue processing Units even if a dependency fails.",
		}, TerragruntIgnoreDependencyErrorsFlagName),
		BoolWithDeprecatedFlag(opts, &cli.BoolFlag{
			Name:        QueueIgnoreDAGOrderFlagName,
			EnvVars:     EnvVars(QueueIgnoreDAGOrderFlagName),
			Destination: &opts.IgnoreDependencyOrder,
			Usage:       "Ignore DAG order for --all commands.",
		}, TerragruntIgnoreDependencyOrderFlagName),
		BoolWithDeprecatedFlag(opts, &cli.BoolFlag{
			Name:        QueueExcludeExternalFlagName,
			EnvVars:     EnvVars(QueueExcludeExternalFlagName),
			Destination: &opts.IgnoreExternalDependencies,
			Usage:       "Ignore external dependencies for --all commands.",
		}, TerragruntIgnoreExternalDependenciesFlagName),
		BoolWithDeprecatedFlag(opts, &cli.BoolFlag{
			Name:        QueueIncludeExternalFlagName,
			EnvVars:     EnvVars(QueueIncludeExternalFlagName),
			Destination: &opts.IncludeExternalDependencies,
			Usage:       "Include external dependencies for --all commands without asking.",
		}, TerragruntIncludeExternalDependenciesFlagName),
		GenericWithDeprecatedFlag(opts, &cli.GenericFlag[int]{
			Name:        ParallelismFlagName,
			EnvVars:     EnvVars(ParallelismFlagName),
			Destination: &opts.Parallelism,
			Usage:       "Parallelism for --all commands.",
		}),
		GenericWithDeprecatedFlag(opts, &cli.GenericFlag[string]{
			Name:        QueueExcludeFileFlagName,
			EnvVars:     EnvVars(QueueExcludeFileFlagName),
			Destination: &opts.ExcludesFile,
			Usage:       "Path to a file with a list of directories that need to be excluded when running *-all commands.",
		}, TerragruntExcludesFileFlagName),
		SliceWithDeprecatedFlag(opts, &cli.SliceFlag[string]{
			Name:        QueueExcludeDirFlagName,
			EnvVars:     EnvVars(QueueExcludeDirFlagName),
			Destination: &opts.ExcludeDirs,
			Usage:       "Unix-style glob of directories to exclude from the queue of Units to run.",
		}, TerragruntExcludeDirFlagName),
		SliceWithDeprecatedFlag(opts, &cli.SliceFlag[string]{
			Name:        QueueIncludeDirFlagName,
			EnvVars:     EnvVars(QueueIncludeDirFlagName),
			Destination: &opts.IncludeDirs,
			Usage:       "Unix-style glob of directories to include from the queue of Units to run.",
		}, TerragruntIncludeDirFlagName),
		BoolWithDeprecatedFlag(opts, &cli.BoolFlag{
			Name:        DebugInputsFlagName,
			EnvVars:     EnvVars(DebugInputsFlagName),
			Destination: &opts.Debug,
			Usage:       "Write debug.tfvars to working folder to help root-cause issues.",
		}, TerragruntDebugFlagName),
		BoolWithDeprecatedFlag(opts, &cli.BoolFlag{
			Name:        UsePartialParseConfigCacheFlagName,
			EnvVars:     EnvVars(UsePartialParseConfigCacheFlagName),
			Destination: &opts.UsePartialParseConfigCache,
			Usage:       "Enables caching of includes during partial parsing operations. Will also be used for the --iam-role option if provided.",
		}),
		BoolWithDeprecatedFlag(opts, &cli.BoolFlag{
			Name:        DependencyFetchOutputFromStateFlagName,
			EnvVars:     EnvVars(DependencyFetchOutputFromStateFlagName),
			Destination: &opts.FetchDependencyOutputFromState,
			Usage:       "The option fetches dependency output directly from the state file instead of init dependencies and running terraform on them.",
		}, TerragruntFetchDependencyOutputFromStateFlagName),
		BoolWithDeprecatedFlag(opts, &cli.BoolFlag{
			Name:        TFForwardStdoutFlagName,
			EnvVars:     EnvVars(TFForwardStdoutFlagName),
			Destination: &opts.ForwardTFStdout,
			Usage:       "If specified, the output of OpenTofu/Terraform commands will be printed as is, without being integrated into the Terragrunt log.",
		}, TerragruntForwardTFStdoutFlagName),
		BoolWithDeprecatedFlag(opts, &cli.BoolFlag{
			Name:        QueueStrictIncludeFlagName,
			EnvVars:     EnvVars(QueueStrictIncludeFlagName),
			Destination: &opts.StrictInclude,
			Usage:       "If flag is set, only modules under the directories passed in with '--include-dir' will be included.",
		}, TerragruntStrictIncludeFlagName),
		SliceWithDeprecatedFlag(opts, &cli.SliceFlag[string]{
			Name:        UnitsThatIncludeFlagName,
			EnvVars:     EnvVars(UnitsThatIncludeFlagName),
			Destination: &opts.ModulesThatInclude,
			Usage:       "If flag is set, 'run-all' will only run the command against Terragrunt modules that include the specified file.",
		}, TerragruntModulesThatIncludeFlagName),
		SliceWithDeprecatedFlag(opts, &cli.SliceFlag[string]{
			Name:        QueueIncludeUnitsReadingFlagName,
			EnvVars:     EnvVars(QueueIncludeUnitsReadingFlagName),
			Destination: &opts.UnitsReading,
			Usage:       "If flag is set, 'run-all' will only run the command against Terragrunt units that read the specified file via an HCL function.",
		}, TerragruntUnitsReadingFlagName),
		BoolWithDeprecatedFlag(opts, &cli.BoolFlag{
			Name:        BackendRequireBootstrapFlagName,
			EnvVars:     EnvVars(BackendRequireBootstrapFlagName),
			Destination: &opts.FailIfBucketCreationRequired,
			Usage:       "When this flag is set Terragrunt will fail if the remote state bucket needs to be created.",
		}, TerragruntFailOnStateBucketCreationFlagName),
		BoolWithDeprecatedFlag(opts, &cli.BoolFlag{
			Name:        DisableBucketUpdateFlagName,
			EnvVars:     EnvVars(DisableBucketUpdateFlagName),
			Destination: &opts.DisableBucketUpdate,
			Usage:       "When this flag is set Terragrunt will not update the remote state bucket.",
		}),
		BoolWithDeprecatedFlag(opts, &cli.BoolFlag{
			Name:        DisableCommandValidationFlagName,
			EnvVars:     EnvVars(DisableCommandValidationFlagName),
			Destination: &opts.DisableCommandValidation,
			Usage:       "When this flag is set, Terragrunt will not validate the terraform command.",
		}),
		BoolWithDeprecatedFlag(opts, &cli.BoolFlag{
			Name:        NoDestroyDependenciesCheckFlagName,
			EnvVars:     EnvVars(NoDestroyDependenciesCheckFlagName),
			Destination: &opts.NoDestroyDependenciesCheck,
			Usage:       "When this flag is set, Terragrunt will not check for dependent modules when destroying.",
		}),
		// Terragrunt Provider Cache flags
		BoolWithDeprecatedFlag(opts, &cli.BoolFlag{
			Name:        ProviderCacheFlagName,
			EnvVars:     EnvVars(ProviderCacheFlagName),
			Destination: &opts.ProviderCache,
			Usage:       "Enables Terragrunt's provider caching.",
		}),
		GenericWithDeprecatedFlag(opts, &cli.GenericFlag[string]{
			Name:        ProviderCacheDirFlagName,
			EnvVars:     EnvVars(ProviderCacheDirFlagName),
			Destination: &opts.ProviderCacheDir,
			Usage:       "The path to the Terragrunt provider cache directory. By default, 'terragrunt/providers' folder in the user cache directory.",
		}),
		GenericWithDeprecatedFlag(opts, &cli.GenericFlag[string]{
			Name:        ProviderCacheTokenFlagName,
			EnvVars:     EnvVars(ProviderCacheTokenFlagName),
			Destination: &opts.ProviderCacheToken,
			Usage:       "The Token for authentication to the Terragrunt Provider Cache server. By default, assigned automatically.",
		}),
		GenericWithDeprecatedFlag(opts, &cli.GenericFlag[string]{
			Name:        ProviderCacheHostnameFlagName,
			EnvVars:     EnvVars(ProviderCacheHostnameFlagName),
			Destination: &opts.ProviderCacheHostname,
			Usage:       "The hostname of the Terragrunt Provider Cache server. By default, 'localhost'.",
		}),
		GenericWithDeprecatedFlag(opts, &cli.GenericFlag[int]{
			Name:        ProviderCachePortFlagName,
			EnvVars:     EnvVars(ProviderCachePortFlagName),
			Destination: &opts.ProviderCachePort,
			Usage:       "The port of the Terragrunt Provider Cache server. By default, assigned automatically.",
		}),
		SliceWithDeprecatedFlag(opts, &cli.SliceFlag[string]{
			Name:        ProviderCacheRegistryNamesFlagName,
			EnvVars:     EnvVars(ProviderCacheRegistryNamesFlagName),
			Destination: &opts.ProviderCacheRegistryNames,
			Usage:       "The list of remote registries to cached by Terragrunt Provider Cache server. By default, 'registry.terraform.io', 'registry.opentofu.org'.",
		}),
		GenericWithDeprecatedFlag(opts, &cli.GenericFlag[string]{
			Name:        AuthProviderCmdFlagName,
			EnvVars:     EnvVars(AuthProviderCmdFlagName),
			Destination: &opts.AuthProviderCmd,
			Usage:       "Run the provided command and arguments to authenticate Terragrunt dynamically when necessary.",
		}),
		MapWithDeprecatedFlag(opts, &cli.MapFlag[string, string]{
			Name:        FeatureMapFlagName,
			EnvVars:     EnvVars(FeatureMapFlagName),
			Destination: &opts.FeatureFlags,
			Usage:       "Set feature flags for the HCL code.",
			Splitter:    util.SplitComma,
		}),
		// Terragrunt engine flags
		BoolWithDeprecatedFlag(opts, &cli.BoolFlag{
			Name:        EngineEnableFlagName,
			EnvVars:     EnvVars(EngineEnableFlagName),
			Destination: &opts.EngineEnabled,
			Usage:       "Enable Terragrunt experimental engine.",
			Hidden:      true,
		}),
		GenericWithDeprecatedFlag(opts, &cli.GenericFlag[string]{
			Name:        EngineCachePathFlagName,
			EnvVars:     EnvVars(EngineCachePathFlagName),
			Destination: &opts.EngineCachePath,
			Usage:       "Cache path for Terragrunt engine files.",
			Hidden:      true,
		}),
		BoolWithDeprecatedFlag(opts, &cli.BoolFlag{
			Name:        EngineSkipCheckFlagName,
			EnvVars:     EnvVars(EngineSkipCheckFlagName),
			Destination: &opts.EngineSkipChecksumCheck,
			Usage:       "Skip checksum check for Terragrunt engine files.",
			Hidden:      true,
		}),
		GenericWithDeprecatedFlag(opts, &cli.GenericFlag[string]{
			Name:        EngineLogLevelFlagName,
			EnvVars:     EnvVars(EngineLogLevelFlagName),
			Destination: &opts.EngineLogLevel,
			Usage:       "Terragrunt engine log level.",
			Hidden:      true,
		}),
	}

	return flags.Sort()
}

func GetDefaultRootFileName(opts *options.TerragruntOptions) string {
	if control, ok := strict.GetStrictControl(strict.RootTerragruntHCL); ok {
		warn, triggered, err := control.Evaluate(opts)
		if err != nil {
			return config.RecommendedParentConfigName
		}

		if !triggered {
			opts.Logger.Warnf(warn)
		}
	}

	return config.DefaultTerragruntConfigPath
}

func NewRootFileNameFlag(opts *options.TerragruntOptions) cli.Flag {
	return &cli.GenericFlag[string]{
		Name:        RootFileNameFlagName,
		Destination: &opts.ScaffoldRootFileName,
		Usage:       "Name of the root Terragrunt configuration file, if used.",
		Action: func(_ *cli.Context, value string) error {
			if value == "" {
				return errors.New("root-file-name flag cannot be empty")
			}

			if value == opts.TerragruntConfigPath {
				if control, ok := strict.GetStrictControl(strict.RootTerragruntHCL); ok {
					warn, triggered, err := control.Evaluate(opts)
					if err != nil {
						return err
					}

					if !triggered {
						opts.Logger.Warnf(warn)
					}
				}
			}

			opts.ScaffoldRootFileName = value

			return nil
		},
	}
}

func NewNoIncludeRootFlag(opts *options.TerragruntOptions) cli.Flag {
	return &cli.BoolFlag{
		Name:        NoIncludeRootFlagName,
		Destination: &opts.ScaffoldNoIncludeRoot,
		Usage:       "Do not include root unit in scaffolding done by catalog.",
	}
}
