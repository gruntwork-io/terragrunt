// Package commands provides the implementation of the Terragrunt commands.
package commands

import (
	"fmt"

	"github.com/gruntwork-io/go-commons/collections"
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

const (
	TerragruntConfigFlagName = "terragrunt-config"
	TerragruntConfigEnvName  = "TERRAGRUNT_CONFIG"

	TerragruntTFPathFlagName = "terragrunt-tfpath"
	TerragruntTFPathEnvName  = "TERRAGRUNT_TFPATH"

	TerragruntNoAutoInitFlagName = "terragrunt-no-auto-init"
	TerragruntNoAutoInitEnvName  = "TERRAGRUNT_NO_AUTO_INIT"

	TerragruntNoAutoRetryFlagName = "terragrunt-no-auto-retry"
	TerragruntNoAutoRetryEnvName  = "TERRAGRUNT_NO_AUTO_RETRY"

	TerragruntNoAutoApproveFlagName = "terragrunt-no-auto-approve"
	TerragruntNoAutoApproveEnvName  = "TERRAGRUNT_NO_AUTO_APPROVE"

	TerragruntNonInteractiveFlagName = "terragrunt-non-interactive"
	TerragruntNonInteractiveEnvName  = "TERRAGRUNT_NON_INTERACTIVE"

	TerragruntWorkingDirFlagName = "terragrunt-working-dir"
	TerragruntWorkingDirEnvName  = "TERRAGRUNT_WORKING_DIR"

	TerragruntDownloadDirFlagName = "terragrunt-download-dir"
	TerragruntDownloadDirEnvName  = "TERRAGRUNT_DOWNLOAD"

	TerragruntSourceFlagName = "terragrunt-source"
	TerragruntSourceEnvName  = "TERRAGRUNT_SOURCE"

	TerragruntSourceMapFlagName = "terragrunt-source-map"
	TerragruntSourceMapEnvName  = "TERRAGRUNT_SOURCE_MAP"

	TerragruntSourceUpdateFlagName = "terragrunt-source-update"
	TerragruntSourceUpdateEnvName  = "TERRAGRUNT_SOURCE_UPDATE"

	TerragruntIAMRoleFlagName = "terragrunt-iam-role"
	TerragruntIAMRoleEnvName  = "TERRAGRUNT_IAM_ROLE"

	TerragruntIAMAssumeRoleDurationFlagName = "terragrunt-iam-assume-role-duration"
	TerragruntIAMAssumeRoleDurationEnvName  = "TERRAGRUNT_IAM_ASSUME_ROLE_DURATION"

	TerragruntIAMAssumeRoleSessionNameFlagName = "terragrunt-iam-assume-role-session-name"
	TerragruntIAMAssumeRoleSessionNameEnvName  = "TERRAGRUNT_IAM_ASSUME_ROLE_SESSION_NAME"

	TerragruntIAMWebIdentityTokenFlagName = "terragrunt-iam-web-identity-token"
	TerragruntIAMWebIdentityTokenEnvName  = "TERRAGRUNT_IAM_ASSUME_ROLE_WEB_IDENTITY_TOKEN"

	TerragruntParallelismFlagName = "terragrunt-parallelism"
	TerragruntParallelismEnvName  = "TERRAGRUNT_PARALLELISM"

	TerragruntDebugFlagName = "terragrunt-debug"
	TerragruntDebugEnvName  = "TERRAGRUNT_DEBUG"

	TerragruntModulesThatIncludeFlagName = "terragrunt-modules-that-include"
	TerragruntModulesThatIncludeEnvName  = "TERRAGRUNT_MODULES_THAT_INCLUDE"

	TerragruntFetchDependencyOutputFromStateFlagName = "terragrunt-fetch-dependency-output-from-state"
	TerragruntFetchDependencyOutputFromStateEnvName  = "TERRAGRUNT_FETCH_DEPENDENCY_OUTPUT_FROM_STATE"

	TerragruntUsePartialParseConfigCacheFlagName = "terragrunt-use-partial-parse-config-cache"
	TerragruntUsePartialParseConfigCacheEnvName  = "TERRAGRUNT_USE_PARTIAL_PARSE_CONFIG_CACHE"

	TerragruntFailOnStateBucketCreationFlagName = "terragrunt-fail-on-state-bucket-creation"
	TerragruntFailOnStateBucketCreationEnvName  = "TERRAGRUNT_FAIL_ON_STATE_BUCKET_CREATION"

	TerragruntDisableBucketUpdateFlagName = "terragrunt-disable-bucket-update"
	TerragruntDisableBucketUpdateEnvName  = "TERRAGRUNT_DISABLE_BUCKET_UPDATE"

	TerragruntDisableCommandValidationFlagName = "terragrunt-disable-command-validation"
	TerragruntDisableCommandValidationEnvName  = "TERRAGRUNT_DISABLE_COMMAND_VALIDATION"

	TerragruntAuthProviderCmdFlagName = "terragrunt-auth-provider-cmd"
	TerragruntAuthProviderCmdEnvName  = "TERRAGRUNT_AUTH_PROVIDER_CMD"

	TerragruntOutDirFlagEnvName = "TERRAGRUNT_OUT_DIR"
	TerragruntOutDirFlagName    = "terragrunt-out-dir"

	TerragruntJSONOutDirFlagEnvName = "TERRAGRUNT_JSON_OUT_DIR"
	TerragruntJSONOutDirFlagName    = "terragrunt-json-out-dir"

	TerragruntNoDestroyDependenciesCheckFlagEnvName = "TERRAGRUNT_NO_DESTROY_DEPENDENCIES_CHECK"
	TerragruntNoDestroyDependenciesCheckFlagName    = "terragrunt-no-destroy-dependencies-check"

	// Queue related flags

	TerragruntIgnoreDependencyErrorsFlagName = "terragrunt-ignore-dependency-errors"
	TerragruntIgnoreDependencyErrorsEnvName  = "TERRAGRUNT_IGNORE_DEPENDENCY_ERRORS"

	TerragruntIgnoreDependencyOrderFlagName = "terragrunt-ignore-dependency-order"
	TerragruntIgnoreDependencyOrderEnvName  = "TERRAGRUNT_IGNORE_DEPENDENCY_ORDER"

	TerragruntIgnoreExternalDependenciesFlagName = "terragrunt-ignore-external-dependencies"
	TerragruntIgnoreExternalDependenciesEnvName  = "TERRAGRUNT_IGNORE_EXTERNAL_DEPENDENCIES"

	TerragruntIncludeExternalDependenciesFlagName = "terragrunt-include-external-dependencies"
	TerragruntIncludeExternalDependenciesEnvName  = "TERRAGRUNT_INCLUDE_EXTERNAL_DEPENDENCIES"

	TerragruntExcludesFileFlagName = "terragrunt-excludes-file"
	TerragruntExcludesFileEnvName  = "TERRAGRUNT_EXCLUDES_FILE"

	TerragruntExcludeDirFlagName = "terragrunt-exclude-dir"
	TerragruntExcludeDirEnvName  = "TERRAGRUNT_EXCLUDE_DIR"

	TerragruntIncludeDirFlagName = "terragrunt-include-dir"
	TerragruntIncludeDirEnvName  = "TERRAGRUNT_INCLUDE_DIR"

	TerragruntStrictIncludeFlagName = "terragrunt-strict-include"
	TerragruntStrictIncludeEnvName  = "TERRAGRUNT_STRICT_INCLUDE"

	TerragruntUnitsReadingFlagName = "terragrunt-queue-include-units-reading"
	TerragruntUnitsReadingEnvName  = "TERRAGRUNT_QUEUE_INCLUDE_UNITS_READING"

	// Logs related flags/envs

	TerragruntLogLevelFlagName = "terragrunt-log-level"
	TerragruntLogLevelEnvName  = "TERRAGRUNT_LOG_LEVEL"

	TerragruntLogDisableFlagName = "terragrunt-log-disable"
	TerragruntLogDisableEnvName  = "TERRAGRUNT_LOG_DISABLE"

	TerragruntNoColorFlagName = "terragrunt-no-color"
	TerragruntNoColorEnvName  = "TERRAGRUNT_NO_COLOR"

	TerragruntShowLogAbsPathsFlagName = "terragrunt-log-show-abs-paths"
	TerragruntShowLogAbsPathsEnvName  = "TERRAGRUNT_LOG_SHOW_ABS_PATHS"

	TerragruntForwardTFStdoutFlagName = "terragrunt-forward-tf-stdout"
	TerragruntForwardTFStdoutEnvName  = "TERRAGRUNT_FORWARD_TF_STDOUT"

	TerragruntLogFormatFlagName = "terragrunt-log-format"
	TerragruntLogFormatEnvName  = "TERRAGRUNT_LOG_FORMAT"

	TerragruntLogCustomFormatFlagName = "terragrunt-log-custom-format"
	TerragruntLogCustomFormatEnvName  = "TERRAGRUNT_LOG_CUSTOM_FORMAT"

	TerragruntLogDisableErrorSummaryFlagName = "terragrunt-log-disable-error-summary"
	TerragruntLogDisableErrorSummaryEnvName  = "TERRAGRUNT_LOG_DISABLE_ERROR_SUMMARY"

	// Strict Mode related flags/envs
	TerragruntStrictModeFlagName = "strict-mode"
	TerragruntStrictModeEnvName  = "TERRAGRUNT_STRICT_MODE"

	TerragruntStrictControlFlagName = "strict-control"
	TerragruntStrictControlEnvName  = "TERRAGRUNT_STRICT_CONTROL"

	// Experiment Mode related flags/envs
	TerragruntExperimentModeFlagName = "experiment-mode"
	TerragruntExperimentModeEnvName  = "TERRAGRUNT_EXPERIMENT_MODE"

	TerragruntExperimentFlagName = "experiment"
	TerragruntExperimentEnvName  = "TERRAGRUNT_EXPERIMENT"

	// Terragrunt Provider Cache related flags/envs

	TerragruntProviderCacheFlagName = "terragrunt-provider-cache"
	TerragruntProviderCacheEnvName  = "TERRAGRUNT_PROVIDER_CACHE"

	TerragruntProviderCacheDirFlagName = "terragrunt-provider-cache-dir"
	TerragruntProviderCacheDirEnvName  = "TERRAGRUNT_PROVIDER_CACHE_DIR"

	TerragruntProviderCacheHostnameFlagName = "terragrunt-provider-cache-hostname"
	TerragruntProviderCacheHostnameEnvName  = "TERRAGRUNT_PROVIDER_CACHE_HOSTNAME"

	TerragruntProviderCachePortFlagName = "terragrunt-provider-cache-port"
	TerragruntProviderCachePortEnvName  = "TERRAGRUNT_PROVIDER_CACHE_PORT"

	TerragruntProviderCacheTokenFlagName = "terragrunt-provider-cache-token"
	TerragruntProviderCacheTokenEnvName  = "TERRAGRUNT_PROVIDER_CACHE_TOKEN"

	TerragruntProviderCacheRegistryNamesFlagName = "terragrunt-provider-cache-registry-names"
	TerragruntProviderCacheRegistryNamesEnvName  = "TERRAGRUNT_PROVIDER_CACHE_REGISTRY_NAMES"

	TerragruntFeatureMapFlagName = "feature"
	TerragruntFeatureMapEnvName  = "TERRAGRUNT_FEATURE"

	// Engine related environment variables.

	TerragruntEngineEnableEnvName = "TG_EXPERIMENTAL_ENGINE"
	TerragruntEngineCachePathEnv  = "TG_ENGINE_CACHE_PATH"
	TerragruntEngineSkipCheckEnv  = "TG_ENGINE_SKIP_CHECK"
	TerragruntEngineLogLevelEnv   = "TG_ENGINE_LOG_LEVEL"

	HelpFlagName    = "help"
	VersionFlagName = "version"
)

// NewGlobalFlags creates and returns global flags.
func NewGlobalFlags(opts *options.TerragruntOptions) cli.Flags {
	flags := cli.Flags{
		&cli.GenericFlag[string]{
			Name:        TerragruntConfigFlagName,
			EnvVar:      TerragruntConfigEnvName,
			Destination: &opts.TerragruntConfigPath,
			Usage:       "The path to the Terragrunt config file. Default is terragrunt.hcl.",
		},
		&cli.GenericFlag[string]{
			Name:        TerragruntTFPathFlagName,
			EnvVar:      TerragruntTFPathEnvName,
			Destination: &opts.TerraformPath,
			Usage:       "Path to the Terraform binary. Default is tofu (on PATH).",
		},
		&cli.BoolFlag{
			Name:        TerragruntNoAutoInitFlagName,
			EnvVar:      TerragruntNoAutoInitEnvName,
			Usage:       "Don't automatically run 'terraform/tofu init' during other terragrunt commands. You must run 'terragrunt init' manually.",
			Negative:    true,
			Destination: &opts.AutoInit,
		},
		&cli.BoolFlag{
			Name:        TerragruntNoAutoRetryFlagName,
			EnvVar:      TerragruntNoAutoRetryEnvName,
			Destination: &opts.AutoRetry,
			Usage:       "Don't automatically re-run command in case of transient errors.",
			Negative:    true,
		},
		&cli.BoolFlag{
			Name:        TerragruntNoAutoApproveFlagName,
			EnvVar:      TerragruntNoAutoApproveEnvName,
			Destination: &opts.RunAllAutoApprove,
			Usage:       "Don't automatically append `-auto-approve` to the underlying OpenTofu/Terraform commands run with 'run-all'.",
			Negative:    true,
		},
		&cli.BoolFlag{
			Name:        TerragruntNonInteractiveFlagName,
			EnvVar:      TerragruntNonInteractiveEnvName,
			Destination: &opts.NonInteractive,
			Usage:       `Assume "yes" for all prompts.`,
		},
		&cli.GenericFlag[string]{
			Name:        TerragruntWorkingDirFlagName,
			EnvVar:      TerragruntWorkingDirEnvName,
			Destination: &opts.WorkingDir,
			Usage:       "The path to the directory of Terragrunt configurations. Default is current directory.",
		},
		&cli.GenericFlag[string]{
			Name:        TerragruntDownloadDirFlagName,
			EnvVar:      TerragruntDownloadDirEnvName,
			Destination: &opts.DownloadDir,
			Usage:       "The path to download OpenTofu/Terraform modules into. Default is .terragrunt-cache in the working directory.",
		},
		&cli.GenericFlag[string]{
			Name:        TerragruntSourceFlagName,
			EnvVar:      TerragruntSourceEnvName,
			Destination: &opts.Source,
			Usage:       "Download OpenTofu/Terraform configurations from the specified source into a temporary folder, and run Terraform in that temporary folder.",
		},
		&cli.BoolFlag{
			Name:        TerragruntSourceUpdateFlagName,
			EnvVar:      TerragruntSourceUpdateEnvName,
			Destination: &opts.SourceUpdate,
			Usage:       "Delete the contents of the temporary folder to clear out any old, cached source code before downloading new source code into it.",
		},
		&cli.MapFlag[string, string]{
			Name:        TerragruntSourceMapFlagName,
			EnvVar:      TerragruntSourceMapEnvName,
			Destination: &opts.SourceMap,
			Usage:       "Replace any source URL (including the source URL of a config pulled in with dependency blocks) that has root source with dest.",
			Splitter:    util.SplitUrls,
		},
		&cli.GenericFlag[string]{
			Name:        TerragruntIAMRoleFlagName,
			EnvVar:      TerragruntIAMRoleEnvName,
			Destination: &opts.IAMRoleOptions.RoleARN,
			Usage:       "Assume the specified IAM role before executing OpenTofu/Terraform. Can also be set via the TERRAGRUNT_IAM_ROLE environment variable.",
		},
		&cli.GenericFlag[int64]{
			Name:        TerragruntIAMAssumeRoleDurationFlagName,
			EnvVar:      TerragruntIAMAssumeRoleDurationEnvName,
			Destination: &opts.IAMRoleOptions.AssumeRoleDuration,
			Usage:       "Session duration for IAM Assume Role session. Can also be set via the TERRAGRUNT_IAM_ASSUME_ROLE_DURATION environment variable.",
		},
		&cli.GenericFlag[string]{
			Name:        TerragruntIAMAssumeRoleSessionNameFlagName,
			EnvVar:      TerragruntIAMAssumeRoleSessionNameEnvName,
			Destination: &opts.IAMRoleOptions.AssumeRoleSessionName,
			Usage:       "Name for the IAM Assumed Role session. Can also be set via TERRAGRUNT_IAM_ASSUME_ROLE_SESSION_NAME environment variable.",
		},
		&cli.GenericFlag[string]{
			Name:        TerragruntIAMWebIdentityTokenFlagName,
			EnvVar:      TerragruntIAMWebIdentityTokenEnvName,
			Destination: &opts.IAMRoleOptions.WebIdentityToken,
			Usage:       "For AssumeRoleWithWebIdentity, the WebIdentity token. Can also be set via TERRAGRUNT_IAM_ASSUME_ROLE_WEB_IDENTITY_TOKEN environment variable",
		},
		&cli.BoolFlag{
			Name:        TerragruntIgnoreDependencyErrorsFlagName,
			EnvVar:      TerragruntIgnoreDependencyErrorsEnvName,
			Destination: &opts.IgnoreDependencyErrors,
			Usage:       "*-all commands continue processing components even if a dependency fails.",
		},
		&cli.BoolFlag{
			Name:        TerragruntIgnoreDependencyOrderFlagName,
			EnvVar:      TerragruntIgnoreDependencyOrderEnvName,
			Destination: &opts.IgnoreDependencyOrder,
			Usage:       "*-all commands will be run disregarding the dependencies",
		},
		&cli.BoolFlag{
			Name:        TerragruntIgnoreExternalDependenciesFlagName,
			EnvVar:      TerragruntIgnoreExternalDependenciesEnvName,
			Destination: &opts.IgnoreExternalDependencies,
			Usage:       "*-all commands will not attempt to include external dependencies",
		},
		&cli.BoolFlag{
			Name:        TerragruntIncludeExternalDependenciesFlagName,
			EnvVar:      TerragruntIncludeExternalDependenciesEnvName,
			Destination: &opts.IncludeExternalDependencies,
			Usage:       "*-all commands will include external dependencies",
		},
		&cli.GenericFlag[int]{
			Name:        TerragruntParallelismFlagName,
			EnvVar:      TerragruntParallelismEnvName,
			Destination: &opts.Parallelism,
			Usage:       "*-all commands parallelism set to at most N modules",
		},
		&cli.GenericFlag[string]{
			Name:        TerragruntExcludesFileFlagName,
			EnvVar:      TerragruntExcludesFileEnvName,
			Destination: &opts.ExcludesFile,
			Usage:       "Path to a file with a list of directories that need to be excluded when running *-all commands.",
		},
		&cli.SliceFlag[string]{
			Name:        TerragruntExcludeDirFlagName,
			EnvVar:      TerragruntExcludeDirEnvName,
			Destination: &opts.ExcludeDirs,
			Usage:       "Unix-style glob of directories to exclude when running *-all commands.",
		},
		&cli.SliceFlag[string]{
			Name:        TerragruntIncludeDirFlagName,
			EnvVar:      TerragruntIncludeDirEnvName,
			Destination: &opts.IncludeDirs,
			Usage:       "Unix-style glob of directories to include when running *-all commands",
		},
		&cli.BoolFlag{
			Name:        TerragruntDebugFlagName,
			EnvVar:      TerragruntDebugEnvName,
			Destination: &opts.Debug,
			Usage:       "Write terragrunt-debug.tfvars to working folder to help root-cause issues.",
		},
		&cli.GenericFlag[string]{
			Name:        TerragruntLogLevelFlagName,
			EnvVar:      TerragruntLogLevelEnvName,
			DefaultText: opts.LogLevel.String(),
			Usage:       fmt.Sprintf("Sets the logging level for Terragrunt. Supported levels: %s", log.AllLevels),
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
					return cli.NewExitError(errors.Errorf("flag --%s, %w", TerragruntLogLevelFlagName, err), 1)
				}

				opts.Logger.SetOptions(log.WithLevel(level))
				opts.LogLevel = level
				return nil
			},
		},
		&cli.BoolFlag{
			Name:        TerragruntLogDisableFlagName,
			EnvVar:      TerragruntLogDisableEnvName,
			Usage:       "Disable logging",
			Destination: &opts.DisableLog,
			Action: func(_ *cli.Context, _ bool) error {
				opts.ForwardTFStdout = true
				opts.LogFormatter.SetFormat(nil)
				return nil
			},
		},
		&cli.BoolFlag{
			Name:        TerragruntShowLogAbsPathsFlagName,
			EnvVar:      TerragruntShowLogAbsPathsEnvName,
			Destination: &opts.LogShowAbsPaths,
			Usage:       "Show absolute paths in logs",
		},
		&cli.BoolFlag{
			Name:        TerragruntNoColorFlagName,
			EnvVar:      TerragruntNoColorEnvName,
			Destination: &opts.DisableLogColors,
			Usage:       "If specified, Terragrunt output won't contain any color.",
			Action: func(_ *cli.Context, _ bool) error {
				opts.LogFormatter.DisableColors()
				return nil
			},
		},
		&cli.BoolFlag{
			Name:        TerragruntUsePartialParseConfigCacheFlagName,
			EnvVar:      TerragruntUsePartialParseConfigCacheEnvName,
			Destination: &opts.UsePartialParseConfigCache,
			Usage:       "Enables caching of includes during partial parsing operations. Will also be used for the --terragrunt-iam-role option if provided.",
		},
		&cli.BoolFlag{
			Name:        TerragruntFetchDependencyOutputFromStateFlagName,
			EnvVar:      TerragruntFetchDependencyOutputFromStateEnvName,
			Destination: &opts.FetchDependencyOutputFromState,
			Usage:       "The option fetches dependency output directly from the state file instead of init dependencies and running terraform on them.",
		},
		&cli.BoolFlag{
			Name:        TerragruntForwardTFStdoutFlagName,
			EnvVar:      TerragruntForwardTFStdoutEnvName,
			Destination: &opts.ForwardTFStdout,
			Usage:       "If specified, the output of OpenTofu/Terraform commands will be printed as is, without being integrated into the Terragrunt log.",
		},
		&cli.GenericFlag[string]{
			Name:   TerragruntLogFormatFlagName,
			EnvVar: TerragruntLogFormatEnvName,
			Usage:  "Set the log format",
			Action: func(_ *cli.Context, val string) error {
				phs, err := format.ParseFormat(val)
				if err != nil {
					return cli.NewExitError(errors.Errorf("flag --%s, invalid format %q, %v", TerragruntLogFormatFlagName, val, err), 1)
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
		},
		&cli.GenericFlag[string]{
			Name:   TerragruntLogCustomFormatFlagName,
			EnvVar: TerragruntLogCustomFormatEnvName,
			Usage:  "Set the custom log formatting",
			Action: func(_ *cli.Context, val string) error {
				phs, err := placeholders.Parse(val)
				if err != nil {
					return cli.NewExitError(errors.Errorf("flag --%s, %w", TerragruntLogCustomFormatFlagName, err), 1)
				}

				opts.LogFormatter.SetFormat(phs)

				return nil
			},
		},
		&cli.BoolFlag{
			Name:        TerragruntStrictIncludeFlagName,
			EnvVar:      TerragruntStrictIncludeEnvName,
			Destination: &opts.StrictInclude,
			Usage:       "If flag is set, only modules under the directories passed in with '--terragrunt-include-dir' will be included.",
		},
		&cli.SliceFlag[string]{
			Name:        TerragruntModulesThatIncludeFlagName,
			EnvVar:      TerragruntModulesThatIncludeEnvName,
			Destination: &opts.ModulesThatInclude,
			Usage:       "If flag is set, 'run-all' will only run the command against Terragrunt modules that include the specified file.",
		},
		&cli.SliceFlag[string]{
			Name:        TerragruntUnitsReadingFlagName,
			EnvVar:      TerragruntUnitsReadingEnvName,
			Destination: &opts.UnitsReading,
			Usage:       "If flag is set, 'run-all' will only run the command against Terragrunt units that read the specified file via an HCL function.",
		},
		&cli.BoolFlag{
			Name:        TerragruntFailOnStateBucketCreationFlagName,
			EnvVar:      TerragruntFailOnStateBucketCreationEnvName,
			Destination: &opts.FailIfBucketCreationRequired,
			Usage:       "When this flag is set Terragrunt will fail if the remote state bucket needs to be created.",
		},
		&cli.BoolFlag{
			Name:        TerragruntDisableBucketUpdateFlagName,
			EnvVar:      TerragruntDisableBucketUpdateEnvName,
			Destination: &opts.DisableBucketUpdate,
			Usage:       "When this flag is set Terragrunt will not update the remote state bucket.",
		},
		&cli.BoolFlag{
			Name:        TerragruntDisableCommandValidationFlagName,
			EnvVar:      TerragruntDisableCommandValidationEnvName,
			Destination: &opts.DisableCommandValidation,
			Usage:       "When this flag is set, Terragrunt will not validate the terraform command.",
		},
		&cli.BoolFlag{
			Name:        TerragruntNoDestroyDependenciesCheckFlagName,
			EnvVar:      TerragruntNoDestroyDependenciesCheckFlagEnvName,
			Destination: &opts.NoDestroyDependenciesCheck,
			Usage:       "When this flag is set, Terragrunt will not check for dependent modules when destroying.",
		},
		// Strict Mode flags
		&cli.BoolFlag{
			Name:        TerragruntStrictModeFlagName,
			EnvVar:      TerragruntStrictModeEnvName,
			Destination: &opts.StrictMode,
			Usage:       "Enables strict mode for Terragrunt. For more information, see https://terragrunt.gruntwork.io/docs/reference/strict-mode .",
		},
		&cli.SliceFlag[string]{
			Name:        TerragruntStrictControlFlagName,
			EnvVar:      TerragruntStrictControlEnvName,
			Destination: &opts.StrictControls,
			Usage:       "Enables specific strict controls. For a list of available controls, see https://terragrunt.gruntwork.io/docs/reference/strict-mode .",
			Action: func(ctx *cli.Context, val []string) error {
				warning, err := strict.StrictControls.ValidateControlNames(val)
				if err != nil {
					return cli.NewExitError(err, 1)
				}

				if warning != "" {
					log.Warn(warning)
				}

				return nil
			},
		},
		// Experiment Mode flags
		&cli.BoolFlag{
			Name:        TerragruntExperimentModeFlagName,
			EnvVar:      TerragruntExperimentModeEnvName,
			Destination: &opts.ExperimentMode,
			Usage:       "Enables experiment mode for Terragrunt. For more information, see https://terragrunt.gruntwork.io/docs/reference/experiment-mode .",
		},
		&cli.SliceFlag[string]{
			Name:   TerragruntExperimentFlagName,
			EnvVar: TerragruntExperimentEnvName,
			Usage:  "Enables specific experiments. For a list of available experiments, see https://terragrunt.gruntwork.io/docs/reference/experiment-mode .",
			Action: func(ctx *cli.Context, val []string) error {
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
		// Terragrunt Provider Cache flags
		&cli.BoolFlag{
			Name:        TerragruntProviderCacheFlagName,
			Destination: &opts.ProviderCache,
			EnvVar:      TerragruntProviderCacheEnvName,
			Usage:       "Enables Terragrunt's provider caching.",
		},
		&cli.GenericFlag[string]{
			Name:        TerragruntProviderCacheDirFlagName,
			Destination: &opts.ProviderCacheDir,
			EnvVar:      TerragruntProviderCacheDirEnvName,
			Usage:       "The path to the Terragrunt provider cache directory. By default, 'terragrunt/providers' folder in the user cache directory.",
		},
		&cli.GenericFlag[string]{
			Name:        TerragruntProviderCacheTokenFlagName,
			Destination: &opts.ProviderCacheToken,
			EnvVar:      TerragruntProviderCacheTokenEnvName,
			Usage:       "The Token for authentication to the Terragrunt Provider Cache server. By default, assigned automatically.",
		},
		&cli.GenericFlag[string]{
			Name:        TerragruntProviderCacheHostnameFlagName,
			Destination: &opts.ProviderCacheHostname,
			EnvVar:      TerragruntProviderCacheHostnameEnvName,
			Usage:       "The hostname of the Terragrunt Provider Cache server. By default, 'localhost'.",
		},
		&cli.GenericFlag[int]{
			Name:        TerragruntProviderCachePortFlagName,
			Destination: &opts.ProviderCachePort,
			EnvVar:      TerragruntProviderCachePortEnvName,
			Usage:       "The port of the Terragrunt Provider Cache server. By default, assigned automatically.",
		},
		&cli.SliceFlag[string]{
			Name:        TerragruntProviderCacheRegistryNamesFlagName,
			Destination: &opts.ProviderCacheRegistryNames,
			EnvVar:      TerragruntProviderCacheRegistryNamesEnvName,
			Usage:       "The list of remote registries to cached by Terragrunt Provider Cache server. By default, 'registry.terraform.io', 'registry.opentofu.org'.",
		},
		&cli.GenericFlag[string]{
			Name:        TerragruntAuthProviderCmdFlagName,
			Destination: &opts.AuthProviderCmd,
			EnvVar:      TerragruntAuthProviderCmdEnvName,
			Usage:       "The command and arguments that can be used to fetch authentication configurations.",
		},
		&cli.MapFlag[string, string]{
			Name:     TerragruntFeatureMapFlagName,
			EnvVar:   TerragruntFeatureMapEnvName,
			Usage:    "Set feature flags for the HCL code.",
			Splitter: util.SplitComma,
			Action: func(_ *cli.Context, value map[string]string) error {
				for key, val := range value {
					opts.FeatureFlags.Store(key, val)
				}

				return nil
			},
		},
		// Terragrunt engine flags
		&cli.BoolFlag{
			Name:        TerragruntEngineEnableEnvName,
			EnvVar:      TerragruntEngineEnableEnvName,
			Destination: &opts.EngineEnabled,
			Usage:       "Enable Terragrunt experimental engine.",
			Hidden:      true,
		},
		&cli.GenericFlag[string]{
			Name:        TerragruntEngineCachePathEnv,
			EnvVar:      TerragruntEngineCachePathEnv,
			Destination: &opts.EngineCachePath,
			Usage:       "Cache path for Terragrunt engine files.",
			Hidden:      true,
		},
		&cli.BoolFlag{
			Name:        TerragruntEngineSkipCheckEnv,
			EnvVar:      TerragruntEngineSkipCheckEnv,
			Destination: &opts.EngineSkipChecksumCheck,
			Usage:       "Skip checksum check for Terragrunt engine files.",
			Hidden:      true,
		},
		&cli.GenericFlag[string]{
			Name:        TerragruntEngineLogLevelEnv,
			EnvVar:      TerragruntEngineLogLevelEnv,
			Destination: &opts.EngineLogLevel,
			Usage:       "Terragrunt engine log level.",
			Hidden:      true,
		},
		&cli.BoolFlag{
			Name:        TerragruntLogDisableErrorSummaryFlagName,
			EnvVar:      TerragruntLogDisableErrorSummaryEnvName,
			Destination: &opts.LogDisableErrorSummary,
			Usage:       "Skip error summary at the end of the command.",
		},
	}

	flags.Sort()
	flags.Add(NewHelpVersionFlags(opts)...)

	return flags
}

func NewHelpVersionFlags(opts *options.TerragruntOptions) cli.Flags {
	return cli.Flags{
		NewHelpFlag(opts),
		NewVersionFlag(opts),
	}
}

func NewHelpFlag(opts *options.TerragruntOptions) cli.Flag {
	return &cli.BoolFlag{
		Name:    HelpFlagName,  // --help, -help
		Aliases: []string{"h"}, //  -h
		Usage:   "Show help",
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
		Usage:   "Show terragrunt version",
		Action: func(ctx *cli.Context, _ bool) (err error) {
			defer func() {
				// exit the app
				err = cli.NewExitError(err, 0)
			}()

			return cli.ShowVersion(ctx)
		},
	}
}

// Scaffold/Catalog shared flags

const (
	LegacyParentConfigName      = "terragrunt.hcl"
	RecommendedParentConfigName = "root.hcl"

	RootFileNameFlagName  = "root-file-name"
	NoIncludeRootFlagName = "no-include-root"
)

func GetDefaultRootFileName(opts *options.TerragruntOptions) string {
	if control, ok := strict.GetStrictControl(strict.RootTerragruntHCL); ok {
		warn, triggered, err := control.Evaluate(opts)
		if err != nil {
			return RecommendedParentConfigName
		}

		if !triggered {
			opts.Logger.Warnf(warn)
		}
	}

	return LegacyParentConfigName
}

func NewRootFileNameFlag(opts *options.TerragruntOptions) cli.Flag {
	return &cli.GenericFlag[string]{
		Name:        RootFileNameFlagName,
		Destination: &opts.ScaffoldRootFileName,
		Usage:       "Name of the root Terragrunt configuration file, if used.",
		Action: func(ctx *cli.Context, value string) error {
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
