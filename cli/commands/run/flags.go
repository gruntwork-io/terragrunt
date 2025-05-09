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

	BackendBootstrapFlagName        = "backend-bootstrap"
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

	// `--all` related flags.

	OutDirFlagName     = "out-dir"
	JSONOutDirFlagName = "json-out-dir"

	// `--graph` related flags.

	GraphRootFlagName = "graph-root"
)

// NewFlags creates and returns global flags.
func NewFlags(opts *options.TerragruntOptions) cli.Flags {
	strictControl := flags.StrictControlsByCommand(opts.StrictControls, CommandName)
	legacyLogsControl := flags.StrictControlsByCommand(opts.StrictControls, CommandName, controls.LegacyLogs)

	flags := cli.Flags{
		// `--all` related flags.

		flags.NewFlag(&cli.GenericFlag[string]{
			Name:        OutDirFlagName,
			EnvVars:     flags.EnvVarsWithTgPrefix(OutDirFlagName),
			ConfigKey:   flags.ConfigKey(OutDirFlagName),
			Destination: &opts.OutputFolder,
			Usage:       "Directory to store plan files.",
		},
			flags.WithDeprecatedNames(flags.FlagNamesWithTerragruntPrefix("out-dir"), strictControl)),

		flags.NewFlag(&cli.GenericFlag[string]{
			Name:        JSONOutDirFlagName,
			EnvVars:     flags.EnvVarsWithTgPrefix(JSONOutDirFlagName),
			ConfigKey:   flags.ConfigKey(JSONOutDirFlagName),
			Destination: &opts.JSONOutputFolder,
			Usage:       "Directory to store json plan files.",
		},
			flags.WithDeprecatedNames(flags.FlagNamesWithTerragruntPrefix("json-out-dir"), strictControl)),

		// `graph/-grpah` related flags.

		flags.NewFlag(&cli.GenericFlag[string]{
			Name:        GraphRootFlagName,
			EnvVars:     flags.EnvVarsWithTgPrefix(GraphRootFlagName),
			ConfigKey:   flags.ConfigKey(GraphRootFlagName),
			Destination: &opts.GraphRoot,
			Usage:       "Root directory from where to build graph dependencies.",
		},
			flags.WithDeprecatedName(flags.FlagNameWithTerragruntPrefix("graph-root"), strictControl)),

		//  Backward compatibility with `terragrunt-` prefix flags.

		flags.NewFlag(&cli.GenericFlag[string]{
			Name:        ConfigFlagName,
			EnvVars:     flags.EnvVarsWithTgPrefix(ConfigFlagName),
			ConfigKey:   flags.ConfigKey(ConfigFlagName),
			Destination: &opts.TerragruntConfigPath,
			Usage:       "The path to the Terragrunt config file. Default is terragrunt.hcl.",
		},
			flags.WithDeprecatedNames(flags.FlagNamesWithTerragruntPrefix("config"), strictControl)),

		flags.NewFlag(&cli.GenericFlag[string]{
			Name:        TFPathFlagName,
			EnvVars:     flags.EnvVarsWithTgPrefix(TFPathFlagName),
			ConfigKey:   flags.ConfigKey(TFPathFlagName),
			Destination: &opts.TerraformPath,
			Usage:       "Path to the OpenTofu/Terraform binary. Default is tofu (on PATH).",
		},
			flags.WithDeprecatedNames(flags.FlagNamesWithTerragruntPrefix("tfpath"), strictControl)),

		flags.NewFlag(&cli.BoolFlag{
			Name:        NoAutoInitFlagName,
			EnvVars:     flags.EnvVarsWithTgPrefix(NoAutoInitFlagName),
			ConfigKey:   flags.ConfigKey(NoAutoInitFlagName),
			Usage:       "Don't automatically run 'terraform/tofu init' during other terragrunt commands. You must run 'terragrunt init' manually.",
			Negative:    true,
			Destination: &opts.AutoInit,
		},
			flags.WithDeprecatedNames(flags.FlagNamesWithTerragruntPrefix("no-auto-init"), strictControl),
			flags.WithDeprecatedFlag(&cli.BoolFlag{
				EnvVars: flags.EnvVarsWithTerragruntPrefix("auto-init"),
			}, nil, strictControl)),

		flags.NewFlag(&cli.BoolFlag{
			Name:        NoAutoRetryFlagName,
			EnvVars:     flags.EnvVarsWithTgPrefix(NoAutoRetryFlagName),
			ConfigKey:   flags.ConfigKey(NoAutoRetryFlagName),
			Destination: &opts.AutoRetry,
			Usage:       "Don't automatically re-run command in case of transient errors.",
			Negative:    true,
		},
			flags.WithDeprecatedNames(flags.FlagNamesWithTerragruntPrefix("no-auto-retry"), strictControl),
			flags.WithDeprecatedFlag(&cli.BoolFlag{
				EnvVars: flags.EnvVarsWithTerragruntPrefix("auto-retry"),
			}, nil, strictControl)),

		flags.NewFlag(&cli.BoolFlag{
			Name:        NoAutoApproveFlagName,
			EnvVars:     flags.EnvVarsWithTgPrefix(NoAutoApproveFlagName),
			ConfigKey:   flags.ConfigKey(NoAutoApproveFlagName),
			Destination: &opts.RunAllAutoApprove,
			Usage:       "Don't automatically append '-auto-approve' to the underlying OpenTofu/Terraform commands run with 'run --all'.",
			Negative:    true,
		},
			flags.WithDeprecatedNames(flags.FlagNamesWithTerragruntPrefix("no-auto-approve"), strictControl),
			flags.WithDeprecatedFlag(&cli.BoolFlag{
				EnvVars: flags.EnvVarsWithTerragruntPrefix("auto-approve"),
			}, nil, strictControl)),

		flags.NewFlag(&cli.GenericFlag[string]{
			Name:        DownloadDirFlagName,
			EnvVars:     flags.EnvVarsWithTgPrefix(DownloadDirFlagName),
			ConfigKey:   flags.ConfigKey(DownloadDirFlagName),
			Destination: &opts.DownloadDir,
			Usage:       "The path to download OpenTofu/Terraform modules into. Default is .terragrunt-cache in the working directory.",
		}, flags.WithDeprecatedNamesEnvVars(
			flags.FlagNamesWithTerragruntPrefix("download-dir"),
			flags.EnvVarsWithTerragruntPrefix("download"),
			strictControl)),

		flags.NewFlag(&cli.GenericFlag[string]{
			Name:        SourceFlagName,
			EnvVars:     flags.EnvVarsWithTgPrefix(SourceFlagName),
			ConfigKey:   flags.ConfigKey(SourceFlagName),
			Destination: &opts.Source,
			Usage:       "Download OpenTofu/Terraform configurations from the specified source into a temporary folder, and run Terraform in that temporary folder.",
		},
			flags.WithDeprecatedNames(flags.FlagNamesWithTerragruntPrefix("source"), strictControl)),

		flags.NewFlag(&cli.BoolFlag{
			Name:        SourceUpdateFlagName,
			EnvVars:     flags.EnvVarsWithTgPrefix(SourceUpdateFlagName),
			ConfigKey:   flags.ConfigKey(SourceUpdateFlagName),
			Destination: &opts.SourceUpdate,
			Usage:       "Delete the contents of the temporary folder to clear out any old, cached source code before downloading new source code into it.",
		},
			flags.WithDeprecatedNames(flags.FlagNamesWithTerragruntPrefix("source-update"), strictControl)),

		flags.NewFlag(&cli.MapFlag[string, string]{
			Name:        SourceMapFlagName,
			EnvVars:     flags.EnvVarsWithTgPrefix(SourceMapFlagName),
			ConfigKey:   flags.ConfigKey(SourceMapFlagName),
			Destination: &opts.SourceMap,
			Usage:       "Replace any source URL (including the source URL of a config pulled in with dependency blocks) that has root source with dest.",
			Splitter:    util.SplitUrls,
		},
			flags.WithDeprecatedNames(flags.FlagNamesWithTerragruntPrefix("source-map"), strictControl)),

		// Assume IAM Role flags.

		flags.NewFlag(&cli.GenericFlag[string]{
			Name:        IAMAssumeRoleFlagName,
			EnvVars:     flags.EnvVarsWithTgPrefix(IAMAssumeRoleFlagName),
			ConfigKey:   flags.ConfigKey(IAMAssumeRoleFlagName),
			Destination: &opts.IAMRoleOptions.RoleARN,
			Usage:       "Assume the specified IAM role before executing OpenTofu/Terraform.",
		},
			flags.WithDeprecatedNames(flags.FlagNamesWithTerragruntPrefix("iam-role"), strictControl)),

		flags.NewFlag(&cli.GenericFlag[int64]{
			Name:        IAMAssumeRoleDurationFlagName,
			EnvVars:     flags.EnvVarsWithTgPrefix(IAMAssumeRoleDurationFlagName),
			ConfigKey:   flags.ConfigKey(IAMAssumeRoleDurationFlagName),
			Destination: &opts.IAMRoleOptions.AssumeRoleDuration,
			Usage:       "Session duration for IAM Assume Role session.",
		},
			flags.WithDeprecatedNames(flags.FlagNamesWithTerragruntPrefix("iam-assume-role-duration"), strictControl)),

		flags.NewFlag(&cli.GenericFlag[string]{
			Name:        IAMAssumeRoleSessionNameFlagName,
			EnvVars:     flags.EnvVarsWithTgPrefix(IAMAssumeRoleSessionNameFlagName),
			ConfigKey:   flags.ConfigKey(IAMAssumeRoleSessionNameFlagName),
			Destination: &opts.IAMRoleOptions.AssumeRoleSessionName,
			Usage:       "Name for the IAM Assumed Role session.",
		},
			flags.WithDeprecatedNames(flags.FlagNamesWithTerragruntPrefix("iam-assume-role-session-name"), strictControl)),

		flags.NewFlag(&cli.GenericFlag[string]{
			Name:        IAMAssumeRoleWebIdentityTokenFlagName,
			EnvVars:     flags.EnvVarsWithTgPrefix(IAMAssumeRoleWebIdentityTokenFlagName),
			ConfigKey:   flags.ConfigKey(IAMAssumeRoleWebIdentityTokenFlagName),
			Destination: &opts.IAMRoleOptions.WebIdentityToken,
			Usage:       "For AssumeRoleWithWebIdentity, the WebIdentity token.",
		},
			flags.WithDeprecatedNamesEnvVars(
				flags.FlagNamesWithTerragruntPrefix("iam-web-identity-token"),
				flags.EnvVarsWithTerragruntPrefix("iam-assume-role-web-identity-token"),
				strictControl)),

		flags.NewFlag(&cli.BoolFlag{
			Name:        QueueIgnoreErrorsFlagName,
			EnvVars:     flags.EnvVarsWithTgPrefix(QueueIgnoreErrorsFlagName),
			ConfigKey:   flags.ConfigKey(QueueIgnoreErrorsFlagName),
			Destination: &opts.IgnoreDependencyErrors,
			Usage:       "Continue processing Units even if a dependency fails.",
		},
			flags.WithDeprecatedNames(flags.FlagNamesWithTerragruntPrefix("ignore-dependency-errors"), strictControl)),

		flags.NewFlag(&cli.BoolFlag{
			Name:        QueueIgnoreDAGOrderFlagName,
			EnvVars:     flags.EnvVarsWithTgPrefix(QueueIgnoreDAGOrderFlagName),
			ConfigKey:   flags.ConfigKey(QueueIgnoreDAGOrderFlagName),
			Destination: &opts.IgnoreDependencyOrder,
			Usage:       "Ignore DAG order for --all commands.",
		},
			flags.WithDeprecatedNames(flags.FlagNamesWithTerragruntPrefix("ignore-dependency-order"), strictControl)),

		flags.NewFlag(&cli.BoolFlag{
			Name:        QueueExcludeExternalFlagName,
			EnvVars:     flags.EnvVarsWithTgPrefix(QueueExcludeExternalFlagName),
			ConfigKey:   flags.ConfigKey(QueueExcludeExternalFlagName),
			Destination: &opts.IgnoreExternalDependencies,
			Usage:       "Ignore external dependencies for --all commands.",
		},
			flags.WithDeprecatedNames(flags.FlagNamesWithTerragruntPrefix("ignore-external-dependencies"), strictControl)),

		flags.NewFlag(&cli.BoolFlag{
			Name:        QueueIncludeExternalFlagName,
			EnvVars:     flags.EnvVarsWithTgPrefix(QueueIncludeExternalFlagName),
			ConfigKey:   flags.ConfigKey(QueueIncludeExternalFlagName),
			Destination: &opts.IncludeExternalDependencies,
			Usage:       "Include external dependencies for --all commands without asking.",
		},
			flags.WithDeprecatedNames(flags.FlagNamesWithTerragruntPrefix("include-external-dependencies"), strictControl)),

		flags.NewFlag(&cli.GenericFlag[int]{
			Name:        ParallelismFlagName,
			EnvVars:     flags.EnvVarsWithTgPrefix(ParallelismFlagName),
			ConfigKey:   flags.ConfigKey(ParallelismFlagName),
			Destination: &opts.Parallelism,
			Usage:       "Parallelism for --all commands.",
		},
			flags.WithDeprecatedNames(flags.FlagNamesWithTerragruntPrefix("parallelism"), strictControl)),

		flags.NewFlag(&cli.GenericFlag[string]{
			Name:        QueueExcludesFileFlagName,
			EnvVars:     flags.EnvVarsWithTgPrefix(QueueExcludesFileFlagName),
			ConfigKey:   flags.ConfigKey(QueueExcludesFileFlagName),
			Destination: &opts.ExcludesFile,
			Usage:       "Path to a file with a list of directories that need to be excluded when running *-all commands.",
		},
			flags.WithDeprecatedNames(flags.FlagNamesWithTerragruntPrefix("excludes-file"), strictControl)),

		flags.NewFlag(&cli.SliceFlag[string]{
			Name:        QueueExcludeDirFlagName,
			EnvVars:     flags.EnvVarsWithTgPrefix(QueueExcludeDirFlagName),
			ConfigKey:   flags.ConfigKey(QueueExcludeDirFlagName),
			Destination: &opts.ExcludeDirs,
			Usage:       "Unix-style glob of directories to exclude from the queue of Units to run.",
		},
			flags.WithDeprecatedNames(flags.FlagNamesWithTerragruntPrefix("exclude-dir"), strictControl)),

		flags.NewFlag(&cli.SliceFlag[string]{
			Name:        QueueIncludeDirFlagName,
			EnvVars:     flags.EnvVarsWithTgPrefix(QueueIncludeDirFlagName),
			ConfigKey:   flags.ConfigKey(QueueIncludeDirFlagName),
			Destination: &opts.IncludeDirs,
			Usage:       "Unix-style glob of directories to include from the queue of Units to run.",
		},
			flags.WithDeprecatedNames(flags.FlagNamesWithTerragruntPrefix("include-dir"), strictControl)),

		flags.NewFlag(&cli.BoolFlag{
			Name:        InputsDebugFlagName,
			EnvVars:     flags.EnvVarsWithTgPrefix(InputsDebugFlagName),
			ConfigKey:   flags.ConfigKey(InputsDebugFlagName),
			Destination: &opts.Debug,
			Usage:       "Write debug.tfvars to working folder to help root-cause issues.",
		},
			flags.WithDeprecatedNames(flags.FlagNamesWithTerragruntPrefix("debug"), strictControl)),

		flags.NewFlag(&cli.BoolFlag{
			Name:        UsePartialParseConfigCacheFlagName,
			EnvVars:     flags.EnvVarsWithTgPrefix(UsePartialParseConfigCacheFlagName),
			ConfigKey:   flags.ConfigKey(UsePartialParseConfigCacheFlagName),
			Destination: &opts.UsePartialParseConfigCache,
			Usage:       "Enables caching of includes during partial parsing operations. Will also be used for the --iam-role option if provided.",
		},
			flags.WithDeprecatedNames(flags.FlagNamesWithTerragruntPrefix("use-partial-parse-config-cache"), strictControl)),

		flags.NewFlag(&cli.BoolFlag{
			Name:        DependencyFetchOutputFromStateFlagName,
			EnvVars:     flags.EnvVarsWithTgPrefix(DependencyFetchOutputFromStateFlagName),
			ConfigKey:   flags.ConfigKey(DependencyFetchOutputFromStateFlagName),
			Destination: &opts.FetchDependencyOutputFromState,
			Usage:       "The option fetches dependency output directly from the state file instead of using tofu/terraform output.",
		},
			flags.WithDeprecatedNames(flags.FlagNamesWithTerragruntPrefix("fetch-dependency-output-from-state"), strictControl)),

		flags.NewFlag(&cli.BoolFlag{
			Name:        TFForwardStdoutFlagName,
			EnvVars:     flags.EnvVarsWithTgPrefix(TFForwardStdoutFlagName),
			ConfigKey:   flags.ConfigKey(TFForwardStdoutFlagName),
			Destination: &opts.ForwardTFStdout,
			Usage:       "If specified, the output of OpenTofu/Terraform commands will be printed as is, without being integrated into the Terragrunt log.",
		},
			flags.WithDeprecatedNames(flags.FlagNamesWithTerragruntPrefix("forward-tf-stdout"), strictControl),
			flags.WithDeprecatedFlag(&cli.BoolFlag{
				Name:    flags.FlagNameWithTerragruntPrefix("include-module-prefix"),
				EnvVars: flags.EnvVarsWithTerragruntPrefix("include-module-prefix"),
				Usage:   "When this flag is set output from Terraform sub-commands is prefixed with module path.",
				Action: func(_ *cli.Context, _ bool) error {
					opts.Logger.Warnf("The --include-module-prefix flag is deprecated. Use the functionality-inverted --%s flag instead. By default, Terraform/OpenTofu output is integrated into the Terragrunt log, which prepends additional data, such as timestamps and prefixes, to log entries.", TFForwardStdoutFlagName)

					return nil
				},
			}, flags.NewValue(strconv.FormatBool(false)), legacyLogsControl)),

		flags.NewFlag(&cli.BoolFlag{
			Name:        QueueStrictIncludeFlagName,
			EnvVars:     flags.EnvVarsWithTgPrefix(QueueStrictIncludeFlagName),
			ConfigKey:   flags.ConfigKey(QueueStrictIncludeFlagName),
			Destination: &opts.StrictInclude,
			Usage:       "If flag is set, only modules under the directories passed in with '--include-dir' will be included.",
		},
			flags.WithDeprecatedNames(flags.FlagNamesWithTerragruntPrefix("strict-include"), strictControl)),

		flags.NewFlag(&cli.SliceFlag[string]{
			Name:        UnitsThatIncludeFlagName,
			EnvVars:     flags.EnvVarsWithTgPrefix(UnitsThatIncludeFlagName),
			ConfigKey:   flags.ConfigKey(UnitsThatIncludeFlagName),
			Destination: &opts.ModulesThatInclude,
			Usage:       "If flag is set, 'run --all' will only run the command against Terragrunt modules that include the specified file.",
		},
			flags.WithDeprecatedNames(flags.FlagNamesWithTerragruntPrefix("modules-that-include"), strictControl)),

		flags.NewFlag(&cli.SliceFlag[string]{
			Name:        QueueIncludeUnitsReadingFlagName,
			EnvVars:     flags.EnvVarsWithTgPrefix(QueueIncludeUnitsReadingFlagName),
			ConfigKey:   flags.ConfigKey(QueueIncludeUnitsReadingFlagName),
			Destination: &opts.UnitsReading,
			Usage:       "If flag is set, 'run --all' will only run the command against Terragrunt units that read the specified file via an HCL function or include.",
		},
			flags.WithDeprecatedNames(flags.FlagNamesWithTerragruntPrefix("queue-include-units-reading"), strictControl)),

		flags.NewFlag(&cli.BoolFlag{
			Name:        BackendBootstrapFlagName,
			EnvVars:     flags.EnvVarsWithTgPrefix(BackendBootstrapFlagName),
			ConfigKey:   flags.ConfigKey(BackendBootstrapFlagName),
			Destination: &opts.BackendBootstrap,
			Usage:       "Automatically bootstrap backend infrastructure before attempting to use it.",
		}),

		flags.NewFlag(&cli.BoolFlag{
			Name:        BackendRequireBootstrapFlagName,
			EnvVars:     flags.EnvVarsWithTgPrefix(BackendRequireBootstrapFlagName),
			ConfigKey:   flags.ConfigKey(BackendRequireBootstrapFlagName),
			Destination: &opts.FailIfBucketCreationRequired,
			Usage:       "When this flag is set Terragrunt will fail if the remote state bucket needs to be created.",
		},
			flags.WithDeprecatedNames(flags.FlagNamesWithTerragruntPrefix("fail-on-state-bucket-creation"), strictControl)),

		flags.NewFlag(&cli.BoolFlag{
			Name:        DisableBucketUpdateFlagName,
			EnvVars:     flags.EnvVarsWithTgPrefix(DisableBucketUpdateFlagName),
			ConfigKey:   flags.ConfigKey(DisableBucketUpdateFlagName),
			Destination: &opts.DisableBucketUpdate,
			Usage:       "When this flag is set Terragrunt will not update the remote state bucket.",
		},
			flags.WithDeprecatedNames(flags.FlagNamesWithTerragruntPrefix("disable-bucket-update"), strictControl)),

		flags.NewFlag(&cli.BoolFlag{
			Name:        DisableCommandValidationFlagName,
			EnvVars:     flags.EnvVarsWithTgPrefix(DisableCommandValidationFlagName),
			ConfigKey:   flags.ConfigKey(DisableCommandValidationFlagName),
			Destination: &opts.DisableCommandValidation,
			Usage:       "When this flag is set, Terragrunt will not validate the tofu/terraform command.",
		},
			flags.WithDeprecatedNames(flags.FlagNamesWithTerragruntPrefix("disable-command-validation"), strictControl)),

		flags.NewFlag(&cli.BoolFlag{
			Name:        NoDestroyDependenciesCheckFlagName,
			EnvVars:     flags.EnvVarsWithTgPrefix(NoDestroyDependenciesCheckFlagName),
			ConfigKey:   flags.ConfigKey(NoDestroyDependenciesCheckFlagName),
			Destination: &opts.NoDestroyDependenciesCheck,
			Usage:       "When this flag is set, Terragrunt will not check for dependent units when destroying.",
		},
			flags.WithDeprecatedNames(flags.FlagNamesWithTerragruntPrefix("no-destroy-dependencies-check"), strictControl)),

		// Terragrunt Provider Cache flags.

		flags.NewFlag(&cli.BoolFlag{
			Name:        ProviderCacheFlagName,
			EnvVars:     flags.EnvVarsWithTgPrefix(ProviderCacheFlagName),
			ConfigKey:   flags.ConfigKey(ProviderCacheFlagName),
			Destination: &opts.ProviderCache,
			Usage:       "Enables Terragrunt's provider caching.",
		},
			flags.WithDeprecatedNames(flags.FlagNamesWithTerragruntPrefix("provider-cache"), strictControl)),

		flags.NewFlag(&cli.GenericFlag[string]{
			Name:        ProviderCacheDirFlagName,
			EnvVars:     flags.EnvVarsWithTgPrefix(ProviderCacheDirFlagName),
			ConfigKey:   flags.ConfigKey(ProviderCacheDirFlagName),
			Destination: &opts.ProviderCacheDir,
			Usage:       "The path to the Terragrunt provider cache directory. By default, 'terragrunt/providers' folder in the user cache directory.",
		},
			flags.WithDeprecatedNames(flags.FlagNamesWithTerragruntPrefix("provider-cache-dir"), strictControl)),

		flags.NewFlag(&cli.GenericFlag[string]{
			Name:        ProviderCacheTokenFlagName,
			EnvVars:     flags.EnvVarsWithTgPrefix(ProviderCacheTokenFlagName),
			ConfigKey:   flags.ConfigKey(ProviderCacheTokenFlagName),
			Destination: &opts.ProviderCacheToken,
			Usage:       "The token for authentication to the Terragrunt Provider Cache server. By default, assigned automatically.",
		},
			flags.WithDeprecatedNames(flags.FlagNamesWithTerragruntPrefix("provider-cache-token"), strictControl)),

		flags.NewFlag(&cli.GenericFlag[string]{
			Name:        ProviderCacheHostnameFlagName,
			EnvVars:     flags.EnvVarsWithTgPrefix(ProviderCacheHostnameFlagName),
			ConfigKey:   flags.ConfigKey(ProviderCacheHostnameFlagName),
			Destination: &opts.ProviderCacheHostname,
			Usage:       "The hostname of the Terragrunt Provider Cache server. By default, 'localhost'.",
		},
			flags.WithDeprecatedNames(flags.FlagNamesWithTerragruntPrefix("provider-cache-hostname"), strictControl)),

		flags.NewFlag(&cli.GenericFlag[int]{
			Name:        ProviderCachePortFlagName,
			EnvVars:     flags.EnvVarsWithTgPrefix(ProviderCachePortFlagName),
			ConfigKey:   flags.ConfigKey(ProviderCachePortFlagName),
			Destination: &opts.ProviderCachePort,
			Usage:       "The port of the Terragrunt Provider Cache server. By default, assigned automatically.",
		},
			flags.WithDeprecatedNames(flags.FlagNamesWithTerragruntPrefix("provider-cache-port"), strictControl)),

		flags.NewFlag(&cli.SliceFlag[string]{
			Name:        ProviderCacheRegistryNamesFlagName,
			EnvVars:     flags.EnvVarsWithTgPrefix(ProviderCacheRegistryNamesFlagName),
			ConfigKey:   flags.ConfigKey(ProviderCacheRegistryNamesFlagName),
			Destination: &opts.ProviderCacheRegistryNames,
			Usage:       "The list of remote registries to cached by Terragrunt Provider Cache server. By default, 'registry.terraform.io', 'registry.opentofu.org'.",
		},
			flags.WithDeprecatedNames(flags.FlagNamesWithTerragruntPrefix("provider-cache-registry-names"), strictControl)),

		flags.NewFlag(&cli.GenericFlag[string]{
			Name:        AuthProviderCmdFlagName,
			EnvVars:     flags.EnvVarsWithTgPrefix(AuthProviderCmdFlagName),
			ConfigKey:   flags.ConfigKey(AuthProviderCmdFlagName),
			Destination: &opts.AuthProviderCmd,
			Usage:       "Run the provided command and arguments to authenticate Terragrunt dynamically when necessary.",
		},
			flags.WithDeprecatedNames(flags.FlagNamesWithTerragruntPrefix("auth-provider-cmd"), strictControl)),

		flags.NewFlag(&cli.MapFlag[string, string]{
			Name:      FeatureFlagName,
			EnvVars:   flags.EnvVarsWithTgPrefix(FeatureFlagName),
			ConfigKey: flags.ConfigKey(FeatureFlagName),
			Usage:     "Set feature flags for the HCL code.",
			Splitter:  util.SplitComma,
			Action: func(_ *cli.Context, value map[string]string) error {
				for key, val := range value {
					opts.FeatureFlags.Store(key, val)
				}

				return nil
			},
		},
			flags.WithDeprecatedNames(flags.FlagNamesWithTerragruntPrefix("feature"), strictControl)),

		// Terragrunt engine flags.

		flags.NewFlag(&cli.BoolFlag{
			Name:        EngineEnableFlagName,
			EnvVars:     flags.EnvVarsWithTgPrefix(EngineEnableFlagName),
			ConfigKey:   flags.ConfigKey(EngineEnableFlagName),
			Destination: &opts.EngineEnabled,
			Usage:       "Enable Terragrunt experimental engine.",
			Hidden:      true,
		},
			flags.WithDeprecatedNames(flags.FlagNamesWithTerragruntPrefix("experimental-engine"), strictControl)),

		flags.NewFlag(&cli.GenericFlag[string]{
			Name:        EngineCachePathFlagName,
			EnvVars:     flags.EnvVarsWithTgPrefix(EngineCachePathFlagName),
			ConfigKey:   flags.ConfigKey(EngineCachePathFlagName),
			Destination: &opts.EngineCachePath,
			Usage:       "Cache path for Terragrunt engine files.",
			Hidden:      true,
		},
			flags.WithDeprecatedNames(flags.FlagNamesWithTerragruntPrefix("engine-cache-path"), strictControl)),

		flags.NewFlag(&cli.BoolFlag{
			Name:        EngineSkipCheckFlagName,
			EnvVars:     flags.EnvVarsWithTgPrefix(EngineSkipCheckFlagName),
			ConfigKey:   flags.ConfigKey(EngineSkipCheckFlagName),
			Destination: &opts.EngineSkipChecksumCheck,
			Usage:       "Skip checksum check for Terragrunt engine files.",
			Hidden:      true,
		},
			flags.WithDeprecatedNames(flags.FlagNamesWithTerragruntPrefix("engine-skip-check"), strictControl)),

		flags.NewFlag(&cli.GenericFlag[string]{
			Name:        EngineLogLevelFlagName,
			EnvVars:     flags.EnvVarsWithTgPrefix(EngineLogLevelFlagName),
			ConfigKey:   flags.ConfigKey(EngineLogLevelFlagName),
			Destination: &opts.EngineLogLevel,
			Usage:       "Terragrunt engine log level.",
			Hidden:      true,
		},
			flags.WithDeprecatedNames(flags.FlagNamesWithTerragruntPrefix("engine-log-level"), strictControl)),
	}

	return flags.Sort()
}
