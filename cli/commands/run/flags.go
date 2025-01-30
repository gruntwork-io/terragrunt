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

	FailOnStateBucketCreationFlagName      = "fail-on-state-bucket-creation"      // `backend-require-bootstrap`
	ModulesThatIncludeFlagName             = "modules-that-include"               // `units-that-include`
	ForwardTFStdoutFlagName                = "forward-tf-stdout"                  // `tf-forward-stdout`.
	TfpathFlagName                         = "tfpath"                             // `tf-path`.
	DownloadEnvVar                         = "download"                           // `download-dir` for old `TERRAGRUNT_DOWNLOAD` env var.
	IAMRoleFlagName                        = "iam-role"                           // `iam-assume-role`.
	IAMWebIdentityTokenFlagName            = "iam-web-identity-token"             // `iam-assume-role-web-identity-token`.
	DebugFlagName                          = "debug"                              // `inputs-debug`.
	FetchDependencyOutputFromStateFlagName = "fetch-dependency-output-from-state" // `dependency-fetch-output-from-state`.
	IgnoreDependencyOrderFlagName          = "ignore-dependency-order"            // `queue-ignore-dag-order`.
	IgnoreExternalDependenciesFlagName     = "ignore-external-dependencies"       // `queue-exclude-external`.
	ExcludeDirFlagName                     = "exclude-dir"                        // `queue-exclude-dir`.
	ExcludesFileFlagName                   = "excludes-file"                      // `queue-excludes-file`.
	IncludeDirFlagName                     = "include-dir"                        // `queue-include-dir`.
	IncludeExternalDependenciesFlagName    = "include-external-dependencies"      // `queue-include-external`.
	StrictIncludeFlagName                  = "strict-include"                     // `queue-strict-include`.
	UnitsReadingFlagName                   = "queue-include-units-reading"        // `queue-include-units-reading`.
	IgnoreDependencyErrorsFlagName         = "ignore-dependency-errors"           // `queue-ignore-errors`.

	// Deprecated flags.

	IncludeModulePrefixFlagName = "include-module-prefix"
)

// NewFlags creates and returns global flags.
func NewFlags(opts *options.TerragruntOptions, prefix flags.Prefix) cli.Flags {
	tgPrefix := prefix.Prepend(flags.TgPrefix)
	terragruntPrefix := prefix.Prepend(flags.TerragruntPrefix)
	cliRedesignControl := flags.StrictControlsByGroup(opts.StrictControls, CommandName, controls.CLIRedesign)
	legacyLogsControl := flags.StrictControlsByGroup(opts.StrictControls, CommandName, controls.LegacyLogs)

	flags := cli.Flags{
		flags.NewFlag(&cli.BoolFlag{
			Name:    AllFlagName,
			EnvVars: tgPrefix.EnvVars(AllFlagName),
			Usage:   `Run the specified OpenTofu/Terraform command on the stack of units in the current directory.`,
		}),

		flags.NewFlag(&cli.BoolFlag{
			Name:    GraphFlagName,
			EnvVars: tgPrefix.EnvVars(GraphFlagName),
			Usage:   "Run the specified OpenTofu/Terraform command following the Directed Acyclic Graph (DAG) of dependencies.",
		}),

		//  Backward compatibility with `terragrunt-` prefix flags.

		flags.NewFlag(&cli.GenericFlag[string]{
			Name:        ConfigFlagName,
			EnvVars:     tgPrefix.EnvVars(ConfigFlagName),
			Destination: &opts.TerragruntConfigPath,
			Usage:       "The path to the Terragrunt config file. Default is terragrunt.hcl.",
		},
			flags.WithDeprecatedPrefix(terragruntPrefix, cliRedesignControl)),

		flags.NewFlag(&cli.GenericFlag[string]{
			Name:        TFPathFlagName,
			EnvVars:     tgPrefix.EnvVars(TFPathFlagName),
			Destination: &opts.TerraformPath,
			Usage:       "Path to the OpenTofu/Terraform binary. Default is tofu (on PATH).",
		},
			flags.WithDeprecatedNames(terragruntPrefix.FlagNames(TfpathFlagName), cliRedesignControl)),

		flags.NewFlag(&cli.BoolFlag{
			Name:        NoAutoInitFlagName,
			EnvVars:     tgPrefix.EnvVars(NoAutoInitFlagName),
			Usage:       "Don't automatically run 'terraform/tofu init' during other terragrunt commands. You must run 'terragrunt init' manually.",
			Negative:    true,
			Destination: &opts.AutoInit,
		},
			flags.WithDeprecatedPrefix(terragruntPrefix, cliRedesignControl)),

		flags.NewFlag(&cli.BoolFlag{
			Name:        NoAutoRetryFlagName,
			EnvVars:     tgPrefix.EnvVars(NoAutoRetryFlagName),
			Destination: &opts.AutoRetry,
			Usage:       "Don't automatically re-run command in case of transient errors.",
			Negative:    true,
		},
			flags.WithDeprecatedPrefix(terragruntPrefix, cliRedesignControl)),

		flags.NewFlag(&cli.BoolFlag{
			Name:        NoAutoApproveFlagName,
			EnvVars:     tgPrefix.EnvVars(NoAutoApproveFlagName),
			Destination: &opts.RunAllAutoApprove,
			Usage:       "Don't automatically append '-auto-approve' to the underlying OpenTofu/Terraform commands run with 'run-all'.",
			Negative:    true,
		},
			flags.WithDeprecatedPrefix(terragruntPrefix, cliRedesignControl)),

		flags.NewFlag(&cli.GenericFlag[string]{
			Name:        DownloadDirFlagName,
			EnvVars:     tgPrefix.EnvVars(DownloadDirFlagName),
			Destination: &opts.DownloadDir,
			Usage:       "The path to download OpenTofu/Terraform modules into. Default is .terragrunt-cache in the working directory.",
		}, flags.WithDeprecatedNamesEnvVars(
			terragruntPrefix.FlagNames(DownloadDirFlagName),
			terragruntPrefix.EnvVars(DownloadEnvVar),
			cliRedesignControl)),

		flags.NewFlag(&cli.GenericFlag[string]{
			Name:        SourceFlagName,
			EnvVars:     tgPrefix.EnvVars(SourceFlagName),
			Destination: &opts.Source,
			Usage:       "Download OpenTofu/Terraform configurations from the specified source into a temporary folder, and run Terraform in that temporary folder.",
		},
			flags.WithDeprecatedPrefix(terragruntPrefix, cliRedesignControl)),

		flags.NewFlag(&cli.BoolFlag{
			Name:        SourceUpdateFlagName,
			EnvVars:     tgPrefix.EnvVars(SourceUpdateFlagName),
			Destination: &opts.SourceUpdate,
			Usage:       "Delete the contents of the temporary folder to clear out any old, cached source code before downloading new source code into it.",
		},
			flags.WithDeprecatedPrefix(terragruntPrefix, cliRedesignControl)),

		flags.NewFlag(&cli.MapFlag[string, string]{
			Name:        SourceMapFlagName,
			EnvVars:     tgPrefix.EnvVars(SourceMapFlagName),
			Destination: &opts.SourceMap,
			Usage:       "Replace any source URL (including the source URL of a config pulled in with dependency blocks) that has root source with dest.",
			Splitter:    util.SplitUrls,
		},
			flags.WithDeprecatedPrefix(terragruntPrefix, cliRedesignControl)),

		// Assume IAM Role flags.

		flags.NewFlag(&cli.GenericFlag[string]{
			Name:        IAMAssumeRoleFlagName,
			EnvVars:     tgPrefix.EnvVars(IAMAssumeRoleFlagName),
			Destination: &opts.IAMRoleOptions.RoleARN,
			Usage:       "Assume the specified IAM role before executing OpenTofu/Terraform.",
		},
			flags.WithDeprecatedNames(terragruntPrefix.FlagNames(IAMRoleFlagName), cliRedesignControl)),

		flags.NewFlag(&cli.GenericFlag[int64]{
			Name:        IAMAssumeRoleDurationFlagName,
			EnvVars:     tgPrefix.EnvVars(IAMAssumeRoleDurationFlagName),
			Destination: &opts.IAMRoleOptions.AssumeRoleDuration,
			Usage:       "Session duration for IAM Assume Role session.",
		},
			flags.WithDeprecatedPrefix(terragruntPrefix, cliRedesignControl)),

		flags.NewFlag(&cli.GenericFlag[string]{
			Name:        IAMAssumeRoleSessionNameFlagName,
			EnvVars:     tgPrefix.EnvVars(IAMAssumeRoleSessionNameFlagName),
			Destination: &opts.IAMRoleOptions.AssumeRoleSessionName,
			Usage:       "Name for the IAM Assumed Role session.",
		},
			flags.WithDeprecatedPrefix(terragruntPrefix, cliRedesignControl)),

		flags.NewFlag(&cli.GenericFlag[string]{
			Name:        IAMAssumeRoleWebIdentityTokenFlagName,
			EnvVars:     tgPrefix.EnvVars(IAMAssumeRoleWebIdentityTokenFlagName),
			Destination: &opts.IAMRoleOptions.WebIdentityToken,
			Usage:       "For AssumeRoleWithWebIdentity, the WebIdentity token.",
		},
			flags.WithDeprecatedNamesEnvVars(
				terragruntPrefix.FlagNames(IAMWebIdentityTokenFlagName),
				terragruntPrefix.EnvVars(IAMAssumeRoleWebIdentityTokenFlagName),
				cliRedesignControl)),

		flags.NewFlag(&cli.BoolFlag{
			Name:        QueueIgnoreErrorsFlagName,
			EnvVars:     tgPrefix.EnvVars(QueueIgnoreErrorsFlagName),
			Destination: &opts.IgnoreDependencyErrors,
			Usage:       "Continue processing Units even if a dependency fails.",
		},
			flags.WithDeprecatedNames(terragruntPrefix.FlagNames(IgnoreDependencyErrorsFlagName), cliRedesignControl)),

		flags.NewFlag(&cli.BoolFlag{
			Name:        QueueIgnoreDAGOrderFlagName,
			EnvVars:     tgPrefix.EnvVars(QueueIgnoreDAGOrderFlagName),
			Destination: &opts.IgnoreDependencyOrder,
			Usage:       "Ignore DAG order for --all commands.",
		},
			flags.WithDeprecatedNames(terragruntPrefix.FlagNames(IgnoreDependencyOrderFlagName), cliRedesignControl)),

		flags.NewFlag(&cli.BoolFlag{
			Name:        QueueExcludeExternalFlagName,
			EnvVars:     tgPrefix.EnvVars(QueueExcludeExternalFlagName),
			Destination: &opts.IgnoreExternalDependencies,
			Usage:       "Ignore external dependencies for --all commands.",
		},
			flags.WithDeprecatedNames(terragruntPrefix.FlagNames(IgnoreExternalDependenciesFlagName), cliRedesignControl)),

		flags.NewFlag(&cli.BoolFlag{
			Name:        QueueIncludeExternalFlagName,
			EnvVars:     tgPrefix.EnvVars(QueueIncludeExternalFlagName),
			Destination: &opts.IncludeExternalDependencies,
			Usage:       "Include external dependencies for --all commands without asking.",
		},
			flags.WithDeprecatedNames(terragruntPrefix.FlagNames(IncludeExternalDependenciesFlagName), cliRedesignControl)),

		flags.NewFlag(&cli.GenericFlag[int]{
			Name:        ParallelismFlagName,
			EnvVars:     tgPrefix.EnvVars(ParallelismFlagName),
			Destination: &opts.Parallelism,
			Usage:       "Parallelism for --all commands.",
		},
			flags.WithDeprecatedPrefix(terragruntPrefix, cliRedesignControl)),

		flags.NewFlag(&cli.GenericFlag[string]{
			Name:        QueueExcludesFileFlagName,
			EnvVars:     tgPrefix.EnvVars(QueueExcludesFileFlagName),
			Destination: &opts.ExcludesFile,
			Usage:       "Path to a file with a list of directories that need to be excluded when running *-all commands.",
		},
			flags.WithDeprecatedNames(terragruntPrefix.FlagNames(ExcludesFileFlagName), cliRedesignControl)),

		flags.NewFlag(&cli.SliceFlag[string]{
			Name:        QueueExcludeDirFlagName,
			EnvVars:     tgPrefix.EnvVars(QueueExcludeDirFlagName),
			Destination: &opts.ExcludeDirs,
			Usage:       "Unix-style glob of directories to exclude from the queue of Units to run.",
		},
			flags.WithDeprecatedNames(terragruntPrefix.FlagNames(ExcludeDirFlagName), cliRedesignControl)),

		flags.NewFlag(&cli.SliceFlag[string]{
			Name:        QueueIncludeDirFlagName,
			EnvVars:     tgPrefix.EnvVars(QueueIncludeDirFlagName),
			Destination: &opts.IncludeDirs,
			Usage:       "Unix-style glob of directories to include from the queue of Units to run.",
		},
			flags.WithDeprecatedNames(terragruntPrefix.FlagNames(IncludeDirFlagName), cliRedesignControl)),

		flags.NewFlag(&cli.BoolFlag{
			Name:        InputsDebugFlagName,
			EnvVars:     tgPrefix.EnvVars(InputsDebugFlagName),
			Destination: &opts.Debug,
			Usage:       "Write debug.tfvars to working folder to help root-cause issues.",
		},
			flags.WithDeprecatedNames(terragruntPrefix.FlagNames(DebugFlagName), cliRedesignControl)),

		flags.NewFlag(&cli.BoolFlag{
			Name:        UsePartialParseConfigCacheFlagName,
			EnvVars:     tgPrefix.EnvVars(UsePartialParseConfigCacheFlagName),
			Destination: &opts.UsePartialParseConfigCache,
			Usage:       "Enables caching of includes during partial parsing operations. Will also be used for the --iam-role option if provided.",
		},
			flags.WithDeprecatedPrefix(terragruntPrefix, cliRedesignControl)),

		flags.NewFlag(&cli.BoolFlag{
			Name:        DependencyFetchOutputFromStateFlagName,
			EnvVars:     tgPrefix.EnvVars(DependencyFetchOutputFromStateFlagName),
			Destination: &opts.FetchDependencyOutputFromState,
			Usage:       "The option fetches dependency output directly from the state file instead of using tofu/terraform output.",
		},
			flags.WithDeprecatedNames(terragruntPrefix.FlagNames(FetchDependencyOutputFromStateFlagName), cliRedesignControl)),

		flags.NewFlag(&cli.BoolFlag{
			Name:        TFForwardStdoutFlagName,
			EnvVars:     tgPrefix.EnvVars(TFForwardStdoutFlagName),
			Destination: &opts.ForwardTFStdout,
			Usage:       "If specified, the output of OpenTofu/Terraform commands will be printed as is, without being integrated into the Terragrunt log.",
		},
			flags.WithDeprecatedNames(terragruntPrefix.FlagNames(ForwardTFStdoutFlagName), cliRedesignControl),
			flags.WithDeprecatedFlag(&cli.BoolFlag{
				Name:    terragruntPrefix.FlagName(IncludeModulePrefixFlagName),
				EnvVars: terragruntPrefix.EnvVars(IncludeModulePrefixFlagName),
				Usage:   "When this flag is set output from Terraform sub-commands is prefixed with module path.",
				Hidden:  true,
				Action: func(_ *cli.Context, _ bool) error {
					opts.Logger.Warnf("The %q flag is deprecated. Use the functionality-inverted %q flag instead. By default, Terraform/OpenTofu output is integrated into the Terragrunt log, which prepends additional data, such as timestamps and prefixes, to log entries.", IncludeModulePrefixFlagName, TFForwardStdoutFlagName)

					return nil
				},
			}, flags.NewValue(strconv.FormatBool(false)), legacyLogsControl)),

		flags.NewFlag(&cli.BoolFlag{
			Name:        QueueStrictIncludeFlagName,
			EnvVars:     tgPrefix.EnvVars(QueueStrictIncludeFlagName),
			Destination: &opts.StrictInclude,
			Usage:       "If flag is set, only modules under the directories passed in with '--include-dir' will be included.",
		},
			flags.WithDeprecatedNames(terragruntPrefix.FlagNames(StrictIncludeFlagName), cliRedesignControl)),

		flags.NewFlag(&cli.SliceFlag[string]{
			Name:        UnitsThatIncludeFlagName,
			EnvVars:     tgPrefix.EnvVars(UnitsThatIncludeFlagName),
			Destination: &opts.ModulesThatInclude,
			Usage:       "If flag is set, 'run-all' will only run the command against Terragrunt modules that include the specified file.",
		},
			flags.WithDeprecatedNames(terragruntPrefix.FlagNames(ModulesThatIncludeFlagName), cliRedesignControl)),

		flags.NewFlag(&cli.SliceFlag[string]{
			Name:        QueueIncludeUnitsReadingFlagName,
			EnvVars:     tgPrefix.EnvVars(QueueIncludeUnitsReadingFlagName),
			Destination: &opts.UnitsReading,
			Usage:       "If flag is set, 'run-all' will only run the command against Terragrunt units that read the specified file via an HCL function or include.",
		},
			flags.WithDeprecatedNames(terragruntPrefix.FlagNames(UnitsReadingFlagName), cliRedesignControl)),

		flags.NewFlag(&cli.BoolFlag{
			Name:        BackendRequireBootstrapFlagName,
			EnvVars:     tgPrefix.EnvVars(BackendRequireBootstrapFlagName),
			Destination: &opts.FailIfBucketCreationRequired,
			Usage:       "When this flag is set Terragrunt will fail if the remote state bucket needs to be created.",
		},
			flags.WithDeprecatedNames(terragruntPrefix.FlagNames(FailOnStateBucketCreationFlagName), cliRedesignControl)),

		flags.NewFlag(&cli.BoolFlag{
			Name:        DisableBucketUpdateFlagName,
			EnvVars:     tgPrefix.EnvVars(DisableBucketUpdateFlagName),
			Destination: &opts.DisableBucketUpdate,
			Usage:       "When this flag is set Terragrunt will not update the remote state bucket.",
		},
			flags.WithDeprecatedPrefix(terragruntPrefix, cliRedesignControl)),

		flags.NewFlag(&cli.BoolFlag{
			Name:        DisableCommandValidationFlagName,
			EnvVars:     tgPrefix.EnvVars(DisableCommandValidationFlagName),
			Destination: &opts.DisableCommandValidation,
			Usage:       "When this flag is set, Terragrunt will not validate the tofu/terraform command.",
		},
			flags.WithDeprecatedPrefix(terragruntPrefix, cliRedesignControl)),

		flags.NewFlag(&cli.BoolFlag{
			Name:        NoDestroyDependenciesCheckFlagName,
			EnvVars:     tgPrefix.EnvVars(NoDestroyDependenciesCheckFlagName),
			Destination: &opts.NoDestroyDependenciesCheck,
			Usage:       "When this flag is set, Terragrunt will not check for dependent units when destroying.",
		},
			flags.WithDeprecatedPrefix(terragruntPrefix, cliRedesignControl)),

		// Terragrunt Provider Cache flags
		flags.NewFlag(&cli.BoolFlag{
			Name:        ProviderCacheFlagName,
			EnvVars:     tgPrefix.EnvVars(ProviderCacheFlagName),
			Destination: &opts.ProviderCache,
			Usage:       "Enables Terragrunt's provider caching.",
		},
			flags.WithDeprecatedPrefix(terragruntPrefix, cliRedesignControl)),

		flags.NewFlag(&cli.GenericFlag[string]{
			Name:        ProviderCacheDirFlagName,
			EnvVars:     tgPrefix.EnvVars(ProviderCacheDirFlagName),
			Destination: &opts.ProviderCacheDir,
			Usage:       "The path to the Terragrunt provider cache directory. By default, 'terragrunt/providers' folder in the user cache directory.",
		},
			flags.WithDeprecatedPrefix(terragruntPrefix, cliRedesignControl)),

		flags.NewFlag(&cli.GenericFlag[string]{
			Name:        ProviderCacheTokenFlagName,
			EnvVars:     tgPrefix.EnvVars(ProviderCacheTokenFlagName),
			Destination: &opts.ProviderCacheToken,
			Usage:       "The token for authentication to the Terragrunt Provider Cache server. By default, assigned automatically.",
		},
			flags.WithDeprecatedPrefix(terragruntPrefix, cliRedesignControl)),

		flags.NewFlag(&cli.GenericFlag[string]{
			Name:        ProviderCacheHostnameFlagName,
			EnvVars:     tgPrefix.EnvVars(ProviderCacheHostnameFlagName),
			Destination: &opts.ProviderCacheHostname,
			Usage:       "The hostname of the Terragrunt Provider Cache server. By default, 'localhost'.",
		},
			flags.WithDeprecatedPrefix(terragruntPrefix, cliRedesignControl)),

		flags.NewFlag(&cli.GenericFlag[int]{
			Name:        ProviderCachePortFlagName,
			EnvVars:     tgPrefix.EnvVars(ProviderCachePortFlagName),
			Destination: &opts.ProviderCachePort,
			Usage:       "The port of the Terragrunt Provider Cache server. By default, assigned automatically.",
		},
			flags.WithDeprecatedPrefix(terragruntPrefix, cliRedesignControl)),

		flags.NewFlag(&cli.SliceFlag[string]{
			Name:        ProviderCacheRegistryNamesFlagName,
			EnvVars:     tgPrefix.EnvVars(ProviderCacheRegistryNamesFlagName),
			Destination: &opts.ProviderCacheRegistryNames,
			Usage:       "The list of remote registries to cached by Terragrunt Provider Cache server. By default, 'registry.terraform.io', 'registry.opentofu.org'.",
		},
			flags.WithDeprecatedPrefix(terragruntPrefix, cliRedesignControl)),

		flags.NewFlag(&cli.GenericFlag[string]{
			Name:        AuthProviderCmdFlagName,
			EnvVars:     tgPrefix.EnvVars(AuthProviderCmdFlagName),
			Destination: &opts.AuthProviderCmd,
			Usage:       "Run the provided command and arguments to authenticate Terragrunt dynamically when necessary.",
		},
			flags.WithDeprecatedPrefix(terragruntPrefix, cliRedesignControl)),

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
			flags.WithDeprecatedPrefix(terragruntPrefix, cliRedesignControl)),

		// Terragrunt engine flags.

		flags.NewFlag(&cli.BoolFlag{
			Name:        EngineEnableFlagName,
			EnvVars:     tgPrefix.EnvVars(EngineEnableFlagName),
			Destination: &opts.EngineEnabled,
			Usage:       "Enable Terragrunt experimental engine.",
			Hidden:      true,
		},
			flags.WithDeprecatedPrefix(terragruntPrefix, cliRedesignControl)),

		flags.NewFlag(&cli.GenericFlag[string]{
			Name:        EngineCachePathFlagName,
			EnvVars:     tgPrefix.EnvVars(EngineCachePathFlagName),
			Destination: &opts.EngineCachePath,
			Usage:       "Cache path for Terragrunt engine files.",
			Hidden:      true,
		},
			flags.WithDeprecatedPrefix(terragruntPrefix, cliRedesignControl)),

		flags.NewFlag(&cli.BoolFlag{
			Name:        EngineSkipCheckFlagName,
			EnvVars:     tgPrefix.EnvVars(EngineSkipCheckFlagName),
			Destination: &opts.EngineSkipChecksumCheck,
			Usage:       "Skip checksum check for Terragrunt engine files.",
			Hidden:      true,
		},
			flags.WithDeprecatedPrefix(terragruntPrefix, cliRedesignControl)),

		flags.NewFlag(&cli.GenericFlag[string]{
			Name:        EngineLogLevelFlagName,
			EnvVars:     tgPrefix.EnvVars(EngineLogLevelFlagName),
			Destination: &opts.EngineLogLevel,
			Usage:       "Terragrunt engine log level.",
			Hidden:      true,
		},
			flags.WithDeprecatedPrefix(terragruntPrefix, cliRedesignControl)),
	}

	return flags.Sort()
}
