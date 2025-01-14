// Package run provides Terragrunt command flags.
package run

import (
	"github.com/gruntwork-io/terragrunt/cli/flags"
	"github.com/gruntwork-io/terragrunt/internal/cli"
	"github.com/gruntwork-io/terragrunt/internal/strict"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/pkg/log/format"
	"github.com/gruntwork-io/terragrunt/util"
)

const (
	AllFlagName                            = "all"
	GraphFlagName                          = "graph"
	ConfigFlagName                         = "config"
	NoAutoInitFlagName                     = "no-auto-init"
	NoAutoRetryFlagName                    = "no-auto-retry"
	NoAutoApproveFlagName                  = "no-auto-approve"
	DownloadDirFlagName                    = "download-dir"
	TFForwardStdoutFlagName                = "tf-forward-stdout"
	TFPathFlagName                         = "tf-path"
	FeatureFlagName                        = "feature"
	ParallelismFlagName                    = "parallelism"
	DebugInputsFlagName                    = "debug-inputs"
	UnitsThatIncludeFlagName               = "units-that-include"
	DependencyFetchOutputFromStateFlagName = "dependency-fetch-output-from-state"
	UsePartialParseConfigCacheFlagName     = "use-partial-parse-config-cache"

	BackendRequireBootstrapFlagName = "backend-require-bootstrap"
	DisableBucketUpdateFlagName     = "disable-bucket-update"

	DisableCommandValidationFlagName   = "disable-command-validation"
	AuthProviderCmdFlagName            = "auth-provider-cmd"
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
	QueueExcludesFileFlagName        = "queue-excludes-file"
	QueueIncludeDirFlagName          = "queue-include-dir"
	QueueIncludeExternalFlagName     = "queue-include-external"
	QueueStrictIncludeFlagName       = "queue-strict-include"
	QueueIncludeUnitsReadingFlagName = "queue-include-units-reading"

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

	// Renamed flags.

	TerragruntFailOnStateBucketCreationFlagName      = flags.DeprecatedFlagNamePrefix + "fail-on-state-bucket-creation"      // `backend-require-bootstrap`
	TerragruntModulesThatIncludeFlagName             = flags.DeprecatedFlagNamePrefix + "modules-that-include"               // `units-that-include`
	TerragruntForwardTFStdoutFlagName                = flags.DeprecatedFlagNamePrefix + "forward-tf-stdout"                  // `tf-forward-stdout`.
	TerragruntTFPathFlagName                         = flags.DeprecatedFlagNamePrefix + "tfpath"                             // `tf-path`.
	TerragruntDownloadFlagName                       = flags.DeprecatedFlagNamePrefix + "download"                           // `download-dir` for old `TERRAGRUNT_DOWNLOAD` env var.
	TerragruntIAMRoleFlagName                        = flags.DeprecatedFlagNamePrefix + "iam-role"                           // `iam-assume-role`.
	TerragruntIAMWebIdentityTokenFlagName            = flags.DeprecatedFlagNamePrefix + "iam-web-identity-token"             // `iam-assume-role-web-identity-token`.
	TerragruntDebugFlagName                          = flags.DeprecatedFlagNamePrefix + "debug"                              // `debug-inputs`.
	TerragruntFetchDependencyOutputFromStateFlagName = flags.DeprecatedFlagNamePrefix + "fetch-dependency-output-from-state" // `dependency-fetch-output-from-state`.
	TerragruntIgnoreDependencyOrderFlagName          = flags.DeprecatedFlagNamePrefix + "ignore-dependency-order"            // `queue-ignore-dag-order`.
	TerragruntIgnoreExternalDependenciesFlagName     = flags.DeprecatedFlagNamePrefix + "ignore-external-dependencies"       // `queue-exclude-external`.
	TerragruntExcludeDirFlagName                     = flags.DeprecatedFlagNamePrefix + "exclude-dir"                        // `queue-exclude-dir`.
	TerragruntExcludesFileFlagName                   = flags.DeprecatedFlagNamePrefix + "excludes-file"                      // `queue-excludes-file`.
	TerragruntIncludeDirFlagName                     = flags.DeprecatedFlagNamePrefix + "include-dir"                        // `queue-include-dir`.
	TerragruntIncludeExternalDependenciesFlagName    = flags.DeprecatedFlagNamePrefix + "include-external-dependencies"      // `queue-include-external`.
	TerragruntStrictIncludeFlagName                  = flags.DeprecatedFlagNamePrefix + "strict-include"                     // `queue-strict-include`.
	TerragruntUnitsReadingFlagName                   = flags.DeprecatedFlagNamePrefix + "queue-include-units-reading"        // `queue-include-units-reading`.
	TerragruntIgnoreDependencyErrorsFlagName         = flags.DeprecatedFlagNamePrefix + "ignore-dependency-errors"           // `queue-ignore-errors`.

	// Deprecated flags.

	TerragruntIncludeModulePrefixFlagName = flags.DeprecatedFlagNamePrefix + "include-module-prefix"
	TerragruntTfLogJSONFlagName           = flags.DeprecatedFlagNamePrefix + "tf-logs-to-json"
)

// NewFlags creates and returns global flags.
func NewFlags(opts *options.TerragruntOptions) cli.Flags {
	flags := cli.Flags{
		&cli.BoolFlag{
			Name:        AllFlagName,
			EnvVars:     flags.EnvVars(AllFlagName),
			Destination: &opts.SourceUpdate,
			Usage:       `Run the specified OpenTofu/Terraform command on the "Stack" of Units in the current directory.`,
		},
		&cli.BoolFlag{
			Name:        GraphFlagName,
			EnvVars:     flags.EnvVars(GraphFlagName),
			Destination: &opts.SourceUpdate,
			Usage:       "Run the specified OpenTofu/Terraform command following the Directed Acyclic Graph (DAG) of dependencies.",
		},

		//  Backward compatibility with `terragrunt-` prefix flags.

		flags.GenericWithDeprecatedFlag(opts, &cli.GenericFlag[string]{
			Name:        ConfigFlagName,
			EnvVars:     flags.EnvVars(ConfigFlagName),
			Destination: &opts.TerragruntConfigPath,
			Usage:       "The path to the Terragrunt config file. Default is terragrunt.hcl.",
		}),
		flags.GenericWithDeprecatedFlag(opts, &cli.GenericFlag[string]{
			Name:        TFPathFlagName,
			EnvVars:     flags.EnvVars(TFPathFlagName),
			Destination: &opts.TerraformPath,
			Usage:       "Path to the OpenTofu/Terraform binary. Default is tofu (on PATH).",
		}, TerragruntTFPathFlagName),
		flags.BoolWithDeprecatedFlag(opts, &cli.BoolFlag{
			Name:        NoAutoInitFlagName,
			EnvVars:     flags.EnvVars(NoAutoInitFlagName),
			Usage:       "Don't automatically run 'terraform/tofu init' during other terragrunt commands. You must run 'terragrunt init' manually.",
			Negative:    true,
			Destination: &opts.AutoInit,
		}),
		flags.BoolWithDeprecatedFlag(opts, &cli.BoolFlag{
			Name:        NoAutoRetryFlagName,
			EnvVars:     flags.EnvVars(NoAutoRetryFlagName),
			Destination: &opts.AutoRetry,
			Usage:       "Don't automatically re-run command in case of transient errors.",
			Negative:    true,
		}),
		flags.BoolWithDeprecatedFlag(opts, &cli.BoolFlag{
			Name:        NoAutoApproveFlagName,
			EnvVars:     flags.EnvVars(NoAutoApproveFlagName),
			Destination: &opts.RunAllAutoApprove,
			Usage:       "Don't automatically append `-auto-approve` to the underlying OpenTofu/Terraform commands run with 'run-all'.",
			Negative:    true,
		}),
		flags.GenericWithDeprecatedFlag(opts, &cli.GenericFlag[string]{
			Name:        DownloadDirFlagName,
			EnvVars:     flags.EnvVars(DownloadDirFlagName),
			Destination: &opts.DownloadDir,
			Usage:       "The path to download OpenTofu/Terraform modules into. Default is .cache in the working directory.",
		}, flags.DeprecatedFlagNamePrefix+DownloadDirFlagName, TerragruntDownloadFlagName), // the old flag had `terragrunt-download-dir` name and `TERRAGRUNT_DOWNLOAD` env.
		flags.GenericWithDeprecatedFlag(opts, &cli.GenericFlag[string]{
			Name:        SourceFlagName,
			EnvVars:     flags.EnvVars(SourceFlagName),
			Destination: &opts.Source,
			Usage:       "Download OpenTofu/Terraform configurations from the specified source into a temporary folder, and run Terraform in that temporary folder.",
		}),
		flags.BoolWithDeprecatedFlag(opts, &cli.BoolFlag{
			Name:        SourceUpdateFlagName,
			EnvVars:     flags.EnvVars(SourceUpdateFlagName),
			Destination: &opts.SourceUpdate,
			Usage:       "Delete the contents of the temporary folder to clear out any old, cached source code before downloading new source code into it.",
		}),
		flags.MapWithDeprecatedFlag(opts, &cli.MapFlag[string, string]{
			Name:        SourceMapFlagName,
			EnvVars:     flags.EnvVars(SourceMapFlagName),
			Destination: &opts.SourceMap,
			Usage:       "Replace any source URL (including the source URL of a config pulled in with dependency blocks) that has root source with dest.",
			Splitter:    util.SplitUrls,
		}),

		// Assume IAM Role flags.
		flags.GenericWithDeprecatedFlag(opts, &cli.GenericFlag[string]{
			Name:        IAMAssumeRoleFlagName,
			EnvVars:     flags.EnvVars(IAMAssumeRoleFlagName),
			Destination: &opts.IAMRoleOptions.RoleARN,
			Usage:       "Assume the specified IAM role before executing OpenTofu/Terraform.",
		}, TerragruntIAMRoleFlagName),
		flags.GenericWithDeprecatedFlag(opts, &cli.GenericFlag[int64]{
			Name:        IAMAssumeRoleDurationFlagName,
			EnvVars:     flags.EnvVars(IAMAssumeRoleDurationFlagName),
			Destination: &opts.IAMRoleOptions.AssumeRoleDuration,
			Usage:       "Session duration for IAM Assume Role session.",
		}),
		flags.GenericWithDeprecatedFlag(opts, &cli.GenericFlag[string]{
			Name:        IAMAssumeRoleSessionNameFlagName,
			EnvVars:     flags.EnvVars(IAMAssumeRoleSessionNameFlagName),
			Destination: &opts.IAMRoleOptions.AssumeRoleSessionName,
			Usage:       "Name for the IAM Assumed Role session.",
		}),
		flags.GenericWithDeprecatedFlag(opts, &cli.GenericFlag[string]{
			Name:        IAMAssumeRoleWebIdentityTokenFlagName,
			EnvVars:     flags.EnvVars(IAMAssumeRoleWebIdentityTokenFlagName),
			Destination: &opts.IAMRoleOptions.WebIdentityToken,
			Usage:       "For AssumeRoleWithWebIdentity, the WebIdentity token.",
		}, TerragruntIAMWebIdentityTokenFlagName),

		flags.BoolWithDeprecatedFlag(opts, &cli.BoolFlag{
			Name:        QueueIgnoreErrorsFlagName,
			EnvVars:     flags.EnvVars(QueueIgnoreErrorsFlagName),
			Destination: &opts.IgnoreDependencyErrors,
			Usage:       "Continue processing Units even if a dependency fails.",
		}, TerragruntIgnoreDependencyErrorsFlagName),
		flags.BoolWithDeprecatedFlag(opts, &cli.BoolFlag{
			Name:        QueueIgnoreDAGOrderFlagName,
			EnvVars:     flags.EnvVars(QueueIgnoreDAGOrderFlagName),
			Destination: &opts.IgnoreDependencyOrder,
			Usage:       "Ignore DAG order for --all commands.",
		}, TerragruntIgnoreDependencyOrderFlagName),
		flags.BoolWithDeprecatedFlag(opts, &cli.BoolFlag{
			Name:        QueueExcludeExternalFlagName,
			EnvVars:     flags.EnvVars(QueueExcludeExternalFlagName),
			Destination: &opts.IgnoreExternalDependencies,
			Usage:       "Ignore external dependencies for --all commands.",
		}, TerragruntIgnoreExternalDependenciesFlagName),
		flags.BoolWithDeprecatedFlag(opts, &cli.BoolFlag{
			Name:        QueueIncludeExternalFlagName,
			EnvVars:     flags.EnvVars(QueueIncludeExternalFlagName),
			Destination: &opts.IncludeExternalDependencies,
			Usage:       "Include external dependencies for --all commands without asking.",
		}, TerragruntIncludeExternalDependenciesFlagName),
		flags.GenericWithDeprecatedFlag(opts, &cli.GenericFlag[int]{
			Name:        ParallelismFlagName,
			EnvVars:     flags.EnvVars(ParallelismFlagName),
			Destination: &opts.Parallelism,
			Usage:       "Parallelism for --all commands.",
		}),
		flags.GenericWithDeprecatedFlag(opts, &cli.GenericFlag[string]{
			Name:        QueueExcludesFileFlagName,
			EnvVars:     flags.EnvVars(QueueExcludesFileFlagName),
			Destination: &opts.ExcludesFile,
			Usage:       "Path to a file with a list of directories that need to be excluded when running *-all commands.",
		}, TerragruntExcludesFileFlagName),
		flags.SliceWithDeprecatedFlag(opts, &cli.SliceFlag[string]{
			Name:        QueueExcludeDirFlagName,
			EnvVars:     flags.EnvVars(QueueExcludeDirFlagName),
			Destination: &opts.ExcludeDirs,
			Usage:       "Unix-style glob of directories to exclude from the queue of Units to run.",
		}, TerragruntExcludeDirFlagName),
		flags.SliceWithDeprecatedFlag(opts, &cli.SliceFlag[string]{
			Name:        QueueIncludeDirFlagName,
			EnvVars:     flags.EnvVars(QueueIncludeDirFlagName),
			Destination: &opts.IncludeDirs,
			Usage:       "Unix-style glob of directories to include from the queue of Units to run.",
		}, TerragruntIncludeDirFlagName),
		flags.BoolWithDeprecatedFlag(opts, &cli.BoolFlag{
			Name:        DebugInputsFlagName,
			EnvVars:     flags.EnvVars(DebugInputsFlagName),
			Destination: &opts.Debug,
			Usage:       "Write debug.tfvars to working folder to help root-cause issues.",
		}, TerragruntDebugFlagName),
		flags.BoolWithDeprecatedFlag(opts, &cli.BoolFlag{
			Name:        UsePartialParseConfigCacheFlagName,
			EnvVars:     flags.EnvVars(UsePartialParseConfigCacheFlagName),
			Destination: &opts.UsePartialParseConfigCache,
			Usage:       "Enables caching of includes during partial parsing operations. Will also be used for the --iam-role option if provided.",
		}),
		flags.BoolWithDeprecatedFlag(opts, &cli.BoolFlag{
			Name:        DependencyFetchOutputFromStateFlagName,
			EnvVars:     flags.EnvVars(DependencyFetchOutputFromStateFlagName),
			Destination: &opts.FetchDependencyOutputFromState,
			Usage:       "The option fetches dependency output directly from the state file instead of init dependencies and running terraform on them.",
		}, TerragruntFetchDependencyOutputFromStateFlagName),
		flags.BoolWithDeprecatedFlag(opts, &cli.BoolFlag{
			Name:        TFForwardStdoutFlagName,
			EnvVars:     flags.EnvVars(TFForwardStdoutFlagName),
			Destination: &opts.ForwardTFStdout,
			Usage:       "If specified, the output of OpenTofu/Terraform commands will be printed as is, without being integrated into the Terragrunt log.",
		}, TerragruntForwardTFStdoutFlagName),
		flags.BoolWithDeprecatedFlag(opts, &cli.BoolFlag{
			Name:        QueueStrictIncludeFlagName,
			EnvVars:     flags.EnvVars(QueueStrictIncludeFlagName),
			Destination: &opts.StrictInclude,
			Usage:       "If flag is set, only modules under the directories passed in with '--include-dir' will be included.",
		}, TerragruntStrictIncludeFlagName),
		flags.SliceWithDeprecatedFlag(opts, &cli.SliceFlag[string]{
			Name:        UnitsThatIncludeFlagName,
			EnvVars:     flags.EnvVars(UnitsThatIncludeFlagName),
			Destination: &opts.ModulesThatInclude,
			Usage:       "If flag is set, 'run-all' will only run the command against Terragrunt modules that include the specified file.",
		}, TerragruntModulesThatIncludeFlagName),
		flags.SliceWithDeprecatedFlag(opts, &cli.SliceFlag[string]{
			Name:        QueueIncludeUnitsReadingFlagName,
			EnvVars:     flags.EnvVars(QueueIncludeUnitsReadingFlagName),
			Destination: &opts.UnitsReading,
			Usage:       "If flag is set, 'run-all' will only run the command against Terragrunt units that read the specified file via an HCL function.",
		}, TerragruntUnitsReadingFlagName),
		flags.BoolWithDeprecatedFlag(opts, &cli.BoolFlag{
			Name:        BackendRequireBootstrapFlagName,
			EnvVars:     flags.EnvVars(BackendRequireBootstrapFlagName),
			Destination: &opts.FailIfBucketCreationRequired,
			Usage:       "When this flag is set Terragrunt will fail if the remote state bucket needs to be created.",
		}, TerragruntFailOnStateBucketCreationFlagName),
		flags.BoolWithDeprecatedFlag(opts, &cli.BoolFlag{
			Name:        DisableBucketUpdateFlagName,
			EnvVars:     flags.EnvVars(DisableBucketUpdateFlagName),
			Destination: &opts.DisableBucketUpdate,
			Usage:       "When this flag is set Terragrunt will not update the remote state bucket.",
		}),
		flags.BoolWithDeprecatedFlag(opts, &cli.BoolFlag{
			Name:        DisableCommandValidationFlagName,
			EnvVars:     flags.EnvVars(DisableCommandValidationFlagName),
			Destination: &opts.DisableCommandValidation,
			Usage:       "When this flag is set, Terragrunt will not validate the terraform command.",
		}),
		flags.BoolWithDeprecatedFlag(opts, &cli.BoolFlag{
			Name:        NoDestroyDependenciesCheckFlagName,
			EnvVars:     flags.EnvVars(NoDestroyDependenciesCheckFlagName),
			Destination: &opts.NoDestroyDependenciesCheck,
			Usage:       "When this flag is set, Terragrunt will not check for dependent modules when destroying.",
		}),
		// Terragrunt Provider Cache flags
		flags.BoolWithDeprecatedFlag(opts, &cli.BoolFlag{
			Name:        ProviderCacheFlagName,
			EnvVars:     flags.EnvVars(ProviderCacheFlagName),
			Destination: &opts.ProviderCache,
			Usage:       "Enables Terragrunt's provider caching.",
		}),
		flags.GenericWithDeprecatedFlag(opts, &cli.GenericFlag[string]{
			Name:        ProviderCacheDirFlagName,
			EnvVars:     flags.EnvVars(ProviderCacheDirFlagName),
			Destination: &opts.ProviderCacheDir,
			Usage:       "The path to the Terragrunt provider cache directory. By default, 'terragrunt/providers' folder in the user cache directory.",
		}),
		flags.GenericWithDeprecatedFlag(opts, &cli.GenericFlag[string]{
			Name:        ProviderCacheTokenFlagName,
			EnvVars:     flags.EnvVars(ProviderCacheTokenFlagName),
			Destination: &opts.ProviderCacheToken,
			Usage:       "The Token for authentication to the Terragrunt Provider Cache server. By default, assigned automatically.",
		}),
		flags.GenericWithDeprecatedFlag(opts, &cli.GenericFlag[string]{
			Name:        ProviderCacheHostnameFlagName,
			EnvVars:     flags.EnvVars(ProviderCacheHostnameFlagName),
			Destination: &opts.ProviderCacheHostname,
			Usage:       "The hostname of the Terragrunt Provider Cache server. By default, 'localhost'.",
		}),
		flags.GenericWithDeprecatedFlag(opts, &cli.GenericFlag[int]{
			Name:        ProviderCachePortFlagName,
			EnvVars:     flags.EnvVars(ProviderCachePortFlagName),
			Destination: &opts.ProviderCachePort,
			Usage:       "The port of the Terragrunt Provider Cache server. By default, assigned automatically.",
		}),
		flags.SliceWithDeprecatedFlag(opts, &cli.SliceFlag[string]{
			Name:        ProviderCacheRegistryNamesFlagName,
			EnvVars:     flags.EnvVars(ProviderCacheRegistryNamesFlagName),
			Destination: &opts.ProviderCacheRegistryNames,
			Usage:       "The list of remote registries to cached by Terragrunt Provider Cache server. By default, 'registry.terraform.io', 'registry.opentofu.org'.",
		}),
		flags.GenericWithDeprecatedFlag(opts, &cli.GenericFlag[string]{
			Name:        AuthProviderCmdFlagName,
			EnvVars:     flags.EnvVars(AuthProviderCmdFlagName),
			Destination: &opts.AuthProviderCmd,
			Usage:       "Run the provided command and arguments to authenticate Terragrunt dynamically when necessary.",
		}),
		flags.MapWithDeprecatedFlag(opts, &cli.MapFlag[string, string]{
			Name:     FeatureFlagName,
			EnvVars:  flags.EnvVars(FeatureFlagName),
			Usage:    "Set feature flags for the HCL code.",
			Splitter: util.SplitComma,
			Action: func(_ *cli.Context, value map[string]string) error {
				for key, val := range value {
					opts.FeatureFlags.Store(key, val)
				}

				return nil
			},
		}),
		// Terragrunt engine flags
		flags.BoolWithDeprecatedFlag(opts, &cli.BoolFlag{
			Name:        EngineEnableFlagName,
			EnvVars:     flags.EnvVars(EngineEnableFlagName),
			Destination: &opts.EngineEnabled,
			Usage:       "Enable Terragrunt experimental engine.",
			Hidden:      true,
		}),
		flags.GenericWithDeprecatedFlag(opts, &cli.GenericFlag[string]{
			Name:        EngineCachePathFlagName,
			EnvVars:     flags.EnvVars(EngineCachePathFlagName),
			Destination: &opts.EngineCachePath,
			Usage:       "Cache path for Terragrunt engine files.",
			Hidden:      true,
		}),
		flags.BoolWithDeprecatedFlag(opts, &cli.BoolFlag{
			Name:        EngineSkipCheckFlagName,
			EnvVars:     flags.EnvVars(EngineSkipCheckFlagName),
			Destination: &opts.EngineSkipChecksumCheck,
			Usage:       "Skip checksum check for Terragrunt engine files.",
			Hidden:      true,
		}),
		flags.GenericWithDeprecatedFlag(opts, &cli.GenericFlag[string]{
			Name:        EngineLogLevelFlagName,
			EnvVars:     flags.EnvVars(EngineLogLevelFlagName),
			Destination: &opts.EngineLogLevel,
			Usage:       "Terragrunt engine log level.",
			Hidden:      true,
		}),

		// Deprecated flags.
		&cli.BoolFlag{
			Name:    TerragruntIncludeModulePrefixFlagName,
			EnvVars: flags.EnvVars(TerragruntIncludeModulePrefixFlagName),
			Usage:   "When this flag is set output from Terraform sub-commands is prefixed with module path.",
			Hidden:  true,
			Action: func(_ *cli.Context, _ bool) error {
				opts.Logger.Warnf("The %q flag is deprecated. Use the functionality-inverted %q flag instead. By default, Terraform/OpenTofu output is integrated into the Terragrunt log, which prepends additional data, such as timestamps and prefixes, to log entries.", TerragruntIncludeModulePrefixFlagName, TFForwardStdoutFlagName)

				return nil
			},
		},
		&cli.BoolFlag{
			Name:    TerragruntTfLogJSONFlagName,
			EnvVars: flags.EnvVars(TerragruntTfLogJSONFlagName),
			Usage:   "If specified, Terragrunt will wrap Terraform stdout and stderr in JSON.",
			Hidden:  true,
			Action: func(_ *cli.Context, _ bool) error {

				newFlagName := flags.LogCustomFormatFlagName + "=" + format.JSONFormatName

				if err := opts.StrictControls.Evaluate(opts.Logger, strict.DeprecatedFlags, TerragruntTfLogJSONFlagName, newFlagName); err != nil {
					return cli.NewExitError(err, cli.ExitCodeGeneralError)
				}

				return nil
			},
		},
	}

	return flags.Sort()
}
