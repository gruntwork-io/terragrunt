// Package run provides Terragrunt command flags.
package run

import (
	"fmt"
	"path/filepath"

	"github.com/gruntwork-io/terragrunt/cli/flags"
	"github.com/gruntwork-io/terragrunt/cli/flags/shared"
	"github.com/gruntwork-io/terragrunt/internal/cli"
	"github.com/gruntwork-io/terragrunt/internal/report"
	"github.com/gruntwork-io/terragrunt/internal/strict/controls"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/util"
)

const (
	NoAutoInitFlagName                     = "no-auto-init"
	NoAutoRetryFlagName                    = "no-auto-retry"
	NoAutoApproveFlagName                  = "no-auto-approve"
	NoAutoProviderCacheDirFlagName         = "no-auto-provider-cache-dir"
	TFForwardStdoutFlagName                = "tf-forward-stdout"
	UnitsThatIncludeFlagName               = "units-that-include"
	DependencyFetchOutputFromStateFlagName = "dependency-fetch-output-from-state"
	UsePartialParseConfigCacheFlagName     = "use-partial-parse-config-cache"
	SummaryPerUnitFlagName                 = "summary-per-unit"
	VersionManagerFileNameFlagName         = "version-manager-file-name"

	DisableCommandValidationFlagName   = "disable-command-validation"
	NoDestroyDependenciesCheckFlagName = "no-destroy-dependencies-check"

	SourceFlagName       = "source"
	SourceMapFlagName    = "source-map"
	SourceUpdateFlagName = "source-update"

	NoStackGenerate = "no-stack-generate"

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

	// Report related flags.

	SummaryDisableFlagName = "summary-disable"
	ReportFileFlagName     = "report-file"
	ReportFormatFlagName   = "report-format"
	ReportSchemaFlagName   = "report-schema-file"

	// `--all` related flags.

	OutDirFlagName     = "out-dir"
	JSONOutDirFlagName = "json-out-dir"

	// `--graph` related flags.
	GraphRootFlagName = "graph-root"

	FailFastFlagName = "fail-fast"

	// Backend and feature flags (shared with backend commands) - use shared package constants
	BackendBootstrapFlagName        = shared.BackendBootstrapFlagName
	BackendRequireBootstrapFlagName = shared.BackendRequireBootstrapFlagName
	DisableBucketUpdateFlagName     = shared.DisableBucketUpdateFlagName
	FeatureFlagName                 = shared.FeatureFlagName

	// Config and download flags - use shared package constants
	ConfigFlagName      = shared.ConfigFlagName
	DownloadDirFlagName = shared.DownloadDirFlagName

	// Auth and IAM flags - use shared package constants
	AuthProviderCmdFlagName               = shared.AuthProviderCmdFlagName
	InputsDebugFlagName                   = shared.InputsDebugFlagName
	IAMAssumeRoleFlagName                 = shared.IAMAssumeRoleFlagName
	IAMAssumeRoleDurationFlagName         = shared.IAMAssumeRoleDurationFlagName
	IAMAssumeRoleSessionNameFlagName      = shared.IAMAssumeRoleSessionNameFlagName
	IAMAssumeRoleWebIdentityTokenFlagName = shared.IAMAssumeRoleWebIdentityTokenFlagName
)

// NewFlags creates and returns global flags.
func NewFlags(l log.Logger, opts *options.TerragruntOptions, prefix flags.Prefix) cli.Flags {
	tgPrefix := flags.Prefix{flags.TgPrefix}
	terragruntPrefix := flags.Prefix{flags.TerragruntPrefix}
	terragruntPrefixControl := flags.StrictControlsByCommand(opts.StrictControls, CommandName)
	legacyLogsControl := flags.StrictControlsByCommand(opts.StrictControls, CommandName, controls.LegacyLogs)

	flags := cli.Flags{
		// `--all` related flags.

		flags.NewFlag(&cli.GenericFlag[string]{
			Name:        OutDirFlagName,
			EnvVars:     tgPrefix.EnvVars(OutDirFlagName),
			Destination: &opts.OutputFolder,
			Usage:       "Directory to store plan files.",
		},
			flags.WithDeprecatedEnvVars(terragruntPrefix.EnvVars("out-dir"), terragruntPrefixControl)),

		flags.NewFlag(&cli.GenericFlag[string]{
			Name:        JSONOutDirFlagName,
			EnvVars:     tgPrefix.EnvVars(JSONOutDirFlagName),
			Destination: &opts.JSONOutputFolder,
			Usage:       "Directory to store json plan files.",
		},
			flags.WithDeprecatedEnvVars(terragruntPrefix.EnvVars("json-out-dir"), terragruntPrefixControl)),

		// `graph/-graph` related flags.

		flags.NewFlag(&cli.GenericFlag[string]{
			Name:        GraphRootFlagName,
			EnvVars:     tgPrefix.EnvVars(GraphRootFlagName),
			Destination: &opts.GraphRoot,
			Usage:       "Root directory from where to build graph dependencies.",
		},
			flags.WithDeprecatedEnvVars(terragruntPrefix.EnvVars("graph-root"), terragruntPrefixControl)),

		// `--all` and `--graph` related flags.

		flags.NewFlag(&cli.BoolFlag{
			Name:        NoStackGenerate,
			EnvVars:     tgPrefix.EnvVars(NoStackGenerate),
			Destination: &opts.NoStackGenerate,
			Usage:       "Disable automatic stack regeneration before running the command.",
		}),

		//  Backward compatibility with `terragrunt-` prefix flags.

		shared.NewConfigFlag(opts, prefix, CommandName),

		shared.NewTFPathFlag(opts),

		flags.NewFlag(&cli.BoolFlag{
			Name:        NoAutoInitFlagName,
			EnvVars:     tgPrefix.EnvVars(NoAutoInitFlagName),
			Usage:       "Don't automatically run 'terraform/tofu init' during other terragrunt commands. You must run 'terragrunt init' manually.",
			Negative:    true,
			Destination: &opts.AutoInit,
		},
			flags.WithDeprecatedFlag(&cli.BoolFlag{
				EnvVars: terragruntPrefix.EnvVars("auto-init"),
			}, nil, terragruntPrefixControl)),

		flags.NewFlag(&cli.BoolFlag{
			Name:        NoAutoRetryFlagName,
			EnvVars:     tgPrefix.EnvVars(NoAutoRetryFlagName),
			Destination: &opts.AutoRetry,
			Usage:       "Don't automatically re-run command in case of transient errors.",
			Negative:    true,
		},
			flags.WithDeprecatedFlag(&cli.BoolFlag{
				EnvVars: terragruntPrefix.EnvVars("auto-retry"),
			}, nil, terragruntPrefixControl)),

		flags.NewFlag(&cli.BoolFlag{
			Name:        NoAutoApproveFlagName,
			EnvVars:     tgPrefix.EnvVars(NoAutoApproveFlagName),
			Destination: &opts.RunAllAutoApprove,
			Usage:       "Don't automatically append '-auto-approve' to the underlying OpenTofu/Terraform commands run with 'run --all'.",
			Negative:    true,
		},
			flags.WithDeprecatedFlag(&cli.BoolFlag{
				EnvVars: terragruntPrefix.EnvVars("auto-approve"),
			}, nil, terragruntPrefixControl)),

		flags.NewFlag(&cli.BoolFlag{
			Name:        NoAutoProviderCacheDirFlagName,
			EnvVars:     tgPrefix.EnvVars(NoAutoProviderCacheDirFlagName),
			Destination: &opts.NoAutoProviderCacheDir,
			Usage:       "Disable the auto-provider-cache-dir feature even when the experiment is enabled.",
		}),

		shared.NewDownloadDirFlag(opts, prefix, CommandName),

		flags.NewFlag(&cli.GenericFlag[string]{
			Name:        SourceFlagName,
			EnvVars:     tgPrefix.EnvVars(SourceFlagName),
			Destination: &opts.Source,
			Usage:       "Download OpenTofu/Terraform configurations from the specified source into a temporary folder, and run Terraform in that temporary folder.",
		},
			flags.WithDeprecatedEnvVars(terragruntPrefix.EnvVars("source"), terragruntPrefixControl)),

		flags.NewFlag(&cli.BoolFlag{
			Name:        SourceUpdateFlagName,
			EnvVars:     tgPrefix.EnvVars(SourceUpdateFlagName),
			Destination: &opts.SourceUpdate,
			Usage:       "Delete the contents of the temporary folder to clear out any old, cached source code before downloading new source code into it.",
		},
			flags.WithDeprecatedEnvVars(terragruntPrefix.EnvVars("source-update"), terragruntPrefixControl)),

		flags.NewFlag(&cli.MapFlag[string, string]{
			Name:        SourceMapFlagName,
			EnvVars:     tgPrefix.EnvVars(SourceMapFlagName),
			Destination: &opts.SourceMap,
			Usage:       "Replace any source URL (including the source URL of a config pulled in with dependency blocks) that has root source with dest.",
			Splitter:    util.SplitUrls,
		},
			flags.WithDeprecatedEnvVars(terragruntPrefix.EnvVars("source-map"), terragruntPrefixControl)),

		// Assume IAM Role flags.
		shared.NewInputsDebugFlag(opts, prefix, CommandName),

		flags.NewFlag(&cli.BoolFlag{
			Name:        UsePartialParseConfigCacheFlagName,
			EnvVars:     tgPrefix.EnvVars(UsePartialParseConfigCacheFlagName),
			Destination: &opts.UsePartialParseConfigCache,
			Usage:       "Enables caching of includes during partial parsing operations. Will also be used for the --iam-role option if provided.",
		},
			flags.WithDeprecatedEnvVars(terragruntPrefix.EnvVars("use-partial-parse-config-cache"), terragruntPrefixControl)),

		flags.NewFlag(&cli.SliceFlag[string]{
			Name:        VersionManagerFileNameFlagName,
			EnvVars:     tgPrefix.EnvVars(VersionManagerFileNameFlagName),
			Destination: &opts.VersionManagerFileName,
			Usage:       "File names used during the computation of the cache key for the version manager files.",
		}),

		flags.NewFlag(&cli.BoolFlag{
			Name:        DependencyFetchOutputFromStateFlagName,
			EnvVars:     tgPrefix.EnvVars(DependencyFetchOutputFromStateFlagName),
			Destination: &opts.FetchDependencyOutputFromState,
			Usage:       "The option fetches dependency output directly from the state file instead of using tofu/terraform output.",
		},
			flags.WithDeprecatedEnvVars(terragruntPrefix.EnvVars("fetch-dependency-output-from-state"), terragruntPrefixControl)),

		flags.NewFlag(&cli.BoolFlag{
			Name:        TFForwardStdoutFlagName,
			EnvVars:     tgPrefix.EnvVars(TFForwardStdoutFlagName),
			Destination: &opts.ForwardTFStdout,
			Usage:       "If specified, the output of OpenTofu/Terraform commands will be printed as is, without being integrated into the Terragrunt log.",
		},
			flags.WithDeprecatedEnvVars(terragruntPrefix.EnvVars("forward-tf-stdout"), terragruntPrefixControl),
			flags.WithDeprecatedEnvVars(terragruntPrefix.EnvVars("include-module-prefix"), legacyLogsControl)),

		flags.NewFlag(&cli.SliceFlag[string]{
			Name:        UnitsThatIncludeFlagName,
			EnvVars:     tgPrefix.EnvVars(UnitsThatIncludeFlagName),
			Destination: &opts.ModulesThatInclude,
			Usage:       "If flag is set, 'run --all' will only run the command against Terragrunt modules that include the specified file.",
		},
			flags.WithDeprecatedEnvVars(terragruntPrefix.EnvVars("modules-that-include"), terragruntPrefixControl)),

		flags.NewFlag(&cli.BoolFlag{
			Name:        DisableCommandValidationFlagName,
			EnvVars:     tgPrefix.EnvVars(DisableCommandValidationFlagName),
			Destination: &opts.DisableCommandValidation,
			Usage:       "When this flag is set, Terragrunt will not validate the tofu/terraform command.",
		},
			flags.WithDeprecatedEnvVars(terragruntPrefix.EnvVars("disable-command-validation"), terragruntPrefixControl)),

		flags.NewFlag(&cli.BoolFlag{
			Name:        NoDestroyDependenciesCheckFlagName,
			EnvVars:     tgPrefix.EnvVars(NoDestroyDependenciesCheckFlagName),
			Destination: &opts.NoDestroyDependenciesCheck,
			Usage:       "When this flag is set, Terragrunt will not check for dependent units when destroying.",
		},
			flags.WithDeprecatedEnvVars(terragruntPrefix.EnvVars("no-destroy-dependencies-check"), terragruntPrefixControl)),

		// Terragrunt Provider Cache flags.

		flags.NewFlag(&cli.BoolFlag{
			Name:        ProviderCacheFlagName,
			EnvVars:     tgPrefix.EnvVars(ProviderCacheFlagName),
			Destination: &opts.ProviderCache,
			Usage:       "Enables Terragrunt's provider caching.",
		},
			flags.WithDeprecatedEnvVars(terragruntPrefix.EnvVars("provider-cache"), terragruntPrefixControl)),

		flags.NewFlag(&cli.GenericFlag[string]{
			Name:        ProviderCacheDirFlagName,
			EnvVars:     tgPrefix.EnvVars(ProviderCacheDirFlagName),
			Destination: &opts.ProviderCacheDir,
			Usage:       "The path to the Terragrunt provider cache directory. By default, 'terragrunt/providers' folder in the user cache directory.",
		},
			flags.WithDeprecatedEnvVars(terragruntPrefix.EnvVars("provider-cache-dir"), terragruntPrefixControl)),

		flags.NewFlag(&cli.GenericFlag[string]{
			Name:        ProviderCacheTokenFlagName,
			EnvVars:     tgPrefix.EnvVars(ProviderCacheTokenFlagName),
			Destination: &opts.ProviderCacheToken,
			Usage:       "The token for authentication to the Terragrunt Provider Cache server. By default, assigned automatically.",
		},
			flags.WithDeprecatedEnvVars(terragruntPrefix.EnvVars("provider-cache-token"), terragruntPrefixControl)),

		flags.NewFlag(&cli.GenericFlag[string]{
			Name:        ProviderCacheHostnameFlagName,
			EnvVars:     tgPrefix.EnvVars(ProviderCacheHostnameFlagName),
			Destination: &opts.ProviderCacheHostname,
			Usage:       "The hostname of the Terragrunt Provider Cache server. By default, 'localhost'.",
		},
			flags.WithDeprecatedEnvVars(terragruntPrefix.EnvVars("provider-cache-hostname"), terragruntPrefixControl)),

		flags.NewFlag(&cli.GenericFlag[int]{
			Name:        ProviderCachePortFlagName,
			EnvVars:     tgPrefix.EnvVars(ProviderCachePortFlagName),
			Destination: &opts.ProviderCachePort,
			Usage:       "The port of the Terragrunt Provider Cache server. By default, assigned automatically.",
		},
			flags.WithDeprecatedEnvVars(terragruntPrefix.EnvVars("provider-cache-port"), terragruntPrefixControl)),

		flags.NewFlag(&cli.SliceFlag[string]{
			Name:        ProviderCacheRegistryNamesFlagName,
			EnvVars:     tgPrefix.EnvVars(ProviderCacheRegistryNamesFlagName),
			Destination: &opts.ProviderCacheRegistryNames,
			Usage:       "The list of remote registries to cached by Terragrunt Provider Cache server. By default, 'registry.terraform.io', 'registry.opentofu.org'.",
		},
			flags.WithDeprecatedEnvVars(terragruntPrefix.EnvVars("provider-cache-registry-names"), terragruntPrefixControl)),

		shared.NewAuthProviderCmdFlag(opts, prefix, CommandName),

		// Terragrunt engine flags.

		flags.NewFlag(&cli.BoolFlag{
			Name:        EngineEnableFlagName,
			EnvVars:     tgPrefix.EnvVars(EngineEnableFlagName),
			Destination: &opts.EngineEnabled,
			Usage:       "Enable Terragrunt experimental engine.",
			Hidden:      true,
		},
			flags.WithDeprecatedEnvVars(terragruntPrefix.EnvVars("experimental-engine"), terragruntPrefixControl)),

		flags.NewFlag(&cli.GenericFlag[string]{
			Name:        EngineCachePathFlagName,
			EnvVars:     tgPrefix.EnvVars(EngineCachePathFlagName),
			Destination: &opts.EngineCachePath,
			Usage:       "Cache path for Terragrunt engine files.",
			Hidden:      true,
		},
			flags.WithDeprecatedEnvVars(terragruntPrefix.EnvVars("engine-cache-path"), terragruntPrefixControl)),

		flags.NewFlag(&cli.BoolFlag{
			Name:        EngineSkipCheckFlagName,
			EnvVars:     tgPrefix.EnvVars(EngineSkipCheckFlagName),
			Destination: &opts.EngineSkipChecksumCheck,
			Usage:       "Skip checksum check for Terragrunt engine files.",
			Hidden:      true,
		},
			flags.WithDeprecatedEnvVars(terragruntPrefix.EnvVars("engine-skip-check"), terragruntPrefixControl)),

		flags.NewFlag(&cli.GenericFlag[string]{
			Name:        EngineLogLevelFlagName,
			EnvVars:     tgPrefix.EnvVars(EngineLogLevelFlagName),
			Destination: &opts.EngineLogLevel,
			Usage:       "Terragrunt engine log level.",
			Hidden:      true,
		},
			flags.WithDeprecatedEnvVars(terragruntPrefix.EnvVars("engine-log-level"), terragruntPrefixControl)),

		flags.NewFlag(&cli.BoolFlag{
			Name:        SummaryDisableFlagName,
			EnvVars:     tgPrefix.EnvVars(SummaryDisableFlagName),
			Destination: &opts.SummaryDisable,
			Usage:       `Disable the summary output at the end of a run.`,
		}),

		flags.NewFlag(&cli.BoolFlag{
			Name:        SummaryPerUnitFlagName,
			EnvVars:     tgPrefix.EnvVars(SummaryPerUnitFlagName),
			Destination: &opts.SummaryPerUnit,
			Usage:       `Show duration information for each unit in the summary output.`,
		}),

		flags.NewFlag(&cli.GenericFlag[string]{
			Name:    ReportFileFlagName,
			EnvVars: tgPrefix.EnvVars(ReportFileFlagName),
			Usage:   `Path to generate report file in.`,
			Setter: func(value string) error {
				if value == "" {
					return nil
				}

				opts.ReportFile = value

				ext := filepath.Ext(value)
				if ext == "" {
					ext = ".csv"
				}

				if ext != ".csv" && ext != ".json" {
					return nil
				}

				if opts.ReportFormat == "" {
					opts.ReportFormat = report.Format(ext[1:])
				}

				return nil
			},
		}),

		flags.NewFlag(&cli.GenericFlag[string]{
			Name:    ReportFormatFlagName,
			EnvVars: tgPrefix.EnvVars(ReportFormatFlagName),
			Usage:   `Format of the report file.`,
			Setter: func(value string) error {
				if value == "" && opts.ReportFormat == "" {
					opts.ReportFormat = report.FormatCSV

					return nil
				}

				opts.ReportFormat = report.Format(value)

				switch opts.ReportFormat {
				case report.FormatCSV:
				case report.FormatJSON:
				default:
					return fmt.Errorf("unsupported report format: %s", value)
				}

				return nil
			},
		}),

		flags.NewFlag(&cli.GenericFlag[string]{
			Name:        ReportSchemaFlagName,
			EnvVars:     tgPrefix.EnvVars(ReportSchemaFlagName),
			Usage:       `Path to generate report schema file in.`,
			Destination: &opts.ReportSchemaFile,
		}),

		flags.NewFlag(&cli.BoolFlag{
			Name:        FailFastFlagName,
			EnvVars:     tgPrefix.EnvVars(FailFastFlagName),
			Destination: &opts.FailFast,
			Usage:       "Fail the run if any unit fails. This will make it so that any unit failing causes the whole run to fail.",
		}),
	}

	// Add shared flags
	flags = flags.Add(shared.NewBackendFlags(opts, prefix)...)
	flags = flags.Add(shared.NewFeatureFlags(opts, prefix)...)
	flags = flags.Add(shared.NewIAMAssumeRoleFlags(opts, prefix, CommandName)...)
	flags = flags.Add(shared.NewQueueFlags(opts, prefix)...)
	flags = flags.Add(shared.NewFilterFlags(opts)...)
	flags = flags.Add(shared.NewParallelismFlag(opts))

	return flags.Sort()
}
