// Package commands provides the implementation of the Terragrunt commands.
// This file contains the definition of the global flags used by Terragrunt.
package commands

import (
	"errors"
	"fmt"

	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/pkg/cli"
	"github.com/gruntwork-io/terragrunt/shell"
	"github.com/gruntwork-io/terragrunt/util"
)

const (
	// TerragruntConfigFlagName is the name of the flag that specifies
	// the path to the Terragrunt config file.
	TerragruntConfigFlagName = "terragrunt-config"

	// TerragruntTFPathFlagName is the name of the flag that specifies
	// the path to the Terraform binary.
	TerragruntTFPathFlagName = "terragrunt-tfpath"

	// TerragruntNoAutoInitFlagName is the name of the flag that specifies
	// whether Terragrunt should automatically run.
	TerragruntNoAutoInitFlagName = "terragrunt-no-auto-init"

	// TerragruntNoAutoRetryFlagName is the name of the flag that specifies
	// whether Terragrunt should automatically retry.
	TerragruntNoAutoRetryFlagName = "terragrunt-no-auto-retry"

	// TerragruntNoAutoApproveFlagName is the name of the flag that specifies
	// whether Terragrunt should automatically approve.
	TerragruntNoAutoApproveFlagName = "terragrunt-no-auto-approve"

	// TerragruntNonInteractiveFlagName is the name of the flag that specifies
	// whether Terragrunt should run in non-interactive mode.
	TerragruntNonInteractiveFlagName = "terragrunt-non-interactive"

	// TerragruntWorkingDirFlagName is the name of the flag that specifies
	// the path to working directory where Terragrunt should run.
	TerragruntWorkingDirFlagName = "terragrunt-working-dir"

	// TerragruntDownloadDirFlagName is the name of the flag that specifies
	// the path to download modules to.
	TerragruntDownloadDirFlagName = "terragrunt-download-dir"

	// TerragruntSourceFlagName is the name of the flag that specifies
	// the source URL to download OpenTofu/Terraform configurations from.
	TerragruntSourceFlagName = "terragrunt-source"

	// TerragruntSourceMapFlagName is the name of the flag that specifies
	// how Terragrunt should map source URLs to destination URLs.
	TerragruntSourceMapFlagName = "terragrunt-source-map"

	// TerragruntSourceUpdateFlagName is the name of the flag that specifies
	// whether Terragrunt should delete the contents of the temporary folder
	// before downloading new source code into it.
	TerragruntSourceUpdateFlagName = "terragrunt-source-update"

	// TerragruntIAMRoleFlagName is the name of the flag that specifies
	// the IAM role to assume before running OpenTofu/Terraform.
	TerragruntIAMRoleFlagName = "terragrunt-iam-role"

	// TerragruntIAMAssumeRoleDurationFlagName is the name of the flag that specifies
	// the duration of the IAM role session.
	TerragruntIAMAssumeRoleDurationFlagName = "terragrunt-iam-assume-role-duration"

	// TerragruntIAMAssumeRoleSessionNameFlagName is the name of the flag that specifies
	// the name of the IAM role session.
	TerragruntIAMAssumeRoleSessionNameFlagName = "terragrunt-iam-assume-role-session-name"

	// TerragruntIAMWebIdentityTokenFlagName is the name of the flag that specifies
	// the WebIdentity token for AssumeRoleWithWebIdentity.
	TerragruntIAMWebIdentityTokenFlagName = "terragrunt-iam-web-identity-token"

	// TerragruntIgnoreDependencyErrorsFlagName is the name of the flag that specifies
	// whether Terragrunt should ignore dependency errors.
	TerragruntIgnoreDependencyErrorsFlagName = "terragrunt-ignore-dependency-errors"

	// TerragruntIgnoreDependencyOrderFlagName is the name of the flag that specifies
	// whether Terragrunt should ignore dependency order.
	TerragruntIgnoreDependencyOrderFlagName = "terragrunt-ignore-dependency-order"

	// TerragruntIgnoreExternalDependenciesFlagName is the name of the flag that specifies
	// whether Terragrunt should ignore external dependencies.
	TerragruntIgnoreExternalDependenciesFlagName = "terragrunt-ignore-external-dependencies"

	// TerragruntIncludeExternalDependenciesFlagName is the name of the flag that specifies
	// whether Terragrunt should include external dependencies.
	TerragruntIncludeExternalDependenciesFlagName = "terragrunt-include-external-dependencies"

	// TerragruntExcludesFile is the name of the flag that specifies
	// the path to a file with a list of directories that need to be excluded when running *-all commands.
	TerragruntExcludesFile = "terragrunt-excludes-file"

	// TerragruntExcludeDirFlagName is the name of the flag that specifies
	// the Unix-style glob of directories to exclude when running *-all commands.
	TerragruntExcludeDirFlagName = "terragrunt-exclude-dir"

	// TerragruntIncludeDirFlagName is the name of the flag that specifies
	// the Unix-style glob of directories to include when running *-all commands.
	TerragruntIncludeDirFlagName = "terragrunt-include-dir"

	// TerragruntStrictIncludeFlagName is the name of the flag that specifies
	// whether Terragrunt should only include modules under the directories passed in with '--terragrunt-include-dir'.
	TerragruntStrictIncludeFlagName = "terragrunt-strict-include"

	// TerragruntParallelismFlagName is the name of the flag that specifies
	// the parallelism level for the *-all commands.
	TerragruntParallelismFlagName = "terragrunt-parallelism"

	// TerragruntDebugFlagName is the name of the flag that specifies
	// whether Terragrunt should write terragrunt-debug.tfvars to the working folder.
	TerragruntDebugFlagName = "terragrunt-debug"

	// TerragruntLogLevelFlagName is the name of the flag that specifies
	// the logging level for Terragrunt.
	TerragruntLogLevelFlagName = "terragrunt-log-level"

	// TerragruntNoColorFlagName is the name of the flag that specifies
	// whether Terragrunt output should contain any color.
	TerragruntNoColorFlagName = "terragrunt-no-color"

	// TerragruntJSONLogFlagName is the name of the flag that specifies
	// whether Terragrunt should output its logs in JSON format.
	TerragruntJSONLogFlagName = "terragrunt-json-log"

	// TerragruntTfLogJSONFlagName is the name of the flag that specifies
	// whether Terragrunt should wrap Terraform stdout and stderr in JSON.
	TerragruntTfLogJSONFlagName = "terragrunt-tf-logs-to-json"

	// TerragruntModulesThatIncludeFlagName is the name of the flag that specifies
	// that 'run-all' should only run the command against modules that include the specified file.
	TerragruntModulesThatIncludeFlagName = "terragrunt-modules-that-include"

	// TerragruntFetchDependencyOutputFromStateFlagName is the name of the flag that specifies
	// that the option fetchs dependency output directly from the state file instead of init-ing
	// dependencies and running tofu/terraform output on them.
	TerragruntFetchDependencyOutputFromStateFlagName = "terragrunt-fetch-dependency-output-from-state"

	// TerragruntUsePartialParseConfigCacheFlagName is the name of the flag that specifies
	// whether Terragrunt should cache includes during partial parsing operations.
	TerragruntUsePartialParseConfigCacheFlagName = "terragrunt-use-partial-parse-config-cache"

	// TerragruntIncludeModulePrefixFlagName is the name of the flag that specifies
	// whether output from Terraform sub-commands should be prefixed with module path.
	TerragruntIncludeModulePrefixFlagName = "terragrunt-include-module-prefix"

	// TerragruntFailOnStateBucketCreationFlagName is the name of the flag that specifies
	// whether Terragrunt should fail if the remote state bucket needs to be created.
	TerragruntFailOnStateBucketCreationFlagName = "terragrunt-fail-on-state-bucket-creation"

	// TerragruntDisableBucketUpdateFlagName is the name of the flag that specifies
	// whether Terragrunt should update the remote state bucket.
	TerragruntDisableBucketUpdateFlagName = "terragrunt-disable-bucket-update"

	// TerragruntDisableCommandValidationFlagName is the name of the flag that specifies
	// whether Terragrunt should validate the terraform command.
	TerragruntDisableCommandValidationFlagName = "terragrunt-disable-command-validation"

	// TerragruntAuthProviderCmdFlagName is the name of the flag that specifies
	// the command and arguments that can be used to fetch authentication configurations.
	TerragruntAuthProviderCmdFlagName = "terragrunt-auth-provider-cmd"
	// TerragruntAuthProviderCmdEnvVarName is the name of the environment variable that specifies
	// the command and arguments that can be used to fetch authentication configurations.
	TerragruntAuthProviderCmdEnvVarName = "TERRAGRUNT_AUTH_PROVIDER_CMD"

	// TerragruntOutDirFlagEnvVarName is the name of the environment variable that specifies
	// the path to the output directory.
	TerragruntOutDirFlagEnvVarName = "TERRAGRUNT_OUT_DIR"
	// TerragruntOutDirFlagName is the name of the flag that specifies
	// the path to the output directory.
	TerragruntOutDirFlagName = "terragrunt-out-dir"

	// TerragruntJSONOutDirFlagEnvVarName is the name of the environment variable that specifies
	// the path to the JSON output directory.
	TerragruntJSONOutDirFlagEnvVarName = "TERRAGRUNT_JSON_OUT_DIR"
	// TerragruntJSONOutDirFlagName is the name of the flag that specifies
	// the path to the JSON output directory.
	TerragruntJSONOutDirFlagName = "terragrunt-json-out-dir"

	// Terragrunt Provider Cache flags/envs.

	// TerragruntProviderCacheFlagName is the name of the flag that specifies
	// whether Terragrunt should enable provider caching.
	TerragruntProviderCacheFlagName = "terragrunt-provider-cache"
	// TerragruntProviderCacheEnvVarName is the name of the environment variable that specifies
	// whether Terragrunt should enable provider caching.
	TerragruntProviderCacheEnvVarName = "TERRAGRUNT_PROVIDER_CACHE"

	// TerragruntProviderCacheDirFlagName is the name of the flag that specifies
	// the path to the Terragrunt provider cache directory.
	TerragruntProviderCacheDirFlagName = "terragrunt-provider-cache-dir"
	// TerragruntProviderCacheDirEnvVarName is the name of the environment variable that specifies
	// the path to the Terragrunt provider cache directory.
	TerragruntProviderCacheDirEnvVarName = "TERRAGRUNT_PROVIDER_CACHE_DIR"

	// TerragruntProviderCacheHostnameFlagName is the name of the flag that specifies
	// the hostname of the Terragrunt Provider Cache server.
	TerragruntProviderCacheHostnameFlagName = "terragrunt-provider-cache-hostname"
	// TerragruntProviderCacheHostnameEnvVarName is the name of the environment variable that specifies
	// the hostname of the Terragrunt Provider Cache server.
	TerragruntProviderCacheHostnameEnvVarName = "TERRAGRUNT_PROVIDER_CACHE_HOSTNAME"

	// TerragruntProviderCachePortFlagName is the name of the flag that specifies
	// the port of the Terragrunt Provider Cache server.
	TerragruntProviderCachePortFlagName = "terragrunt-provider-cache-port"
	// TerragruntProviderCachePortEnvVarName is the name of the environment variable that specifies
	// the port of the Terragrunt Provider Cache server.
	TerragruntProviderCachePortEnvVarName = "TERRAGRUNT_PROVIDER_CACHE_PORT"

	// TerragruntProviderCacheTokenFlagName is the name of the flag that specifies
	// the token for authentication to the Terragrunt Provider Cache server.
	TerragruntProviderCacheTokenFlagName = "terragrunt-provider-cache-token"
	// TerragruntProviderCacheTokenEnvVarName is the name of the environment variable that specifies
	// the token for authentication to the Terragrunt Provider Cache server.
	TerragruntProviderCacheTokenEnvVarName = "TERRAGRUNT_PROVIDER_CACHE_TOKEN"

	// TerragruntProviderCacheRegistryNamesFlagName is the name of the flag that specifies
	// the list of remote registries to be cached by the Terragrunt Provider Cache server.
	TerragruntProviderCacheRegistryNamesFlagName = "terragrunt-provider-cache-registry-names"
	// TerragruntProviderCacheRegistryNamesEnvVarName is the name of the environment variable that specifies
	// the list of remote registries to be cached by the Terragrunt Provider Cache server.
	TerragruntProviderCacheRegistryNamesEnvVarName = "TERRAGRUNT_PROVIDER_CACHE_REGISTRY_NAMES"

	// HelpFlagName is the name of the flag that specifies
	// whether Terragrunt should show help information.
	HelpFlagName = "help"

	// VersionFlagName is the name of the flag that specifies
	// whether Terragrunt should show version information.
	VersionFlagName = "version"
)

// NewGlobalFlags creates and returns global flags.
func NewGlobalFlags(opts *options.TerragruntOptions) cli.Flags {
	flags := cli.Flags{
		&cli.GenericFlag[string]{
			Name:        TerragruntConfigFlagName,
			EnvVar:      "TERRAGRUNT_CONFIG",
			Usage:       "The path to the Terragrunt config file. Default is terragrunt.hcl.",
			Destination: &opts.TerragruntConfigPath,
		},
		&cli.GenericFlag[string]{
			Name:        TerragruntTFPathFlagName,
			EnvVar:      "TERRAGRUNT_TFPATH",
			Usage:       "Path to the Terraform binary. Default is terraform (on PATH).",
			Destination: &opts.TerraformPath,
		},
		&cli.BoolFlag{
			Name:        TerragruntNoAutoInitFlagName,
			EnvVar:      "TERRAGRUNT_NO_AUTO_INIT",
			Usage:       "Don't automatically run 'terraform init' during other terragrunt commands. You must run 'terragrunt init' manually.", //nolint:lll
			Negative:    true,
			Destination: &opts.AutoInit,
		},
		&cli.BoolFlag{
			Name:        TerragruntNoAutoRetryFlagName,
			Destination: &opts.AutoRetry,
			EnvVar:      "TERRAGRUNT_NO_AUTO_RETRY",
			Usage:       "Don't automatically re-run command in case of transient errors.",
			Negative:    true,
		},
		&cli.BoolFlag{
			Name:        TerragruntNoAutoApproveFlagName,
			Destination: &opts.RunAllAutoApprove,
			EnvVar:      "TERRAGRUNT_NO_AUTO_APPROVE",
			Usage:       "Don't automatically append `-auto-approve` to the underlying Terraform commands run with 'run-all'.",
			Negative:    true,
		},
		&cli.BoolFlag{
			Name:        TerragruntNonInteractiveFlagName,
			Destination: &opts.NonInteractive,
			EnvVar:      "TERRAGRUNT_NON_INTERACTIVE",
			Usage:       `Assume "yes" for all prompts.`,
		},
		&cli.GenericFlag[string]{
			Name:        TerragruntWorkingDirFlagName,
			Destination: &opts.WorkingDir,
			EnvVar:      "TERRAGRUNT_WORKING_DIR",
			Usage:       "The path to the Terraform templates. Default is current directory.",
		},
		&cli.GenericFlag[string]{
			Name:        TerragruntDownloadDirFlagName,
			Destination: &opts.DownloadDir,
			EnvVar:      "TERRAGRUNT_DOWNLOAD",
			Usage:       "The path where to download Terraform code. Default is .terragrunt-cache in the working directory.",
		},
		&cli.GenericFlag[string]{
			Name:        TerragruntSourceFlagName,
			Destination: &opts.Source,
			EnvVar:      "TERRAGRUNT_SOURCE",
			Usage:       "Download Terraform configurations from the specified source into a temporary folder, and run Terraform in that temporary folder.", //nolint:lll
		},
		&cli.BoolFlag{
			Name:        TerragruntSourceUpdateFlagName,
			Destination: &opts.SourceUpdate,
			EnvVar:      "TERRAGRUNT_SOURCE_UPDATE",
			Usage:       "Delete the contents of the temporary folder to clear out any old, cached source code before downloading new source code into it.", //nolint:lll
		},
		&cli.MapFlag[string, string]{
			Name:        TerragruntSourceMapFlagName,
			Destination: &opts.SourceMap,
			EnvVar:      "TERRAGRUNT_SOURCE_MAP",
			Usage:       "Replace any source URL (including the source URL of a config pulled in with dependency blocks) that has root source with dest.", //nolint:lll
			Splitter:    util.SplitUrls,
		},
		&cli.GenericFlag[string]{
			Name:        TerragruntIAMRoleFlagName,
			Destination: &opts.IAMRoleOptions.RoleARN,
			EnvVar:      "TERRAGRUNT_IAM_ROLE",
			Usage:       "Assume the specified IAM role before executing Terraform. Can also be set via the TERRAGRUNT_IAM_ROLE environment variable.", //nolint:lll
		},
		&cli.GenericFlag[int64]{
			Name:        TerragruntIAMAssumeRoleDurationFlagName,
			Destination: &opts.IAMRoleOptions.AssumeRoleDuration,
			EnvVar:      "TERRAGRUNT_IAM_ASSUME_ROLE_DURATION",
			Usage:       "Session duration for IAM Assume Role session. Can also be set via the TERRAGRUNT_IAM_ASSUME_ROLE_DURATION environment variable.", //nolint:lll
		},
		&cli.GenericFlag[string]{
			Name:        TerragruntIAMAssumeRoleSessionNameFlagName,
			Destination: &opts.IAMRoleOptions.AssumeRoleSessionName,
			EnvVar:      "TERRAGRUNT_IAM_ASSUME_ROLE_SESSION_NAME",
			Usage:       "Name for the IAM Assummed Role session. Can also be set via TERRAGRUNT_IAM_ASSUME_ROLE_SESSION_NAME environment variable.", //nolint:lll
		},
		&cli.GenericFlag[string]{
			Name:        TerragruntIAMWebIdentityTokenFlagName,
			Destination: &opts.IAMRoleOptions.WebIdentityToken,
			EnvVar:      "TERRAGRUNT_IAM_ASSUME_ROLE_WEB_IDENTITY_TOKEN",
			Usage:       "For AssumeRoleWithWebIdentity, the WebIdentity token. Can also be set via TERRAGRUNT_IAM_ASSUME_ROLE_WEB_IDENTITY_TOKEN environment variable", //nolint:lll
		},
		&cli.BoolFlag{
			Name:        TerragruntIgnoreDependencyErrorsFlagName,
			Destination: &opts.IgnoreDependencyErrors,
			Usage:       "*-all commands continue processing components even if a dependency fails.",
		},
		&cli.BoolFlag{
			Name:        TerragruntIgnoreDependencyOrderFlagName,
			Destination: &opts.IgnoreDependencyOrder,
			Usage:       "*-all commands will be run disregarding the dependencies",
		},
		&cli.BoolFlag{
			Name:        TerragruntIgnoreExternalDependenciesFlagName,
			Destination: &opts.IgnoreExternalDependencies,
			Usage:       "*-all commands will not attempt to include external dependencies",
		},
		&cli.BoolFlag{
			Name:        TerragruntIncludeExternalDependenciesFlagName,
			Destination: &opts.IncludeExternalDependencies,
			EnvVar:      "TERRAGRUNT_INCLUDE_EXTERNAL_DEPENDENCIES",
			Usage:       "*-all commands will include external dependencies",
		},
		&cli.GenericFlag[int]{
			Name:        TerragruntParallelismFlagName,
			Destination: &opts.Parallelism,
			EnvVar:      "TERRAGRUNT_PARALLELISM",
			Usage:       "*-all commands parallelism set to at most N modules",
		},
		&cli.GenericFlag[string]{
			Name:        TerragruntExcludesFile,
			Destination: &opts.ExcludesFile,
			EnvVar:      "TERRAGRUNT_EXCLUDES_FILE",
			Usage:       "Path to a file with a list of directories that need to be excluded when running *-all commands.",
		},
		&cli.SliceFlag[string]{
			Name:        TerragruntExcludeDirFlagName,
			Destination: &opts.ExcludeDirs,
			EnvVar:      "TERRAGRUNT_EXCLUDE_DIR",
			Usage:       "Unix-style glob of directories to exclude when running *-all commands.",
		},
		&cli.SliceFlag[string]{
			Name:        TerragruntIncludeDirFlagName,
			Destination: &opts.IncludeDirs,
			Usage:       "Unix-style glob of directories to include when running *-all commands",
		},
		&cli.BoolFlag{
			Name:        TerragruntDebugFlagName,
			Destination: &opts.Debug,
			EnvVar:      "TERRAGRUNT_DEBUG",
			Usage:       "Write terragrunt-debug.tfvars to working folder to help root-cause issues.",
		},
		&cli.GenericFlag[string]{
			Name:        TerragruntLogLevelFlagName,
			Destination: &opts.LogLevelStr,
			EnvVar:      "TERRAGRUNT_LOG_LEVEL",
			Usage:       "Sets the logging level for Terragrunt. Supported levels: panic, fatal, error, warn, info, debug, trace.", //nolint:lll
		},
		&cli.BoolFlag{
			Name:        TerragruntNoColorFlagName,
			Destination: &opts.DisableLogColors,
			EnvVar:      "TERRAGRUNT_NO_COLOR",
			Usage:       "If specified, Terragrunt output won't contain any color.",
		},
		&cli.BoolFlag{
			Name:        TerragruntJSONLogFlagName,
			Destination: &opts.JSONLogFormat,
			EnvVar:      "TERRAGRUNT_JSON_LOG",
			Usage:       "If specified, Terragrunt will output its logs in JSON format.",
		},
		&cli.BoolFlag{
			Name:        TerragruntTfLogJSONFlagName,
			Destination: &opts.TerraformLogsToJSON,
			EnvVar:      "TERRAGRUNT_TF_JSON_LOG",
			Usage:       "If specified, Terragrunt will wrap Terraform stdout and stderr in JSON.",
		},
		&cli.BoolFlag{
			Name:        TerragruntUsePartialParseConfigCacheFlagName,
			Destination: &opts.UsePartialParseConfigCache,
			EnvVar:      "TERRAGRUNT_USE_PARTIAL_PARSE_CONFIG_CACHE",
			Usage:       "Enables caching of includes during partial parsing operations. Will also be used for the --terragrunt-iam-role option if provided.", //nolint:lll
		},
		&cli.BoolFlag{
			Name:        TerragruntFetchDependencyOutputFromStateFlagName,
			Destination: &opts.FetchDependencyOutputFromState,
			EnvVar:      "TERRAGRUNT_FETCH_DEPENDENCY_OUTPUT_FROM_STATE",
			Usage:       "The option fetchs dependency output directly from the state file instead of init dependencies and running terraform on them.", //nolint:lll
		},
		&cli.BoolFlag{
			Name:        TerragruntIncludeModulePrefixFlagName,
			Destination: &opts.IncludeModulePrefix,
			EnvVar:      "TERRAGRUNT_INCLUDE_MODULE_PREFIX",
			Usage:       "When this flag is set output from Terraform sub-commands is prefixed with module path.",
		},
		&cli.BoolFlag{
			Name:        TerragruntStrictIncludeFlagName,
			Destination: &opts.StrictInclude,
			Usage:       "If flag is set, only modules under the directories passed in with '--terragrunt-include-dir' will be included.", //nolint:lll
		},
		&cli.SliceFlag[string]{
			Name:        TerragruntModulesThatIncludeFlagName,
			Destination: &opts.ModulesThatInclude,
			Usage:       "If flag is set, 'run-all' will only run the command against Terragrunt modules that include the specified file.", //nolint:lll
		},
		&cli.BoolFlag{
			Name:        TerragruntFailOnStateBucketCreationFlagName,
			Destination: &opts.FailIfBucketCreationRequired,
			EnvVar:      "TERRAGRUNT_FAIL_ON_STATE_BUCKET_CREATION",
			Usage:       "When this flag is set Terragrunt will fail if the remote state bucket needs to be created.",
		},
		&cli.BoolFlag{
			Name:        TerragruntDisableBucketUpdateFlagName,
			Destination: &opts.DisableBucketUpdate,
			EnvVar:      "TERRAGRUNT_DISABLE_BUCKET_UPDATE",
			Usage:       "When this flag is set Terragrunt will not update the remote state bucket.",
		},
		&cli.BoolFlag{
			Name:        TerragruntDisableCommandValidationFlagName,
			Destination: &opts.DisableCommandValidation,
			EnvVar:      "TERRAGRUNT_DISABLE_COMMAND_VALIDATION",
			Usage:       "When this flag is set, Terragrunt will not validate the terraform command.",
		},
		// Terragrunt Provider Cache flags
		&cli.BoolFlag{
			Name:        TerragruntProviderCacheFlagName,
			Destination: &opts.ProviderCache,
			EnvVar:      TerragruntProviderCacheEnvVarName,
			Usage:       "Enables Terragrunt's provider caching.",
		},
		&cli.GenericFlag[string]{
			Name:        TerragruntProviderCacheDirFlagName,
			Destination: &opts.ProviderCacheDir,
			EnvVar:      TerragruntProviderCacheDirEnvVarName,
			Usage:       "The path to the Terragrunt provider cache directory. By default, 'terragrunt/providers' folder in the user cache directory.", //nolint:lll
		},
		&cli.GenericFlag[string]{
			Name:        TerragruntProviderCacheTokenFlagName,
			Destination: &opts.ProviderCacheToken,
			EnvVar:      TerragruntProviderCacheTokenEnvVarName,
			Usage:       "The Token for authentication to the Terragrunt Provider Cache server. By default, assigned automatically.", //nolint:lll
		},
		&cli.GenericFlag[string]{
			Name:        TerragruntProviderCacheHostnameFlagName,
			Destination: &opts.ProviderCacheHostname,
			EnvVar:      TerragruntProviderCacheHostnameEnvVarName,
			Usage:       "The hostname of the Terragrunt Provider Cache server. By default, 'localhost'.",
		},
		&cli.GenericFlag[int]{
			Name:        TerragruntProviderCachePortFlagName,
			Destination: &opts.ProviderCachePort,
			EnvVar:      TerragruntProviderCachePortEnvVarName,
			Usage:       "The port of the Terragrunt Provider Cache server. By default, assigned automatically.",
		},
		&cli.SliceFlag[string]{
			Name:        TerragruntProviderCacheRegistryNamesFlagName,
			Destination: &opts.ProviderCacheRegistryNames,
			EnvVar:      TerragruntProviderCacheRegistryNamesEnvVarName,
			Usage:       "The list of remote registries to cached by Terragrunt Provider Cache server. By default, 'registry.terraform.io', 'registry.opentofu.org'.", //nolint:lll
		},
		&cli.GenericFlag[string]{
			Name:        TerragruntAuthProviderCmdFlagName,
			Destination: &opts.AuthProviderCmd,
			EnvVar:      TerragruntAuthProviderCmdEnvVarName,
			Usage:       "The command and arguments that can be used to fetch authentication configurations.",
		},
	}

	flags.Sort()

	return flags
}

// NewHelpVersionFlags creates and returns help and version flags.
func NewHelpVersionFlags(opts *options.TerragruntOptions) cli.Flags {
	return cli.Flags{
		NewHelpFlag(opts),
		NewVersionFlag(opts),
	}
}

// NewHelpFlag creates and returns a help flag.
func NewHelpFlag(opts *options.TerragruntOptions) *cli.BoolFlag {
	return &cli.BoolFlag{
		Name:    HelpFlagName,  // --help, -help
		Aliases: []string{"h"}, //  -h
		Usage:   "Show help",
		Action: func(ctx *cli.Context) (err error) {
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

// NewVersionFlag creates and returns a version flag.
func NewVersionFlag(_ *options.TerragruntOptions) *cli.BoolFlag {
	return &cli.BoolFlag{
		Name:    VersionFlagName, // --version, -version
		Aliases: []string{"v"},   //  -v
		Usage:   "Show terragrunt version",
		Action: func(ctx *cli.Context) (err error) {
			defer func() {
				// exit the app
				err = cli.NewExitError(err, 0)
			}()

			return cli.ShowVersion(ctx)
		},
	}
}
