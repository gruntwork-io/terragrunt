// Package flags provides Terragrunt command flags.
package flags

import (
	"fmt"

	"github.com/gruntwork-io/go-commons/collections"
	"github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/gruntwork-io/terragrunt/internal/strict"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/pkg/cli"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/pkg/log/format"
	"github.com/gruntwork-io/terragrunt/pkg/log/format/placeholders"
	"github.com/gruntwork-io/terragrunt/shell"
	"github.com/gruntwork-io/terragrunt/util"
)

const (
	ConfigFlagName         = "config"
	TFPathFlagName         = "tfpath"
	NoAutoInitFlagName     = "no-auto-init"
	NoAutoRetryFlagName    = "no-auto-retry"
	NoAutoApproveFlagName  = "no-auto-approve"
	NonInteractiveFlagName = "non-interactive"
	WorkingDirFlagName     = "working-dir"
	DownloadDirFlagName    = "download-dir"
	SourceFlagName         = "source"
	SourceMapFlagName      = "source-map"
	SourceUpdateFlagName   = "source-update"

	// Assume IAM Role flags.
	IAMAssumeRoleFlagName                 = "iam-assume-role"
	IAMAssumeRoleDurationFlagName         = "iam-assume-role-duration"
	IAMAssumeRoleSessionNameFlagName      = "iam-assume-role-session-name"
	IAMAssumeRoleWebIdentityTokenFlagName = "iam-assume-role-web-identity-token"

	// Deprecated assume IAM Role flags.
	IAMRoleFlagName             = "iam-role"
	IAMWebIdentityTokenFlagName = "iam-web-identity-token"

	ParallelismFlagName                    = "parallelism"
	DebugFlagName                          = "debug"
	ModulesThatIncludeFlagName             = "modules-that-include"
	FetchDependencyOutputFromStateFlagName = "fetch-dependency-output-from-state"
	UsePartialParseConfigCacheFlagName     = "use-partial-parse-config-cache"
	FailOnStateBucketCreationFlagName      = "fail-on-state-bucket-creation"
	DisableBucketUpdateFlagName            = "disable-bucket-update"
	DisableCommandValidationFlagName       = "disable-command-validation"
	AuthProviderCmdFlagName                = "auth-provider-cmd"
	OutDirFlagName                         = "out-dir"
	JSONOutDirFlagName                     = "json-out-dir"
	NoDestroyDependenciesCheckFlagName     = "no-destroy-dependencies-check"

	// Queue related flags.
	IgnoreDependencyErrorsFlagName      = "ignore-dependency-errors"
	IgnoreDependencyOrderFlagName       = "ignore-dependency-order"
	IgnoreExternalDependenciesFlagName  = "ignore-external-dependencies"
	IncludeExternalDependenciesFlagName = "include-external-dependencies"
	ExcludesFileFlagName                = "excludes-file"
	ExcludeDirFlagName                  = "exclude-dir"
	IncludeDirFlagName                  = "include-dir"
	StrictIncludeFlagName               = "strict-include"
	UnitsReadingFlagName                = "queue-include-units-reading"

	// Logs related flags.
	LogLevelFlagName        = "log-level"
	LogDisableFlagName      = "log-disable"
	NoColorFlagName         = "no-color"
	ShowLogAbsPathsFlagName = "log-show-abs-paths"
	ForwardTFStdoutFlagName = "forward-tf-stdout"
	LogFormatFlagName       = "log-format"
	LogCustomFormatFlagName = "log-custom-format"

	// Deprecated flags.
	IncludeModulePrefixFlagName  = "include-module-prefix"
	DisableLogFormattingFlagName = "disable-log-formatting"
	JSONLogFlagName              = "json-log"
	TfLogJSONFlagName            = "tf-logs-to-json"

	// Strict Mode related flags.
	StrictModeFlagName    = "strict-mode"
	StrictControlFlagName = "strict-control"

	// Terragrunt Provider Cache related flags.
	ProviderCacheFlagName              = "provider-cache"
	ProviderCacheDirFlagName           = "provider-cache-dir"
	ProviderCacheHostnameFlagName      = "provider-cache-hostname"
	ProviderCachePortFlagName          = "provider-cache-port"
	ProviderCacheTokenFlagName         = "provider-cache-token"
	ProviderCacheRegistryNamesFlagName = "provider-cache-registry-names"

	FeatureMapFlagName = "feature"

	// Engine related environment variables.
	EngineEnableFlagName    = "experimental-engine"
	EngineCachePathFlagName = "engine-cache-path"
	EngineSkipCheckFlagName = "engine-skip-check"
	EngineLogLevelFlagName  = "engine-log-level"

	HelpFlagName    = "help"
	VersionFlagName = "version"
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
	return NewGenericFlag(opts, &cli.GenericFlag[string]{
		Name:        LogLevelFlagName,
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
		NewGenericFlag(opts, &cli.GenericFlag[string]{
			Name:        WorkingDirFlagName,
			Destination: &opts.WorkingDir,
			Usage:       "The path to the directory of Terragrunt configurations. Default is current directory.",
		}),
		NewBoolFlag(opts, &cli.BoolFlag{
			Name:        LogDisableFlagName,
			Usage:       "Disable logging.",
			Destination: &opts.DisableLog,
			Action: func(_ *cli.Context, _ bool) error {
				opts.ForwardTFStdout = true
				opts.LogFormatter.SetFormat(nil)
				return nil
			},
		}),
		NewBoolFlag(opts, &cli.BoolFlag{
			Name:        ShowLogAbsPathsFlagName,
			Destination: &opts.LogShowAbsPaths,
			Usage:       "Show absolute paths in logs.",
		}),
		NewBoolFlag(opts, &cli.BoolFlag{
			Name:        NoColorFlagName,
			Destination: &opts.DisableLogColors,
			Usage:       "Disable color output.",
			Action: func(_ *cli.Context, _ bool) error {
				opts.LogFormatter.DisableColors()
				return nil
			},
		}),
		NewGenericFlag(opts, &cli.GenericFlag[string]{
			Name:  LogFormatFlagName,
			Usage: "Set the log format.",
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
		NewGenericFlag(opts, &cli.GenericFlag[string]{
			Name:  LogCustomFormatFlagName,
			Usage: "Set the custom log formatting.",
			Action: func(_ *cli.Context, val string) error {
				phs, err := placeholders.Parse(val)
				if err != nil {
					return cli.NewExitError(errors.Errorf("flag --%s, %w", LogCustomFormatFlagName, err), 1)
				}

				opts.LogFormatter.SetFormat(phs)

				return nil
			},
		}),

		// Strict Mode flags.
		NewBoolFlag(opts, &cli.BoolFlag{
			Name:        StrictModeFlagName,
			Destination: &opts.StrictMode,
			Usage:       "Enables strict mode for Terragrunt. For more information, see https://terragrunt.gruntwork.io/docs/reference/strict-mode .",
		}),
		NewSliceFlag(opts, &cli.SliceFlag[string]{
			Name:        StrictControlFlagName,
			Destination: &opts.StrictControls,
			Usage:       "Enables specific strict controls. For a list of available controls, see https://terragrunt.gruntwork.io/docs/reference/strict-mode .",
			Action: func(ctx *cli.Context, val []string) error {
				if err := strict.StrictControls.ValidateControlNames(val); err != nil {
					return cli.NewExitError(err, 1)
				}

				return nil
			},
		}),

		// Deprecated flags.
		NewBoolFlag(opts, &cli.BoolFlag{
			Name:   IncludeModulePrefixFlagName,
			Usage:  "When this flag is set output from Terraform sub-commands is prefixed with module path.",
			Hidden: true,
			Action: func(ctx *cli.Context, _ bool) error {
				opts.Logger.Warnf("The %q flag is deprecated. Use the functionality-inverted %q flag instead. By default, Terraform/OpenTofu output is integrated into the Terragrunt log, which prepends additional data, such as timestamps and prefixes, to log entries.", IncludeModulePrefixFlagName, ForwardTFStdoutFlagName)
				return nil
			},
		}),
		NewBoolFlag(opts, &cli.BoolFlag{
			Name:        DisableLogFormattingFlagName,
			Destination: &opts.DisableLogFormatting,
			Usage:       "If specified, logs will be displayed in key/value format. By default, logs are formatted in a human readable format.",
			Hidden:      true,
			Action: func(_ *cli.Context, _ bool) error {
				opts.LogFormatter.SetFormat(format.NewKeyValueFormat())

				if control, ok := strict.GetStrictControl(strict.DisableLogFormatting); ok {
					warn, err := control.Evaluate(opts)
					if err != nil {
						return err
					}

					opts.Logger.Warnf(warn)
				}

				return nil
			},
		}),
		NewBoolFlag(opts, &cli.BoolFlag{
			Name:        JSONLogFlagName,
			Destination: &opts.JSONLogFormat,
			Usage:       "If specified, Terragrunt will output its logs in JSON format.",
			Hidden:      true,
			Action: func(_ *cli.Context, _ bool) error {
				opts.LogFormatter.SetFormat(format.NewJSONFormat())

				if control, ok := strict.GetStrictControl(strict.JSONLog); ok {
					warn, err := control.Evaluate(opts)
					if err != nil {
						return err
					}

					opts.Logger.Warnf(warn)
				}

				return nil
			},
		}),
		NewBoolFlag(opts, &cli.BoolFlag{
			Name:   TfLogJSONFlagName,
			Usage:  "If specified, Terragrunt will wrap Terraform stdout and stderr in JSON.",
			Hidden: true,
			Action: func(_ *cli.Context, _ bool) error {
				if control, ok := strict.GetStrictControl(strict.JSONLog); ok {
					warn, err := control.Evaluate(opts)
					if err != nil {
						return err
					}

					opts.Logger.Warnf(warn)
				}

				return nil
			},
		}),
	}

	flags = flags.Sort()
	flags = flags.Add(NewHelpVersionFlags(opts)...)

	return flags
}

// NewCommonFlags creates and returns global flags.
func NewCommonFlags(opts *options.TerragruntOptions) cli.Flags {
	flags := cli.Flags{
		NewGenericFlag(opts, &cli.GenericFlag[string]{
			Name:        ConfigFlagName,
			Destination: &opts.TerragruntConfigPath,
			Usage:       "The path to the Terragrunt config file. Default is terragrunt.hcl.",
		}),
		NewGenericFlag(opts, &cli.GenericFlag[string]{
			Name:        TFPathFlagName,
			Destination: &opts.TerraformPath,
			Usage:       "Path to the Terraform binary. Default is tofu (on PATH).",
		}),
		NewBoolFlag(opts, &cli.BoolFlag{
			Name:        NoAutoInitFlagName,
			Usage:       "Don't automatically run 'terraform/tofu init' during other terragrunt commands. You must run 'terragrunt init' manually.",
			Negative:    true,
			Destination: &opts.AutoInit,
		}),
		NewBoolFlag(opts, &cli.BoolFlag{
			Name:        NoAutoRetryFlagName,
			Destination: &opts.AutoRetry,
			Usage:       "Don't automatically re-run command in case of transient errors.",
			Negative:    true,
		}),
		NewBoolFlag(opts, &cli.BoolFlag{
			Name:        NoAutoApproveFlagName,
			Destination: &opts.RunAllAutoApprove,
			Usage:       "Don't automatically append `-auto-approve` to the underlying OpenTofu/Terraform commands run with 'run-all'.",
			Negative:    true,
		}),
		NewBoolFlag(opts, &cli.BoolFlag{
			Name:        NonInteractiveFlagName,
			Destination: &opts.NonInteractive,
			Usage:       `Assume "yes" for all prompts.`,
		}),
		NewGenericFlag(opts, &cli.GenericFlag[string]{
			Name:        DownloadDirFlagName,
			Destination: &opts.DownloadDir,
			Usage:       "The path to download OpenTofu/Terraform modules into. Default is .cache in the working directory.",
		}),
		NewGenericFlag(opts, &cli.GenericFlag[string]{
			Name:        SourceFlagName,
			Destination: &opts.Source,
			Usage:       "Download OpenTofu/Terraform configurations from the specified source into a temporary folder, and run Terraform in that temporary folder.",
		}),
		NewBoolFlag(opts, &cli.BoolFlag{
			Name:        SourceUpdateFlagName,
			Destination: &opts.SourceUpdate,
			Usage:       "Delete the contents of the temporary folder to clear out any old, cached source code before downloading new source code into it.",
		}),
		NewMapFlag(opts, &cli.MapFlag[string, string]{
			Name:        SourceMapFlagName,
			Destination: &opts.SourceMap,
			Usage:       "Replace any source URL (including the source URL of a config pulled in with dependency blocks) that has root source with dest.",
			Splitter:    util.SplitUrls,
		}),

		// Assume IAM Role flags.
		NewGenericFlag(opts, &cli.GenericFlag[string]{
			Name:        IAMAssumeRoleFlagName,
			Destination: &opts.IAMRoleOptions.RoleARN,
			Usage:       "Assume the specified IAM role before executing OpenTofu/Terraform. Can also be set via the TERRAGRUNT_IAM_ROLE environment variable.",
		}, IAMRoleFlagName),
		NewGenericFlag(opts, &cli.GenericFlag[int64]{
			Name:        IAMAssumeRoleDurationFlagName,
			Destination: &opts.IAMRoleOptions.AssumeRoleDuration,
			Usage:       "Session duration for IAM Assume Role session. Can also be set via the TERRAGRUNT_IAM_ASSUME_ROLE_DURATION environment variable.",
		}),
		NewGenericFlag(opts, &cli.GenericFlag[string]{
			Name:        IAMAssumeRoleSessionNameFlagName,
			Destination: &opts.IAMRoleOptions.AssumeRoleSessionName,
			Usage:       "Name for the IAM Assumed Role session. Can also be set via TERRAGRUNT_IAM_ASSUME_ROLE_SESSION_NAME environment variable.",
		}),
		NewGenericFlag(opts, &cli.GenericFlag[string]{
			Name:        IAMAssumeRoleWebIdentityTokenFlagName,
			Destination: &opts.IAMRoleOptions.WebIdentityToken,
			Usage:       "For AssumeRoleWithWebIdentity, the WebIdentity token. Can also be set via TERRAGRUNT_IAM_ASSUME_ROLE_WEB_IDENTITY_TOKEN environment variable",
		}, IAMWebIdentityTokenFlagName),

		NewBoolFlag(opts, &cli.BoolFlag{
			Name:        IgnoreDependencyErrorsFlagName,
			Destination: &opts.IgnoreDependencyErrors,
			Usage:       "*-all commands continue processing components even if a dependency fails.",
		}),
		NewBoolFlag(opts, &cli.BoolFlag{
			Name:        IgnoreDependencyOrderFlagName,
			Destination: &opts.IgnoreDependencyOrder,
			Usage:       "*-all commands will be run disregarding the dependencies",
		}),
		NewBoolFlag(opts, &cli.BoolFlag{
			Name:        IgnoreExternalDependenciesFlagName,
			Destination: &opts.IgnoreExternalDependencies,
			Usage:       "*-all commands will not attempt to include external dependencies",
		}),
		NewBoolFlag(opts, &cli.BoolFlag{
			Name:        IncludeExternalDependenciesFlagName,
			Destination: &opts.IncludeExternalDependencies,
			Usage:       "*-all commands will include external dependencies",
		}),
		NewGenericFlag(opts, &cli.GenericFlag[int]{
			Name:        ParallelismFlagName,
			Destination: &opts.Parallelism,
			Usage:       "*-all commands parallelism set to at most N modules",
		}),
		NewGenericFlag(opts, &cli.GenericFlag[string]{
			Name:        ExcludesFileFlagName,
			Destination: &opts.ExcludesFile,
			Usage:       "Path to a file with a list of directories that need to be excluded when running *-all commands.",
		}),
		NewSliceFlag(opts, &cli.SliceFlag[string]{
			Name:        ExcludeDirFlagName,
			Destination: &opts.ExcludeDirs,
			Usage:       "Unix-style glob of directories to exclude when running *-all commands.",
		}),
		NewSliceFlag(opts, &cli.SliceFlag[string]{
			Name:        IncludeDirFlagName,
			Destination: &opts.IncludeDirs,
			Usage:       "Unix-style glob of directories to include when running *-all commands",
		}),
		NewBoolFlag(opts, &cli.BoolFlag{
			Name:        DebugFlagName,
			Destination: &opts.Debug,
			Usage:       "Write debug.tfvars to working folder to help root-cause issues.",
		}),
		NewBoolFlag(opts, &cli.BoolFlag{
			Name:        UsePartialParseConfigCacheFlagName,
			Destination: &opts.UsePartialParseConfigCache,
			Usage:       "Enables caching of includes during partial parsing operations. Will also be used for the --iam-role option if provided.",
		}),
		NewBoolFlag(opts, &cli.BoolFlag{
			Name:        FetchDependencyOutputFromStateFlagName,
			Destination: &opts.FetchDependencyOutputFromState,
			Usage:       "The option fetches dependency output directly from the state file instead of init dependencies and running terraform on them.",
		}),
		NewBoolFlag(opts, &cli.BoolFlag{
			Name:        ForwardTFStdoutFlagName,
			Destination: &opts.ForwardTFStdout,
			Usage:       "If specified, the output of OpenTofu/Terraform commands will be printed as is, without being integrated into the Terragrunt log.",
		}),
		NewBoolFlag(opts, &cli.BoolFlag{
			Name:        StrictIncludeFlagName,
			Destination: &opts.StrictInclude,
			Usage:       "If flag is set, only modules under the directories passed in with '--include-dir' will be included.",
		}),
		NewSliceFlag(opts, &cli.SliceFlag[string]{
			Name:        ModulesThatIncludeFlagName,
			Destination: &opts.ModulesThatInclude,
			Usage:       "If flag is set, 'run-all' will only run the command against Terragrunt modules that include the specified file.",
		}),
		NewSliceFlag(opts, &cli.SliceFlag[string]{
			Name:        UnitsReadingFlagName,
			Destination: &opts.UnitsReading,
			Usage:       "If flag is set, 'run-all' will only run the command against Terragrunt units that read the specified file via an HCL function.",
		}),
		NewBoolFlag(opts, &cli.BoolFlag{
			Name:        FailOnStateBucketCreationFlagName,
			Destination: &opts.FailIfBucketCreationRequired,
			Usage:       "When this flag is set Terragrunt will fail if the remote state bucket needs to be created.",
		}),
		NewBoolFlag(opts, &cli.BoolFlag{
			Name:        DisableBucketUpdateFlagName,
			Destination: &opts.DisableBucketUpdate,
			Usage:       "When this flag is set Terragrunt will not update the remote state bucket.",
		}),
		NewBoolFlag(opts, &cli.BoolFlag{
			Name:        DisableCommandValidationFlagName,
			Destination: &opts.DisableCommandValidation,
			Usage:       "When this flag is set, Terragrunt will not validate the terraform command.",
		}),
		NewBoolFlag(opts, &cli.BoolFlag{
			Name:        NoDestroyDependenciesCheckFlagName,
			Destination: &opts.NoDestroyDependenciesCheck,
			Usage:       "When this flag is set, Terragrunt will not check for dependent modules when destroying.",
		}),
		// Terragrunt Provider Cache flags
		NewBoolFlag(opts, &cli.BoolFlag{
			Name:        ProviderCacheFlagName,
			Destination: &opts.ProviderCache,
			Usage:       "Enables Terragrunt's provider caching.",
		}),
		NewGenericFlag(opts, &cli.GenericFlag[string]{
			Name:        ProviderCacheDirFlagName,
			Destination: &opts.ProviderCacheDir,
			Usage:       "The path to the Terragrunt provider cache directory. By default, 'terragrunt/providers' folder in the user cache directory.",
		}),
		NewGenericFlag(opts, &cli.GenericFlag[string]{
			Name:        ProviderCacheTokenFlagName,
			Destination: &opts.ProviderCacheToken,
			Usage:       "The Token for authentication to the Terragrunt Provider Cache server. By default, assigned automatically.",
		}),
		NewGenericFlag(opts, &cli.GenericFlag[string]{
			Name:        ProviderCacheHostnameFlagName,
			Destination: &opts.ProviderCacheHostname,
			Usage:       "The hostname of the Terragrunt Provider Cache server. By default, 'localhost'.",
		}),
		NewGenericFlag(opts, &cli.GenericFlag[int]{
			Name:        ProviderCachePortFlagName,
			Destination: &opts.ProviderCachePort,
			Usage:       "The port of the Terragrunt Provider Cache server. By default, assigned automatically.",
		}),
		NewSliceFlag(opts, &cli.SliceFlag[string]{
			Name:        ProviderCacheRegistryNamesFlagName,
			Destination: &opts.ProviderCacheRegistryNames,
			Usage:       "The list of remote registries to cached by Terragrunt Provider Cache server. By default, 'registry.terraform.io', 'registry.opentofu.org'.",
		}),
		NewGenericFlag(opts, &cli.GenericFlag[string]{
			Name:        AuthProviderCmdFlagName,
			Destination: &opts.AuthProviderCmd,
			Usage:       "The command and arguments that can be used to fetch authentication configurations.",
		}),
		NewMapFlag(opts, &cli.MapFlag[string, string]{
			Name:        FeatureMapFlagName,
			Destination: &opts.FeatureFlags,
			Usage:       "Set feature flags for the HCL code.",
			Splitter:    util.SplitComma,
		}),
		// Terragrunt engine flags
		NewBoolFlag(opts, &cli.BoolFlag{
			Name:        EngineEnableFlagName,
			Destination: &opts.EngineEnabled,
			Usage:       "Enable Terragrunt experimental engine.",
			Hidden:      true,
		}),
		NewGenericFlag(opts, &cli.GenericFlag[string]{
			Name:        EngineCachePathFlagName,
			Destination: &opts.EngineCachePath,
			Usage:       "Cache path for Terragrunt engine files.",
			Hidden:      true,
		}),
		NewBoolFlag(opts, &cli.BoolFlag{
			Name:        EngineSkipCheckFlagName,
			Destination: &opts.EngineSkipChecksumCheck,
			Usage:       "Skip checksum check for Terragrunt engine files.",
			Hidden:      true,
		}),
		NewGenericFlag(opts, &cli.GenericFlag[string]{
			Name:        EngineLogLevelFlagName,
			Destination: &opts.EngineLogLevel,
			Usage:       "Terragrunt engine log level.",
			Hidden:      true,
		}),
	}

	return flags.Sort()
}
