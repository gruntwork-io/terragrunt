// Package run provides Terragrunt command flags.
package run

import (
	"strconv"

	"github.com/gruntwork-io/terragrunt/cli/flags"
	"github.com/gruntwork-io/terragrunt/internal/cli"
	"github.com/gruntwork-io/terragrunt/internal/strict/controls"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/util"
)

const (
	ConfigFlagName                         = "config"
	NoAutoInitFlagName                     = "no-auto-init"
	NoAutoRetryFlagName                    = "no-auto-retry"
	NoAutoApproveFlagName                  = "no-auto-approve"
	DownloadDirFlagName                    = "download-dir"
	TFForwardStdoutFlagName                = "tf-forward-stdout"
	TFPathFlagName                         = "tf-path"
	FeatureFlagName                        = "feature"
	ParallelismFlagName                    = "parallelism"
	InputsDebugFlagName                    = "inputs-debug"
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

	DeprecatedFailOnStateBucketCreationFlagName      = "fail-on-state-bucket-creation"      // `backend-require-bootstrap`
	DeprecatedModulesThatIncludeFlagName             = "modules-that-include"               // `units-that-include`
	DeprecatedForwardTFStdoutFlagName                = "forward-tf-stdout"                  // `tf-forward-stdout`.
	DeprecatedTfpathFlagName                         = "tfpath"                             // `tf-path`.
	DeprecatedDownloadEnvVar                         = "DOWNLOAD"                           // `download-dir` for old `TERRAGRUNT_DOWNLOAD` env var.
	DeprecatedIAMRoleFlagName                        = "iam-role"                           // `iam-assume-role`.
	DeprecatedIAMWebIdentityTokenFlagName            = "iam-web-identity-token"             // `iam-assume-role-web-identity-token`.
	DeprecatedDebugFlagName                          = "debug"                              // `inputs-debug`.
	DeprecatedFetchDependencyOutputFromStateFlagName = "fetch-dependency-output-from-state" // `dependency-fetch-output-from-state`.
	DeprecatedIgnoreDependencyOrderFlagName          = "ignore-dependency-order"            // `queue-ignore-dag-order`.
	DeprecatedIgnoreExternalDependenciesFlagName     = "ignore-external-dependencies"       // `queue-exclude-external`.
	DeprecatedExcludeDirFlagName                     = "exclude-dir"                        // `queue-exclude-dir`.
	DeprecatedExcludesFileFlagName                   = "excludes-file"                      // `queue-excludes-file`.
	DeprecatedIncludeDirFlagName                     = "include-dir"                        // `queue-include-dir`.
	DeprecatedIncludeExternalDependenciesFlagName    = "include-external-dependencies"      // `queue-include-external`.
	DeprecatedStrictIncludeFlagName                  = "strict-include"                     // `queue-strict-include`.
	DeprecatedUnitsReadingFlagName                   = "queue-include-units-reading"        // `queue-include-units-reading`.
	DeprecatedIgnoreDependencyErrorsFlagName         = "ignore-dependency-errors"           // `queue-ignore-errors`.
	DeprecatedConfigFlagName                         = "config"
	DeprecatedDownloadDirFlagName                    = "download-dir"
	DeprecatedIAMAssumeRoleWebIdentityTokenFlagName  = "iam-assume-role-web-identity-token"
	DeprecatedSourceMapFlagName                      = "source-map"
	DeprecatedSourceUpdateFlagName                   = "source-update"
	DeprecatedSourceFlagName                         = "source"
	DeprecatedNoAutoApproveFlagName                  = "no-auto-approve"
	DeprecatedAutoApproveFlagName                    = "auto-approve"
	DeprecatedNoAutoRetryFlagName                    = "no-auto-retry"
	DeprecatedAutoRetryFlagName                      = "auto-retry"
	DeprecatedNoAutoInitFlagName                     = "no-auto-init"
	DeprecatedAutoInitFlagName                       = "auto-init"
	DeprecatedNoDestroyDependenciesCheckFlagName     = "no-destroy-dependencies-check"
	DeprecatedDisableCommandValidationFlagName       = "disable-command-validation"
	DeprecatedDisableBucketUpdateFlagName            = "disable-bucket-update"
	DeprecatedUsePartialParseConfigCacheFlagName     = "use-partial-parse-config-cache"
	DeprecatedParallelismFlagName                    = "parallelism"
	DeprecatedIAMAssumeRoleSessionNameFlagName       = "iam-assume-role-session-name"
	DeprecatedIAMAssumeRoleDurationFlagName          = "iam-assume-role-duration"
	DeprecatedProviderCacheFlagName                  = "provider-cache"
	DeprecatedProviderCacheDirFlagName               = "provider-cache-dir"
	DeprecatedProviderCacheHostnameFlagName          = "provider-cache-hostname"
	DeprecatedProviderCachePortFlagName              = "provider-cache-port"
	DeprecatedProviderCacheTokenFlagName             = "provider-cache-token"
	DeprecatedProviderCacheRegistryNamesFlagName     = "provider-cache-registry-names"
	DeprecatedFeatureFlagName                        = "feature"
	DeprecatedAuthProviderCmdFlagName                = "auth-provider-cmd"
	DeprecatedEngineEnableFlagName                   = "experimental-engine"
	DeprecatedEngineCachePathFlagName                = "engine-cache-path"
	DeprecatedEngineSkipCheckFlagName                = "engine-skip-check"
	DeprecatedEngineLogLevelFlagName                 = "engine-log-level"

	// Deprecated flags.

	DeprecatedIncludeModulePrefixFlagName = "include-module-prefix"
)

// NewFlags creates and returns global flags.
func NewFlags(opts *options.TerragruntOptions, prefix flags.Prefix) cli.Flags {
	tgPrefix := prefix.Prepend(flags.TgPrefix)
	terragruntPrefix := flags.Prefix{flags.TerragruntPrefix}
	terragruntPrefixControl := flags.StrictControlsByCommand(opts.StrictControls, CommandName)
	legacyLogsControl := flags.StrictControlsByCommand(opts.StrictControls, CommandName, controls.LegacyLogs)

	flags := cli.Flags{
		//  Backward compatibility with `terragrunt-` prefix flags.

		flags.NewFlag(&cli.GenericFlag[string]{
			Name:        ConfigFlagName,
			EnvVars:     tgPrefix.EnvVars(ConfigFlagName),
			Destination: &opts.TerragruntConfigPath,
			Usage:       "The path to the Terragrunt config file. Default is terragrunt.hcl.",
		},
			flags.WithDeprecatedNames(terragruntPrefix.FlagNames(DeprecatedConfigFlagName), terragruntPrefixControl)),

		NewTFPathFlag(opts, prefix),

		flags.NewFlag(&cli.BoolFlag{
			Name:        NoAutoInitFlagName,
			EnvVars:     tgPrefix.EnvVars(NoAutoInitFlagName),
			Usage:       "Don't automatically run 'terraform/tofu init' during other terragrunt commands. You must run 'terragrunt init' manually.",
			Negative:    true,
			Destination: &opts.AutoInit,
		},
			flags.WithDeprecatedNames(terragruntPrefix.FlagNames(DeprecatedNoAutoInitFlagName), terragruntPrefixControl),
			flags.WithDeprecatedFlag(&cli.BoolFlag{
				EnvVars: terragruntPrefix.EnvVars(DeprecatedAutoInitFlagName),
			}, nil, terragruntPrefixControl)),

		flags.NewFlag(&cli.BoolFlag{
			Name:        NoAutoRetryFlagName,
			EnvVars:     tgPrefix.EnvVars(NoAutoRetryFlagName),
			Destination: &opts.AutoRetry,
			Usage:       "Don't automatically re-run command in case of transient errors.",
			Negative:    true,
		},
			flags.WithDeprecatedNames(terragruntPrefix.FlagNames(DeprecatedNoAutoRetryFlagName), terragruntPrefixControl),
			flags.WithDeprecatedFlag(&cli.BoolFlag{
				EnvVars: terragruntPrefix.EnvVars(DeprecatedAutoRetryFlagName),
			}, nil, terragruntPrefixControl)),

		flags.NewFlag(&cli.BoolFlag{
			Name:        NoAutoApproveFlagName,
			EnvVars:     tgPrefix.EnvVars(NoAutoApproveFlagName),
			Destination: &opts.RunAllAutoApprove,
			Usage:       "Don't automatically append '-auto-approve' to the underlying OpenTofu/Terraform commands run with 'run-all'.",
			Negative:    true,
		},
			flags.WithDeprecatedNames(terragruntPrefix.FlagNames(DeprecatedNoAutoApproveFlagName), terragruntPrefixControl),
			flags.WithDeprecatedFlag(&cli.BoolFlag{
				EnvVars: terragruntPrefix.EnvVars(DeprecatedAutoApproveFlagName),
			}, nil, terragruntPrefixControl)),

		flags.NewFlag(&cli.GenericFlag[string]{
			Name:        DownloadDirFlagName,
			EnvVars:     tgPrefix.EnvVars(DownloadDirFlagName),
			Destination: &opts.DownloadDir,
			Usage:       "The path to download OpenTofu/Terraform modules into. Default is .terragrunt-cache in the working directory.",
		}, flags.WithDeprecatedNamesEnvVars(
			terragruntPrefix.FlagNames(DeprecatedDownloadDirFlagName),
			terragruntPrefix.EnvVars(DeprecatedDownloadEnvVar),
			terragruntPrefixControl)),

		flags.NewFlag(&cli.GenericFlag[string]{
			Name:        SourceFlagName,
			EnvVars:     tgPrefix.EnvVars(SourceFlagName),
			Destination: &opts.Source,
			Usage:       "Download OpenTofu/Terraform configurations from the specified source into a temporary folder, and run Terraform in that temporary folder.",
		},
			flags.WithDeprecatedNames(terragruntPrefix.FlagNames(DeprecatedSourceFlagName), terragruntPrefixControl)),

		flags.NewFlag(&cli.BoolFlag{
			Name:        SourceUpdateFlagName,
			EnvVars:     tgPrefix.EnvVars(SourceUpdateFlagName),
			Destination: &opts.SourceUpdate,
			Usage:       "Delete the contents of the temporary folder to clear out any old, cached source code before downloading new source code into it.",
		},
			flags.WithDeprecatedNames(terragruntPrefix.FlagNames(DeprecatedSourceUpdateFlagName), terragruntPrefixControl)),

		flags.NewFlag(&cli.MapFlag[string, string]{
			Name:        SourceMapFlagName,
			EnvVars:     tgPrefix.EnvVars(SourceMapFlagName),
			Destination: &opts.SourceMap,
			Usage:       "Replace any source URL (including the source URL of a config pulled in with dependency blocks) that has root source with dest.",
			Splitter:    util.SplitUrls,
		},
			flags.WithDeprecatedNames(terragruntPrefix.FlagNames(DeprecatedSourceMapFlagName), terragruntPrefixControl)),

		// Assume IAM Role flags.

		flags.NewFlag(&cli.GenericFlag[string]{
			Name:        IAMAssumeRoleFlagName,
			EnvVars:     tgPrefix.EnvVars(IAMAssumeRoleFlagName),
			Destination: &opts.IAMRoleOptions.RoleARN,
			Usage:       "Assume the specified IAM role before executing OpenTofu/Terraform.",
		},
			flags.WithDeprecatedNames(terragruntPrefix.FlagNames(DeprecatedIAMRoleFlagName), terragruntPrefixControl)),

		flags.NewFlag(&cli.GenericFlag[int64]{
			Name:        IAMAssumeRoleDurationFlagName,
			EnvVars:     tgPrefix.EnvVars(IAMAssumeRoleDurationFlagName),
			Destination: &opts.IAMRoleOptions.AssumeRoleDuration,
			Usage:       "Session duration for IAM Assume Role session.",
		},
			flags.WithDeprecatedNames(terragruntPrefix.FlagNames(DeprecatedIAMAssumeRoleDurationFlagName), terragruntPrefixControl)),

		flags.NewFlag(&cli.GenericFlag[string]{
			Name:        IAMAssumeRoleSessionNameFlagName,
			EnvVars:     tgPrefix.EnvVars(IAMAssumeRoleSessionNameFlagName),
			Destination: &opts.IAMRoleOptions.AssumeRoleSessionName,
			Usage:       "Name for the IAM Assumed Role session.",
		},
			flags.WithDeprecatedNames(terragruntPrefix.FlagNames(DeprecatedIAMAssumeRoleSessionNameFlagName), terragruntPrefixControl)),

		flags.NewFlag(&cli.GenericFlag[string]{
			Name:        IAMAssumeRoleWebIdentityTokenFlagName,
			EnvVars:     tgPrefix.EnvVars(IAMAssumeRoleWebIdentityTokenFlagName),
			Destination: &opts.IAMRoleOptions.WebIdentityToken,
			Usage:       "For AssumeRoleWithWebIdentity, the WebIdentity token.",
		},
			flags.WithDeprecatedNamesEnvVars(
				terragruntPrefix.FlagNames(DeprecatedIAMWebIdentityTokenFlagName),
				terragruntPrefix.EnvVars(DeprecatedIAMAssumeRoleWebIdentityTokenFlagName),
				terragruntPrefixControl)),

		flags.NewFlag(&cli.BoolFlag{
			Name:        QueueIgnoreErrorsFlagName,
			EnvVars:     tgPrefix.EnvVars(QueueIgnoreErrorsFlagName),
			Destination: &opts.IgnoreDependencyErrors,
			Usage:       "Continue processing Units even if a dependency fails.",
		},
			flags.WithDeprecatedNames(terragruntPrefix.FlagNames(DeprecatedIgnoreDependencyErrorsFlagName), terragruntPrefixControl)),

		flags.NewFlag(&cli.BoolFlag{
			Name:        QueueIgnoreDAGOrderFlagName,
			EnvVars:     tgPrefix.EnvVars(QueueIgnoreDAGOrderFlagName),
			Destination: &opts.IgnoreDependencyOrder,
			Usage:       "Ignore DAG order for --all commands.",
		},
			flags.WithDeprecatedNames(terragruntPrefix.FlagNames(DeprecatedIgnoreDependencyOrderFlagName), terragruntPrefixControl)),

		flags.NewFlag(&cli.BoolFlag{
			Name:        QueueExcludeExternalFlagName,
			EnvVars:     tgPrefix.EnvVars(QueueExcludeExternalFlagName),
			Destination: &opts.IgnoreExternalDependencies,
			Usage:       "Ignore external dependencies for --all commands.",
		},
			flags.WithDeprecatedNames(terragruntPrefix.FlagNames(DeprecatedIgnoreExternalDependenciesFlagName), terragruntPrefixControl)),

		flags.NewFlag(&cli.BoolFlag{
			Name:        QueueIncludeExternalFlagName,
			EnvVars:     tgPrefix.EnvVars(QueueIncludeExternalFlagName),
			Destination: &opts.IncludeExternalDependencies,
			Usage:       "Include external dependencies for --all commands without asking.",
		},
			flags.WithDeprecatedNames(terragruntPrefix.FlagNames(DeprecatedIncludeExternalDependenciesFlagName), terragruntPrefixControl)),

		flags.NewFlag(&cli.GenericFlag[int]{
			Name:        ParallelismFlagName,
			EnvVars:     tgPrefix.EnvVars(ParallelismFlagName),
			Destination: &opts.Parallelism,
			Usage:       "Parallelism for --all commands.",
		},
			flags.WithDeprecatedNames(terragruntPrefix.FlagNames(DeprecatedParallelismFlagName), terragruntPrefixControl)),

		flags.NewFlag(&cli.GenericFlag[string]{
			Name:        QueueExcludesFileFlagName,
			EnvVars:     tgPrefix.EnvVars(QueueExcludesFileFlagName),
			Destination: &opts.ExcludesFile,
			Usage:       "Path to a file with a list of directories that need to be excluded when running *-all commands.",
		},
			flags.WithDeprecatedNames(terragruntPrefix.FlagNames(DeprecatedExcludesFileFlagName), terragruntPrefixControl)),

		flags.NewFlag(&cli.SliceFlag[string]{
			Name:        QueueExcludeDirFlagName,
			EnvVars:     tgPrefix.EnvVars(QueueExcludeDirFlagName),
			Destination: &opts.ExcludeDirs,
			Usage:       "Unix-style glob of directories to exclude from the queue of Units to run.",
		},
			flags.WithDeprecatedNames(terragruntPrefix.FlagNames(DeprecatedExcludeDirFlagName), terragruntPrefixControl)),

		flags.NewFlag(&cli.SliceFlag[string]{
			Name:        QueueIncludeDirFlagName,
			EnvVars:     tgPrefix.EnvVars(QueueIncludeDirFlagName),
			Destination: &opts.IncludeDirs,
			Usage:       "Unix-style glob of directories to include from the queue of Units to run.",
		},
			flags.WithDeprecatedNames(terragruntPrefix.FlagNames(DeprecatedIncludeDirFlagName), terragruntPrefixControl)),

		flags.NewFlag(&cli.BoolFlag{
			Name:        InputsDebugFlagName,
			EnvVars:     tgPrefix.EnvVars(InputsDebugFlagName),
			Destination: &opts.Debug,
			Usage:       "Write debug.tfvars to working folder to help root-cause issues.",
		},
			flags.WithDeprecatedNames(terragruntPrefix.FlagNames(DeprecatedDebugFlagName), terragruntPrefixControl)),

		flags.NewFlag(&cli.BoolFlag{
			Name:        UsePartialParseConfigCacheFlagName,
			EnvVars:     tgPrefix.EnvVars(UsePartialParseConfigCacheFlagName),
			Destination: &opts.UsePartialParseConfigCache,
			Usage:       "Enables caching of includes during partial parsing operations. Will also be used for the --iam-role option if provided.",
		},
			flags.WithDeprecatedNames(terragruntPrefix.FlagNames(DeprecatedUsePartialParseConfigCacheFlagName), terragruntPrefixControl)),

		flags.NewFlag(&cli.BoolFlag{
			Name:        DependencyFetchOutputFromStateFlagName,
			EnvVars:     tgPrefix.EnvVars(DependencyFetchOutputFromStateFlagName),
			Destination: &opts.FetchDependencyOutputFromState,
			Usage:       "The option fetches dependency output directly from the state file instead of using tofu/terraform output.",
		},
			flags.WithDeprecatedNames(terragruntPrefix.FlagNames(DeprecatedFetchDependencyOutputFromStateFlagName), terragruntPrefixControl)),

		flags.NewFlag(&cli.BoolFlag{
			Name:        TFForwardStdoutFlagName,
			EnvVars:     tgPrefix.EnvVars(TFForwardStdoutFlagName),
			Destination: &opts.ForwardTFStdout,
			Usage:       "If specified, the output of OpenTofu/Terraform commands will be printed as is, without being integrated into the Terragrunt log.",
		},
			flags.WithDeprecatedNames(terragruntPrefix.FlagNames(DeprecatedForwardTFStdoutFlagName), terragruntPrefixControl),
			flags.WithDeprecatedFlag(&cli.BoolFlag{
				Name:    terragruntPrefix.FlagName(DeprecatedIncludeModulePrefixFlagName),
				EnvVars: terragruntPrefix.EnvVars(DeprecatedIncludeModulePrefixFlagName),
				Usage:   "When this flag is set output from Terraform sub-commands is prefixed with module path.",
				Action: func(_ *cli.Context, _ bool) error {
					opts.Logger.Warnf("The %q flag is deprecated. Use the functionality-inverted %q flag instead. By default, Terraform/OpenTofu output is integrated into the Terragrunt log, which prepends additional data, such as timestamps and prefixes, to log entries.", DeprecatedIncludeModulePrefixFlagName, TFForwardStdoutFlagName)

					return nil
				},
			}, flags.NewValue(strconv.FormatBool(false)), legacyLogsControl)),

		flags.NewFlag(&cli.BoolFlag{
			Name:        QueueStrictIncludeFlagName,
			EnvVars:     tgPrefix.EnvVars(QueueStrictIncludeFlagName),
			Destination: &opts.StrictInclude,
			Usage:       "If flag is set, only modules under the directories passed in with '--include-dir' will be included.",
		},
			flags.WithDeprecatedNames(terragruntPrefix.FlagNames(DeprecatedStrictIncludeFlagName), terragruntPrefixControl)),

		flags.NewFlag(&cli.SliceFlag[string]{
			Name:        UnitsThatIncludeFlagName,
			EnvVars:     tgPrefix.EnvVars(UnitsThatIncludeFlagName),
			Destination: &opts.ModulesThatInclude,
			Usage:       "If flag is set, 'run-all' will only run the command against Terragrunt modules that include the specified file.",
		},
			flags.WithDeprecatedNames(terragruntPrefix.FlagNames(DeprecatedModulesThatIncludeFlagName), terragruntPrefixControl)),

		flags.NewFlag(&cli.SliceFlag[string]{
			Name:        QueueIncludeUnitsReadingFlagName,
			EnvVars:     tgPrefix.EnvVars(QueueIncludeUnitsReadingFlagName),
			Destination: &opts.UnitsReading,
			Usage:       "If flag is set, 'run-all' will only run the command against Terragrunt units that read the specified file via an HCL function or include.",
		},
			flags.WithDeprecatedNames(terragruntPrefix.FlagNames(DeprecatedUnitsReadingFlagName), terragruntPrefixControl)),

		flags.NewFlag(&cli.BoolFlag{
			Name:        BackendRequireBootstrapFlagName,
			EnvVars:     tgPrefix.EnvVars(BackendRequireBootstrapFlagName),
			Destination: &opts.FailIfBucketCreationRequired,
			Usage:       "When this flag is set Terragrunt will fail if the remote state bucket needs to be created.",
		},
			flags.WithDeprecatedNames(terragruntPrefix.FlagNames(DeprecatedFailOnStateBucketCreationFlagName), terragruntPrefixControl)),

		flags.NewFlag(&cli.BoolFlag{
			Name:        DisableBucketUpdateFlagName,
			EnvVars:     tgPrefix.EnvVars(DisableBucketUpdateFlagName),
			Destination: &opts.DisableBucketUpdate,
			Usage:       "When this flag is set Terragrunt will not update the remote state bucket.",
		},
			flags.WithDeprecatedNames(terragruntPrefix.FlagNames(DeprecatedDisableBucketUpdateFlagName), terragruntPrefixControl)),

		flags.NewFlag(&cli.BoolFlag{
			Name:        DisableCommandValidationFlagName,
			EnvVars:     tgPrefix.EnvVars(DisableCommandValidationFlagName),
			Destination: &opts.DisableCommandValidation,
			Usage:       "When this flag is set, Terragrunt will not validate the tofu/terraform command.",
		},
			flags.WithDeprecatedNames(terragruntPrefix.FlagNames(DeprecatedDisableCommandValidationFlagName), terragruntPrefixControl)),

		flags.NewFlag(&cli.BoolFlag{
			Name:        NoDestroyDependenciesCheckFlagName,
			EnvVars:     tgPrefix.EnvVars(NoDestroyDependenciesCheckFlagName),
			Destination: &opts.NoDestroyDependenciesCheck,
			Usage:       "When this flag is set, Terragrunt will not check for dependent units when destroying.",
		},
			flags.WithDeprecatedNames(terragruntPrefix.FlagNames(DeprecatedNoDestroyDependenciesCheckFlagName), terragruntPrefixControl)),

		// Terragrunt Provider Cache flags
		flags.NewFlag(&cli.BoolFlag{
			Name:        ProviderCacheFlagName,
			EnvVars:     tgPrefix.EnvVars(ProviderCacheFlagName),
			Destination: &opts.ProviderCache,
			Usage:       "Enables Terragrunt's provider caching.",
		},
			flags.WithDeprecatedNames(terragruntPrefix.FlagNames(DeprecatedProviderCacheFlagName), terragruntPrefixControl)),

		flags.NewFlag(&cli.GenericFlag[string]{
			Name:        ProviderCacheDirFlagName,
			EnvVars:     tgPrefix.EnvVars(ProviderCacheDirFlagName),
			Destination: &opts.ProviderCacheDir,
			Usage:       "The path to the Terragrunt provider cache directory. By default, 'terragrunt/providers' folder in the user cache directory.",
		},
			flags.WithDeprecatedNames(terragruntPrefix.FlagNames(DeprecatedProviderCacheDirFlagName), terragruntPrefixControl)),

		flags.NewFlag(&cli.GenericFlag[string]{
			Name:        ProviderCacheTokenFlagName,
			EnvVars:     tgPrefix.EnvVars(ProviderCacheTokenFlagName),
			Destination: &opts.ProviderCacheToken,
			Usage:       "The token for authentication to the Terragrunt Provider Cache server. By default, assigned automatically.",
		},
			flags.WithDeprecatedNames(terragruntPrefix.FlagNames(DeprecatedProviderCacheTokenFlagName), terragruntPrefixControl)),

		flags.NewFlag(&cli.GenericFlag[string]{
			Name:        ProviderCacheHostnameFlagName,
			EnvVars:     tgPrefix.EnvVars(ProviderCacheHostnameFlagName),
			Destination: &opts.ProviderCacheHostname,
			Usage:       "The hostname of the Terragrunt Provider Cache server. By default, 'localhost'.",
		},
			flags.WithDeprecatedNames(terragruntPrefix.FlagNames(DeprecatedProviderCacheHostnameFlagName), terragruntPrefixControl)),

		flags.NewFlag(&cli.GenericFlag[int]{
			Name:        ProviderCachePortFlagName,
			EnvVars:     tgPrefix.EnvVars(ProviderCachePortFlagName),
			Destination: &opts.ProviderCachePort,
			Usage:       "The port of the Terragrunt Provider Cache server. By default, assigned automatically.",
		},
			flags.WithDeprecatedNames(terragruntPrefix.FlagNames(DeprecatedProviderCachePortFlagName), terragruntPrefixControl)),

		flags.NewFlag(&cli.SliceFlag[string]{
			Name:        ProviderCacheRegistryNamesFlagName,
			EnvVars:     tgPrefix.EnvVars(ProviderCacheRegistryNamesFlagName),
			Destination: &opts.ProviderCacheRegistryNames,
			Usage:       "The list of remote registries to cached by Terragrunt Provider Cache server. By default, 'registry.terraform.io', 'registry.opentofu.org'.",
		},
			flags.WithDeprecatedNames(terragruntPrefix.FlagNames(DeprecatedProviderCacheRegistryNamesFlagName), terragruntPrefixControl)),

		flags.NewFlag(&cli.GenericFlag[string]{
			Name:        AuthProviderCmdFlagName,
			EnvVars:     tgPrefix.EnvVars(AuthProviderCmdFlagName),
			Destination: &opts.AuthProviderCmd,
			Usage:       "Run the provided command and arguments to authenticate Terragrunt dynamically when necessary.",
		},
			flags.WithDeprecatedNames(terragruntPrefix.FlagNames(DeprecatedAuthProviderCmdFlagName), terragruntPrefixControl)),

		flags.NewFlag(&cli.MapFlag[string, string]{
			Name:     FeatureFlagName,
			EnvVars:  tgPrefix.EnvVars(FeatureFlagName),
			Usage:    "Set feature flags for the HCL code.",
			Splitter: util.SplitComma,
			Action: func(_ *cli.Context, value map[string]string) error {
				for key, val := range value {
					opts.FeatureFlags.Store(key, val)
				}

				return nil
			},
		},
			flags.WithDeprecatedNames(terragruntPrefix.FlagNames(DeprecatedFeatureFlagName), terragruntPrefixControl)),

		// Terragrunt engine flags.

		flags.NewFlag(&cli.BoolFlag{
			Name:        EngineEnableFlagName,
			EnvVars:     tgPrefix.EnvVars(EngineEnableFlagName),
			Destination: &opts.EngineEnabled,
			Usage:       "Enable Terragrunt experimental engine.",
			Hidden:      true,
		},
			flags.WithDeprecatedNames(terragruntPrefix.FlagNames(DeprecatedEngineEnableFlagName), terragruntPrefixControl)),

		flags.NewFlag(&cli.GenericFlag[string]{
			Name:        EngineCachePathFlagName,
			EnvVars:     tgPrefix.EnvVars(EngineCachePathFlagName),
			Destination: &opts.EngineCachePath,
			Usage:       "Cache path for Terragrunt engine files.",
			Hidden:      true,
		},
			flags.WithDeprecatedNames(terragruntPrefix.FlagNames(DeprecatedEngineCachePathFlagName), terragruntPrefixControl)),

		flags.NewFlag(&cli.BoolFlag{
			Name:        EngineSkipCheckFlagName,
			EnvVars:     tgPrefix.EnvVars(EngineSkipCheckFlagName),
			Destination: &opts.EngineSkipChecksumCheck,
			Usage:       "Skip checksum check for Terragrunt engine files.",
			Hidden:      true,
		},
			flags.WithDeprecatedNames(terragruntPrefix.FlagNames(DeprecatedEngineSkipCheckFlagName), terragruntPrefixControl)),

		flags.NewFlag(&cli.GenericFlag[string]{
			Name:        EngineLogLevelFlagName,
			EnvVars:     tgPrefix.EnvVars(EngineLogLevelFlagName),
			Destination: &opts.EngineLogLevel,
			Usage:       "Terragrunt engine log level.",
			Hidden:      true,
		},
			flags.WithDeprecatedNames(terragruntPrefix.FlagNames(DeprecatedEngineLogLevelFlagName), terragruntPrefixControl)),
	}

	return flags.Sort()
}

// NewTFPathFlag creates a flag for specifying the OpenTofu/Terraform binary path.
func NewTFPathFlag(opts *options.TerragruntOptions, prefix flags.Prefix) *flags.Flag {
	tgPrefix := prefix.Prepend(flags.TgPrefix)
	terragruntPrefix := prefix.Prepend(flags.TerragruntPrefix)
	terragruntPrefixControl := flags.StrictControlsByGlobalFlags(opts.StrictControls)

	return flags.NewFlag(&cli.GenericFlag[string]{
		Name:        TFPathFlagName,
		EnvVars:     tgPrefix.EnvVars(TFPathFlagName),
		Destination: &opts.TerraformPath,
		Usage:       "Path to the OpenTofu/Terraform binary. Default is tofu (on PATH).",
	},
		flags.WithDeprecatedNames(terragruntPrefix.FlagNames(DeprecatedTfpathFlagName), terragruntPrefixControl))
}
