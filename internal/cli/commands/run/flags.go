package run

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/gruntwork-io/terragrunt/internal/cli/flags"
	"github.com/gruntwork-io/terragrunt/internal/cli/flags/shared"
	"github.com/gruntwork-io/terragrunt/internal/clihelper"
	"github.com/gruntwork-io/terragrunt/internal/experiment"
	"github.com/gruntwork-io/terragrunt/internal/filter"
	"github.com/gruntwork-io/terragrunt/internal/report"
	"github.com/gruntwork-io/terragrunt/internal/strict/controls"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/pkg/options"
)

const (
	// NoAutoRetryFlagName is the name of the flag that turns off re-running a
	// command after a transient error.
	NoAutoRetryFlagName = "no-auto-retry"
	// NoAutoApproveFlagName is the name of the flag that stops `run --all` from
	// appending -auto-approve to the tofu command.
	NoAutoApproveFlagName = "no-auto-approve"
	// NoAutoProviderCacheDirFlagName is the name of the flag that turns off the
	// auto-provider-cache-dir feature even when its experiment is enabled.
	NoAutoProviderCacheDirFlagName = "no-auto-provider-cache-dir"
	// NoEngineFlagName is the name of the hidden flag that turns off IaC engines
	// even when the iac-engine experiment is enabled.
	NoEngineFlagName = "no-engine"
	// NoDependencyFetchOutputFromStateFlagName is the name of the flag that
	// reads dependency outputs by running `tofu output` in place of reading
	// the state file.
	NoDependencyFetchOutputFromStateFlagName = "no-dependency-fetch-output-from-state"
	// NoHooksFlagName is the name of the flag that skips Terragrunt hooks.
	NoHooksFlagName = "no-hooks"
	// NoDependencyOutputsFlagName is the name of the flag that skips resolving
	// dependency outputs.
	NoDependencyOutputsFlagName = "no-dependency-outputs"
	// TFForwardStdoutFlagName is the name of the flag that prints tofu output
	// as is, outside the Terragrunt log.
	TFForwardStdoutFlagName = "tf-forward-stdout"
	// UnitsThatIncludeFlagName is the name of the hidden, deprecated flag that
	// limits `run --all` to units that read the given file. Setting it
	// evaluates its strict control.
	UnitsThatIncludeFlagName = "units-that-include"
	// DependencyFetchOutputFromStateFlagName is the name of the deprecated flag
	// that reads dependency outputs from the state file, which is already the
	// default. Setting it evaluates its strict control.
	DependencyFetchOutputFromStateFlagName = "dependency-fetch-output-from-state"
	// UsePartialParseConfigCacheFlagName is the name of the flag that caches
	// includes during partial parsing.
	UsePartialParseConfigCacheFlagName = "use-partial-parse-config-cache"
	// SummaryPerUnitFlagName is the name of the flag that adds each unit's
	// duration to the run summary.
	SummaryPerUnitFlagName = "summary-per-unit"
	// VersionManagerFileNameFlagName is the name of the flag that sets the
	// version manager file names hashed into the cache key.
	VersionManagerFileNameFlagName = "version-manager-file-name"

	// DisableCommandValidationFlagName is the name of the hidden, deprecated
	// flag that skips validating the tofu command name. Setting it evaluates
	// its strict control.
	DisableCommandValidationFlagName = "disable-command-validation"
	// NoDestroyDependenciesCheckFlagName is the name of the hidden, deprecated
	// flag that skips checking for dependent units on destroy. Setting it
	// evaluates its strict control.
	NoDestroyDependenciesCheckFlagName = "no-destroy-dependencies-check"
	// DestroyDependenciesCheckFlagName is the name of the flag that checks for
	// dependent units before a destroy.
	DestroyDependenciesCheckFlagName = "destroy-dependencies-check"

	// SourceUpdateFlagName is the name of the flag that clears the download
	// directory before downloading the unit's source.
	SourceUpdateFlagName = "source-update"

	// Terragrunt Provider Cache related flags.

	// ProviderCacheFlagName is the name of the flag that turns on the
	// Terragrunt Provider Cache.
	ProviderCacheFlagName = "provider-cache"
	// ProviderCacheDirFlagName is the name of the flag that sets the provider
	// cache directory.
	ProviderCacheDirFlagName = "provider-cache-dir"
	// ProviderCacheHostnameFlagName is the name of the flag that sets the
	// hostname of the provider cache server.
	ProviderCacheHostnameFlagName = "provider-cache-hostname"
	// ProviderCachePortFlagName is the name of the flag that sets the port of
	// the provider cache server.
	ProviderCachePortFlagName = "provider-cache-port"
	// ProviderCacheTokenFlagName is the name of the flag that sets the token
	// clients present to the provider cache server.
	ProviderCacheTokenFlagName = "provider-cache-token"
	// ProviderCacheRegistryNamesFlagName is the name of the flag that lists the
	// registries the provider cache server caches.
	ProviderCacheRegistryNamesFlagName = "provider-cache-registry-names"

	// Engine related environment variables.

	// EngineEnableFlagName is the name of the hidden flag that enables the
	// iac-engine experiment.
	EngineEnableFlagName = "experimental-engine"
	// EngineCachePathFlagName is the name of the hidden flag that sets where
	// IaC engine files are cached.
	EngineCachePathFlagName = "engine-cache-path"
	// EngineSkipCheckFlagName is the name of the hidden flag that skips
	// verifying the checksum of IaC engine files.
	EngineSkipCheckFlagName = "engine-skip-check"
	// EngineLogLevelFlagName is the name of the hidden flag that sets the IaC
	// engine log level.
	EngineLogLevelFlagName = "engine-log-level"

	// Report related flags.

	// SummaryDisableFlagName is the name of the flag that omits the summary
	// printed at the end of a run.
	SummaryDisableFlagName = "summary-disable"
	// ReportFileFlagName is the name of the flag that sets the path of the run
	// report. A .csv or .json extension also sets the report format when
	// --report-format is unset.
	ReportFileFlagName = "report-file"
	// ReportFormatFlagName is the name of the flag that sets the run report
	// format: csv or json.
	ReportFormatFlagName = "report-format"
	// ReportSchemaFlagName is the name of the flag that sets where to write the
	// JSON schema of the run report.
	ReportSchemaFlagName = "report-schema-file"

	// `--all` related flags.

	// OutDirFlagName is the name of the flag that sets the directory plan
	// files are saved to.
	OutDirFlagName = "out-dir"
	// JSONOutDirFlagName is the name of the flag that sets the directory JSON
	// plan files are saved to.
	JSONOutDirFlagName = "json-out-dir"

	// `--graph` related flags.

	// GraphRootFlagName is the name of the flag that sets the directory the
	// dependency graph is built from.
	GraphRootFlagName = "graph-root"

	// Config and download flags - use shared package constants

	// ConfigFlagName is [shared.ConfigFlagName].
	ConfigFlagName = shared.ConfigFlagName

	// Source and auto-init flags - use shared package constants

	// NoAutoInitFlagName is [shared.NoAutoInitFlagName].
	NoAutoInitFlagName = shared.NoAutoInitFlagName
	// SourceFlagName is [shared.SourceFlagName].
	SourceFlagName = shared.SourceFlagName
	// SourceMapFlagName is [shared.SourceMapFlagName].
	SourceMapFlagName = shared.SourceMapFlagName

	// Auth and IAM flags - use shared package constants

	// InputsDebugFlagName is [shared.InputsDebugFlagName].
	InputsDebugFlagName = shared.InputsDebugFlagName
	// IAMAssumeRoleFlagName is [shared.IAMAssumeRoleFlagName].
	IAMAssumeRoleFlagName = shared.IAMAssumeRoleFlagName
	// IAMAssumeRoleDurationFlagName is [shared.IAMAssumeRoleDurationFlagName].
	IAMAssumeRoleDurationFlagName = shared.IAMAssumeRoleDurationFlagName
	// IAMAssumeRoleSessionNameFlagName is [shared.IAMAssumeRoleSessionNameFlagName].
	IAMAssumeRoleSessionNameFlagName = shared.IAMAssumeRoleSessionNameFlagName
	// IAMAssumeRoleWebIdentityTokenFlagName is
	// [shared.IAMAssumeRoleWebIdentityTokenFlagName].
	IAMAssumeRoleWebIdentityTokenFlagName = shared.IAMAssumeRoleWebIdentityTokenFlagName
)

// NewFlags creates and returns global flags.
func NewFlags(l log.Logger, opts *options.TerragruntOptions, v *venv.Venv, prefix flags.Prefix) clihelper.Flags {
	tgPrefix := flags.Prefix{flags.TgPrefix}
	terragruntPrefix := flags.Prefix{flags.TerragruntPrefix}
	cmdFlags := clihelper.Flags{
		// `--all` related flags.
		flags.NewFlag(&clihelper.GenericFlag[string]{
			Name:        OutDirFlagName,
			EnvVars:     tgPrefix.EnvVars(OutDirFlagName),
			Destination: &opts.OutputFolder,
			Usage:       "Directory to store plan files.",
		},
			flags.WithDeprecatedEnvVars(terragruntPrefix.EnvVars("out-dir"), opts.StrictControls)),

		flags.NewFlag(
			&clihelper.GenericFlag[string]{
				Name:        JSONOutDirFlagName,
				EnvVars:     tgPrefix.EnvVars(JSONOutDirFlagName),
				Destination: &opts.JSONOutputFolder,
				Usage:       "Directory to store json plan files.",
			},
			flags.WithDeprecatedEnvVars(
				terragruntPrefix.EnvVars("json-out-dir"),
				opts.StrictControls,
			),
		),

		// `graph/-graph` related flags.

		flags.NewFlag(
			&clihelper.GenericFlag[string]{
				Name:        GraphRootFlagName,
				EnvVars:     tgPrefix.EnvVars(GraphRootFlagName),
				Destination: &opts.GraphRoot,
				Usage:       "Root directory from where to build graph dependencies.",
			},
			flags.WithDeprecatedEnvVars(
				terragruntPrefix.EnvVars("graph-root"),
				opts.StrictControls,
			),
		),

		// `--all` and `--graph` related flags.

		shared.NewNoStackGenerateFlag(opts, prefix),

		//  Backward compatibility with `terragrunt-` prefix flags.

		shared.NewConfigFlag(opts, prefix),

		shared.NewTFPathFlag(opts),

		shared.NewNoAutoInitFlag(opts, prefix),

		flags.NewFlag(&clihelper.BoolFlag{
			Name:        NoAutoRetryFlagName,
			EnvVars:     tgPrefix.EnvVars(NoAutoRetryFlagName),
			Destination: &opts.AutoRetry,
			Usage:       "Don't automatically re-run command in case of transient errors.",
			Negative:    true,
		},
			flags.WithDeprecatedFlag(&clihelper.BoolFlag{
				EnvVars: terragruntPrefix.EnvVars("auto-retry"),
			}, nil, opts.StrictControls)),

		flags.NewFlag(&clihelper.BoolFlag{
			Name:        NoAutoApproveFlagName,
			EnvVars:     tgPrefix.EnvVars(NoAutoApproveFlagName),
			Destination: &opts.RunAllAutoApprove,
			Usage:       "Don't automatically append '-auto-approve' to the underlying OpenTofu/Terraform commands run with 'run --all'.",
			Negative:    true,
		},
			flags.WithDeprecatedFlag(&clihelper.BoolFlag{
				EnvVars: terragruntPrefix.EnvVars("auto-approve"),
			}, nil, opts.StrictControls)),

		flags.NewFlag(&clihelper.BoolFlag{
			Name:        NoAutoProviderCacheDirFlagName,
			EnvVars:     tgPrefix.EnvVars(NoAutoProviderCacheDirFlagName),
			Destination: &opts.NoAutoProviderCacheDir,
			Usage:       "Disable the auto-provider-cache-dir feature even when the experiment is enabled.",
		}),

		flags.NewFlag(&clihelper.BoolFlag{
			Name:        NoHooksFlagName,
			EnvVars:     tgPrefix.EnvVars(NoHooksFlagName),
			Destination: &opts.NoRunHooks,
			Usage:       "Disable Terragrunt hooks during run.",
		}),

		flags.NewFlag(&clihelper.BoolFlag{
			Name:        NoDependencyOutputsFlagName,
			EnvVars:     tgPrefix.EnvVars(NoDependencyOutputsFlagName),
			Destination: &opts.SkipOutput,
			Usage:       "Skip all dependency output resolution. Dependency blocks will not call tofu/terraform output.",
		}),

		shared.NewDownloadDirFlag(opts, prefix),

		shared.NewSourceFlag(opts, prefix),

		flags.NewFlag(
			&clihelper.BoolFlag{
				Name:        SourceUpdateFlagName,
				EnvVars:     tgPrefix.EnvVars(SourceUpdateFlagName),
				Destination: &opts.SourceUpdate,
				Usage:       "Delete the contents of the temporary folder to clear out any old, cached source code before downloading new source code into it.",
			},
			flags.WithDeprecatedEnvVars(
				terragruntPrefix.EnvVars("source-update"),
				opts.StrictControls,
			),
		),

		shared.NewSourceMapFlag(opts, prefix),

		// Assume IAM Role flags.
		shared.NewInputsDebugFlag(opts, prefix),

		flags.NewFlag(
			&clihelper.BoolFlag{
				Name:        UsePartialParseConfigCacheFlagName,
				EnvVars:     tgPrefix.EnvVars(UsePartialParseConfigCacheFlagName),
				Destination: &opts.UsePartialParseConfigCache,
				Usage:       "Enables caching of includes during partial parsing operations. Will also be used for the --iam-role option if provided.",
			},
			flags.WithDeprecatedEnvVars(
				terragruntPrefix.EnvVars("use-partial-parse-config-cache"),
				opts.StrictControls,
			),
		),

		flags.NewFlag(&clihelper.SliceFlag[string]{
			Name:        VersionManagerFileNameFlagName,
			EnvVars:     tgPrefix.EnvVars(VersionManagerFileNameFlagName),
			Destination: &opts.VersionManagerFileName,
			Usage:       "File names used during the computation of the cache key for the version manager files.",
		}),

		flags.NewFlag(
			&clihelper.BoolFlag{
				Name:    DependencyFetchOutputFromStateFlagName,
				EnvVars: tgPrefix.EnvVars(DependencyFetchOutputFromStateFlagName),
				Usage:   "Read dependency outputs directly from the state file. Enabled by default; retained for backwards compatibility.",
				Action: func(ctx context.Context, _ *clihelper.Context, val bool) error {
					if val {
						return opts.StrictControls.FilterByNames(controls.DependencyFetchOutputFromState).
							Evaluate(ctx)
					}

					return nil
				},
			},
			flags.WithDeprecatedEnvVars(
				terragruntPrefix.EnvVars("fetch-dependency-output-from-state"),
				opts.StrictControls,
			),
		),

		flags.NewFlag(&clihelper.BoolFlag{
			Name:        NoDependencyFetchOutputFromStateFlagName,
			EnvVars:     tgPrefix.EnvVars(NoDependencyFetchOutputFromStateFlagName),
			Destination: &opts.NoDependencyFetchOutputFromState,
			Usage:       "Read dependency outputs by running tofu/terraform output instead of reading the state file directly.",
		}),

		flags.NewFlag(
			&clihelper.BoolFlag{
				Name:        TFForwardStdoutFlagName,
				EnvVars:     tgPrefix.EnvVars(TFForwardStdoutFlagName),
				Destination: &opts.ForwardTFStdout,
				Usage:       "If specified, the output of OpenTofu/Terraform commands will be printed as is, without being integrated into the Terragrunt log.",
			},
			flags.WithDeprecatedEnvVars(
				terragruntPrefix.EnvVars("forward-tf-stdout"),
				opts.StrictControls,
			),
			flags.WithDeprecatedEnvVars(
				terragruntPrefix.EnvVars("include-module-prefix"),
				opts.StrictControls,
				controls.LegacyLogs,
			),
		),

		flags.NewFlag(
			&clihelper.SliceFlag[string]{
				Name:    UnitsThatIncludeFlagName,
				EnvVars: tgPrefix.EnvVars(UnitsThatIncludeFlagName),
				Usage:   "If flag is set, 'run --all' will only run the command against Terragrunt modules that include the specified file.",
				Hidden:  true,
				Action: func(ctx context.Context, _ *clihelper.Context, value []string) error {
					if len(value) != 0 {
						if err := opts.StrictControls.FilterByNames(controls.UnitsThatInclude).
							Evaluate(ctx); err != nil {
							return err
						}

						for _, v := range value {
							attrExpr, err := filter.NewAttributeExpression(
								filter.AttributeReading,
								v,
							)
							if err != nil {
								return err
							}

							opts.Filters = append(
								opts.Filters,
								filter.NewFilter(attrExpr, attrExpr.String()),
							)
						}

						return nil
					}

					return nil
				},
			},
			flags.WithDeprecatedEnvVars(
				terragruntPrefix.EnvVars("modules-that-include"),
				opts.StrictControls,
			),
		),

		flags.NewFlag(
			&clihelper.BoolFlag{
				Name:        DisableCommandValidationFlagName,
				EnvVars:     tgPrefix.EnvVars(DisableCommandValidationFlagName),
				Destination: &opts.DisableCommandValidation,
				Usage:       "When this flag is set, Terragrunt will not validate the tofu/terraform command.",
				Hidden:      true,
				Action: func(ctx context.Context, _ *clihelper.Context, value bool) error {
					if value {
						return opts.StrictControls.FilterByNames(controls.DisableCommandValidation).
							Evaluate(ctx)
					}

					return nil
				},
			},
			flags.WithDeprecatedEnvVars(
				terragruntPrefix.EnvVars("disable-command-validation"),
				opts.StrictControls,
			),
		),

		flags.NewFlag(&clihelper.BoolFlag{
			Name:    NoDestroyDependenciesCheckFlagName,
			EnvVars: tgPrefix.EnvVars(NoDestroyDependenciesCheckFlagName),
			Usage:   "When this flag is set, Terragrunt will not check for dependent units when destroying.",
			Hidden:  true,
			Action: func(ctx context.Context, _ *clihelper.Context, value bool) error {
				if value {
					return opts.StrictControls.FilterByNames(controls.NoDestroyDependenciesCheck).
						Evaluate(ctx)
				}

				return nil
			},
		}),

		flags.NewFlag(&clihelper.BoolFlag{
			Name:        DestroyDependenciesCheckFlagName,
			EnvVars:     tgPrefix.EnvVars(DestroyDependenciesCheckFlagName),
			Destination: &opts.DestroyDependenciesCheck,
			Usage:       "When this flag is set, Terragrunt will check for dependent units when destroying.",
		}),

		// Terragrunt Provider Cache flags.

		flags.NewFlag(
			&clihelper.BoolFlag{
				Name:        ProviderCacheFlagName,
				EnvVars:     tgPrefix.EnvVars(ProviderCacheFlagName),
				Destination: &opts.ProviderCacheOptions.Enabled,
				Usage:       "Enables Terragrunt's provider caching.",
			},
			flags.WithDeprecatedEnvVars(
				terragruntPrefix.EnvVars("provider-cache"),
				opts.StrictControls,
			),
		),

		flags.NewFlag(
			&clihelper.GenericFlag[string]{
				Name:        ProviderCacheDirFlagName,
				EnvVars:     tgPrefix.EnvVars(ProviderCacheDirFlagName),
				Destination: &opts.ProviderCacheOptions.Dir,
				Usage:       "The path to the Terragrunt provider cache directory. By default, 'terragrunt/providers' folder in the user cache directory.",
			},
			flags.WithDeprecatedEnvVars(
				terragruntPrefix.EnvVars("provider-cache-dir"),
				opts.StrictControls,
			),
		),

		flags.NewFlag(
			&clihelper.GenericFlag[string]{
				Name:        ProviderCacheTokenFlagName,
				EnvVars:     tgPrefix.EnvVars(ProviderCacheTokenFlagName),
				Destination: &opts.ProviderCacheOptions.Token,
				Usage:       "The token for authentication to the Terragrunt Provider Cache server. By default, assigned automatically.",
			},
			flags.WithDeprecatedEnvVars(
				terragruntPrefix.EnvVars("provider-cache-token"),
				opts.StrictControls,
			),
		),

		flags.NewFlag(
			&clihelper.GenericFlag[string]{
				Name:        ProviderCacheHostnameFlagName,
				EnvVars:     tgPrefix.EnvVars(ProviderCacheHostnameFlagName),
				Destination: &opts.ProviderCacheOptions.Hostname,
				Usage:       "The hostname of the Terragrunt Provider Cache server. By default, 'localhost'.",
			},
			flags.WithDeprecatedEnvVars(
				terragruntPrefix.EnvVars("provider-cache-hostname"),
				opts.StrictControls,
			),
		),

		flags.NewFlag(
			&clihelper.GenericFlag[int]{
				Name:        ProviderCachePortFlagName,
				EnvVars:     tgPrefix.EnvVars(ProviderCachePortFlagName),
				Destination: &opts.ProviderCacheOptions.Port,
				Usage:       "The port of the Terragrunt Provider Cache server. By default, assigned automatically.",
			},
			flags.WithDeprecatedEnvVars(
				terragruntPrefix.EnvVars("provider-cache-port"),
				opts.StrictControls,
			),
		),

		flags.NewFlag(
			&clihelper.SliceFlag[string]{
				Name:        ProviderCacheRegistryNamesFlagName,
				EnvVars:     tgPrefix.EnvVars(ProviderCacheRegistryNamesFlagName),
				Destination: &opts.ProviderCacheOptions.RegistryNames,
				Usage:       "The list of remote registries to cached by Terragrunt Provider Cache server. By default, 'registry.terraform.io', 'registry.opentofu.org'.",
			},
			flags.WithDeprecatedEnvVars(
				terragruntPrefix.EnvVars("provider-cache-registry-names"),
				opts.StrictControls,
			),
		),

		shared.NewAuthProviderCmdFlag(opts, prefix),

		shared.NewNoDiscoveryAuthProviderCmdFlag(opts, prefix),

		// Terragrunt engine flags.

		flags.NewFlag(
			&clihelper.BoolFlag{
				Name:    EngineEnableFlagName,
				EnvVars: tgPrefix.EnvVars(EngineEnableFlagName),
				Usage:   "Enable the iac-engine experiment to use IaC engines.",
				Hidden:  true,
				Action: func(_ context.Context, _ *clihelper.Context, val bool) error {
					if val {
						return opts.Experiments.EnableExperiment(experiment.IacEngine)
					}

					return nil
				},
			},
			flags.WithDeprecatedEnvVars(
				terragruntPrefix.EnvVars("experimental-engine"),
				opts.StrictControls,
			),
		),

		flags.NewFlag(
			&clihelper.GenericFlag[string]{
				Name:        EngineCachePathFlagName,
				EnvVars:     tgPrefix.EnvVars(EngineCachePathFlagName),
				Destination: &opts.EngineOptions.CachePath,
				Usage:       "Cache path for Terragrunt engine files.",
				Hidden:      true,
			},
			flags.WithDeprecatedEnvVars(
				terragruntPrefix.EnvVars("engine-cache-path"),
				opts.StrictControls,
			),
		),

		flags.NewFlag(
			&clihelper.BoolFlag{
				Name:        EngineSkipCheckFlagName,
				EnvVars:     tgPrefix.EnvVars(EngineSkipCheckFlagName),
				Destination: &opts.EngineOptions.SkipChecksumCheck,
				Usage:       "Skip checksum check for Terragrunt engine files.",
				Hidden:      true,
			},
			flags.WithDeprecatedEnvVars(
				terragruntPrefix.EnvVars("engine-skip-check"),
				opts.StrictControls,
			),
		),

		flags.NewFlag(
			&clihelper.GenericFlag[string]{
				Name:        EngineLogLevelFlagName,
				EnvVars:     tgPrefix.EnvVars(EngineLogLevelFlagName),
				Destination: &opts.EngineOptions.LogLevel,
				Usage:       "Terragrunt engine log level.",
				Hidden:      true,
			},
			flags.WithDeprecatedEnvVars(
				terragruntPrefix.EnvVars("engine-log-level"),
				opts.StrictControls,
			),
		),

		flags.NewFlag(&clihelper.BoolFlag{
			Name:        NoEngineFlagName,
			EnvVars:     tgPrefix.EnvVars(NoEngineFlagName),
			Destination: &opts.EngineOptions.NoEngine,
			Usage:       "Disable IaC engines even when the iac-engine experiment is enabled.",
			Hidden:      true,
		}),

		flags.NewFlag(&clihelper.BoolFlag{
			Name:        SummaryDisableFlagName,
			EnvVars:     tgPrefix.EnvVars(SummaryDisableFlagName),
			Destination: &opts.SummaryDisable,
			Usage:       `Disable the summary output at the end of a run.`,
		}),

		flags.NewFlag(&clihelper.BoolFlag{
			Name:        SummaryPerUnitFlagName,
			EnvVars:     tgPrefix.EnvVars(SummaryPerUnitFlagName),
			Destination: &opts.SummaryPerUnit,
			Usage:       `Show duration information for each unit in the summary output.`,
		}),

		flags.NewFlag(&clihelper.GenericFlag[string]{
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

		flags.NewFlag(&clihelper.GenericFlag[string]{
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

		flags.NewFlag(&clihelper.GenericFlag[string]{
			Name:        ReportSchemaFlagName,
			EnvVars:     tgPrefix.EnvVars(ReportSchemaFlagName),
			Usage:       `Path to generate report schema file in.`,
			Destination: &opts.ReportSchemaFile,
		}),
	}

	// Add shared flags
	cmdFlags = cmdFlags.Add(shared.NewBackendFlags(opts, prefix)...)
	cmdFlags = cmdFlags.Add(shared.NewFeatureFlags(opts, prefix)...)
	cmdFlags = cmdFlags.Add(shared.NewFailFastFlag(opts))
	cmdFlags = cmdFlags.Add(shared.NewIAMAssumeRoleFlags(opts, prefix)...)
	cmdFlags = cmdFlags.Add(shared.NewQueueFlags(opts, prefix)...)
	cmdFlags = cmdFlags.Add(shared.NewFilterFlags(l, opts, v)...)
	cmdFlags = cmdFlags.Add(shared.NewParallelismFlag(opts))
	cmdFlags = cmdFlags.Add(shared.NewCASFlags(opts, prefix)...)

	return cmdFlags.Sort()
}
