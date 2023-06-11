package cli

import (
	"sort"

	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/pkg/cli"
	"github.com/gruntwork-io/terragrunt/util"
)

const (
	flagTerragruntConfig                         = "terragrunt-config"
	flagTerragruntTFPath                         = "terragrunt-tfpath"
	flagTerragruntNoAutoInit                     = "terragrunt-no-auto-init"
	flagTerragruntNoAutoRetry                    = "terragrunt-no-auto-retry"
	flagTerragruntNoAutoApprove                  = "terragrunt-no-auto-approve"
	flagTerragruntNonInteractive                 = "terragrunt-non-interactive"
	flagTerragruntWorkingDir                     = "terragrunt-working-dir"
	flagTerragruntDownloadDir                    = "terragrunt-download-dir"
	flagTerragruntSource                         = "terragrunt-source"
	flagTerragruntSourceMap                      = "terragrunt-source-map"
	flagTerragruntSourceUpdate                   = "terragrunt-source-update"
	flagTerragruntIAMRole                        = "terragrunt-iam-role"
	flagTerragruntIAMAssumeRoleDuration          = "terragrunt-iam-assume-role-duration"
	flagTerragruntIAMAssumeRoleSessionName       = "terragrunt-iam-assume-role-session-name"
	flagTerragruntIgnoreDependencyErrors         = "terragrunt-ignore-dependency-errors"
	flagTerragruntIgnoreDependencyOrder          = "terragrunt-ignore-dependency-order"
	flagTerragruntIgnoreExternalDependencies     = "terragrunt-ignore-external-dependencies"
	flagTerragruntIncludeExternalDependencies    = "terragrunt-include-external-dependencies"
	flagTerragruntExcludeDir                     = "terragrunt-exclude-dir"
	flagTerragruntIncludeDir                     = "terragrunt-include-dir"
	flagTerragruntStrictInclude                  = "terragrunt-strict-include"
	flagTerragruntParallelism                    = "terragrunt-parallelism"
	flagTerragruntCheck                          = "terragrunt-check"
	flagTerragruntDiff                           = "terragrunt-diff"
	flagTerragruntDebug                          = "terragrunt-debug"
	flagTerragruntLogLevel                       = "terragrunt-log-level"
	flagTerragruntNoColor                        = "terragrunt-no-color"
	flagTerragruntModulesThatInclude             = "terragrunt-modules-that-include"
	flagTerragruntFetchDependencyOutputFromState = "terragrunt-fetch-dependency-output-from-state"
	flagTerragruntUsePartialParseConfigCache     = "terragrunt-use-partial-parse-config-cache"
	flagTerragruntIncludeModulePrefix            = "terragrunt-include-module-prefix"

	flagHelp = "help"
)

func newFlags(opts *options.TerragruntOptions) cli.Flags {
	flags := cli.Flags{
		&cli.GenericFlag[string]{
			Name:        flagTerragruntConfig,
			EnvVar:      "TERRAGRUNT_CONFIG",
			Usage:       "The path to the Terragrunt config file. Default is terragrunt.hcl.",
			Destination: &opts.TerragruntConfigPath,
		},
		&cli.GenericFlag[string]{
			Name:        flagTerragruntTFPath,
			EnvVar:      "TERRAGRUNT_TFPATH",
			Usage:       "Path to the Terraform binary. Default is terraform (on PATH).",
			Destination: &opts.TerraformPath,
		},
		&cli.BoolFlag{
			Name:        flagTerragruntNoAutoInit,
			EnvVar:      "TERRAGRUNT_NO_AUTO_INIT",
			Usage:       "Don't automatically run 'terraform init' during other terragrunt commands. You must run 'terragrunt init' manually.",
			Negative:    true,
			Destination: &opts.AutoInit,
		},
		&cli.BoolFlag{
			Name:        flagTerragruntNoAutoRetry,
			Destination: &opts.AutoRetry,
			EnvVar:      "TERRAGRUNT_NO_AUTO_RETRY",
			Usage:       "Don't automatically re-run command in case of transient errors.",
			Negative:    true,
		},
		&cli.BoolFlag{
			Name:        flagTerragruntNoAutoApprove,
			Destination: &opts.RunAllAutoApprove,
			EnvVar:      "TERRAGRUNT_NO_AUTO_APPROVE",
			Usage:       "Don't automatically append `-auto-approve` to the underlying Terraform commands run with 'run-all'.",
			Negative:    true,
		},
		&cli.BoolFlag{
			Name:        flagTerragruntNonInteractive,
			Destination: &opts.NonInteractive,
			EnvVar:      "TERRAGRUNT_NON_INTERACTIVE",
			Usage:       `Assume "yes" for all prompts.`,
		},
		&cli.GenericFlag[string]{
			Name:        flagTerragruntWorkingDir,
			Destination: &opts.WorkingDir,
			EnvVar:      "TERRAGRUNT_WORKING_DIR",
			Usage:       "The path to the Terraform templates. Default is current directory.",
		},
		&cli.GenericFlag[string]{
			Name:        flagTerragruntDownloadDir,
			Destination: &opts.DownloadDir,
			EnvVar:      "TERRAGRUNT_DOWNLOAD",
			Usage:       "The path where to download Terraform code. Default is .terragrunt-cache in the working directory.",
		},
		&cli.GenericFlag[string]{
			Name:        flagTerragruntSource,
			Destination: &opts.Source,
			EnvVar:      "TERRAGRUNT_SOURCE",
			Usage:       "Download Terraform configurations from the specified source into a temporary folder, and run Terraform in that temporary folder.",
		},
		&cli.BoolFlag{
			Name:        flagTerragruntSourceUpdate,
			Destination: &opts.SourceUpdate,
			EnvVar:      "TERRAGRUNT_SOURCE_UPDATE",
			Usage:       "Delete the contents of the temporary folder to clear out any old, cached source code before downloading new source code into it.",
		},
		&cli.MapFlag[string, string]{
			Name:        flagTerragruntSourceMap,
			Destination: &opts.SourceMap,
			EnvVar:      "TERRAGRUNT_SOURCE_MAP",
			Usage:       "Replace any source URL (including the source URL of a config pulled in with dependency blocks) that has root source with dest.",
			Splitter:    util.SplitUrls,
		},
		&cli.GenericFlag[string]{
			Name:        flagTerragruntIAMRole,
			Destination: &opts.IAMRoleOptions.RoleARN,
			EnvVar:      "TERRAGRUNT_IAM_ROLE",
			Usage:       "Assume the specified IAM role before executing Terraform. Can also be set via the TERRAGRUNT_IAM_ROLE environment variable.",
		},
		&cli.GenericFlag[int64]{
			Name:        flagTerragruntIAMAssumeRoleDuration,
			Destination: &opts.IAMRoleOptions.AssumeRoleDuration,
			EnvVar:      "TERRAGRUNT_IAM_ASSUME_ROLE_DURATION",
			Usage:       "Session duration for IAM Assume Role session. Can also be set via the TERRAGRUNT_IAM_ASSUME_ROLE_DURATION environment variable.",
		},
		&cli.GenericFlag[string]{
			Name:        flagTerragruntIAMAssumeRoleSessionName,
			Destination: &opts.IAMRoleOptions.AssumeRoleSessionName,
			EnvVar:      "TERRAGRUNT_IAM_ASSUME_ROLE_SESSION_NAME",
			Usage:       "Name for the IAM Assummed Role session. Can also be set via TERRAGRUNT_IAM_ASSUME_ROLE_SESSION_NAME environment variable.",
		},
		&cli.BoolFlag{
			Name:        flagTerragruntIgnoreDependencyErrors,
			Destination: &opts.IgnoreDependencyErrors,
			Usage:       "*-all commands continue processing components even if a dependency fails.",
		},
		&cli.BoolFlag{
			Name:        flagTerragruntIgnoreDependencyOrder,
			Destination: &opts.IgnoreDependencyOrder,
			Usage:       "*-all commands will be run disregarding the dependencies",
		},
		&cli.BoolFlag{
			Name:        flagTerragruntIgnoreExternalDependencies,
			Destination: &opts.IgnoreExternalDependencies,
			Usage:       "*-all commands will not attempt to include external dependencies",
		},
		&cli.BoolFlag{
			Name:        flagTerragruntIncludeExternalDependencies,
			Destination: &opts.IncludeExternalDependencies,
			EnvVar:      "TERRAGRUNT_INCLUDE_EXTERNAL_DEPENDENCIES",
			Usage:       "*-all commands will include external dependencies",
		},
		&cli.GenericFlag[int]{
			Name:        flagTerragruntParallelism,
			Destination: &opts.Parallelism,
			EnvVar:      "TERRAGRUNT_PARALLELISM",
			Usage:       "*-all commands parallelism set to at most N modules",
		},
		&cli.SliceFlag[string]{
			Name:        flagTerragruntExcludeDir,
			Destination: &opts.ExcludeDirs,
			EnvVar:      "TERRAGRUNT_EXCLUDE_DIR",
			Usage:       "Unix-style glob of directories to exclude when running *-all commands.",
		},
		&cli.SliceFlag[string]{
			Name:        flagTerragruntIncludeDir,
			Destination: &opts.IncludeDirs,
			Usage:       "Unix-style glob of directories to include when running *-all commands",
		},
		&cli.BoolFlag{
			Name:        flagTerragruntCheck,
			Destination: &opts.Check,
			EnvVar:      "TERRAGRUNT_CHECK",
			Usage:       "Enable check mode in the hclfmt command.",
		},
		&cli.BoolFlag{
			Name:        flagTerragruntDiff,
			Destination: &opts.Diff,
			EnvVar:      "TERRAGRUNT_DIFF",
			Usage:       "Print diff between original and modified file versions when running with 'hclfmt'.",
		},
		&cli.BoolFlag{
			Name:        flagTerragruntDebug,
			Destination: &opts.Debug,
			EnvVar:      "TERRAGRUNT_DEBUG",
			Usage:       "Write terragrunt-debug.tfvars to working folder to help root-cause issues.",
		},
		&cli.GenericFlag[string]{
			Name:        flagTerragruntLogLevel,
			Destination: &opts.LogLevelStr,
			EnvVar:      "TERRAGRUNT_LOG_LEVEL",
			Usage:       "Sets the logging level for Terragrunt. Supported levels: panic, fatal, error, warn, info, debug, trace.",
		},
		&cli.BoolFlag{
			Name:        flagTerragruntNoColor,
			Destination: &opts.TerragruntNoColors,
			EnvVar:      "TERRAGRUNT_NO_COLOR",
			Usage:       "If specified, output won't contain any color.",
		},
		&cli.BoolFlag{
			Name:        flagTerragruntUsePartialParseConfigCache,
			Destination: &opts.UsePartialParseConfigCache,
			EnvVar:      "TERRAGRUNT_USE_PARTIAL_PARSE_CONFIG_CACHE",
			Usage:       "Enables caching of includes during partial parsing operations. Will also be used for the --terragrunt-iam-role option if provided.",
		},
		&cli.BoolFlag{
			Name:        flagTerragruntFetchDependencyOutputFromState,
			Destination: &opts.UsePartialParseConfigCache,
			EnvVar:      "TERRAGRUNT_FETCH_DEPENDENCY_OUTPUT_FROM_STATE",
			Usage:       "The option fetchs dependency output directly from the state file instead of init dependencies and running terraform on them.",
		},
		&cli.BoolFlag{
			Name:        flagTerragruntIncludeModulePrefix,
			Destination: &opts.IncludeModulePrefix,
			EnvVar:      "TERRAGRUNT_INCLUDE_MODULE_PREFIX",
			Usage:       "When this flag is set output from Terraform sub-commands is prefixed with module path.",
		},
		&cli.BoolFlag{
			Name:        flagTerragruntStrictInclude,
			Destination: &opts.StrictInclude,
			Usage:       "If flag is set, only modules under the directories passed in with '--terragrunt-include-dir' will be included.",
		},
		&cli.SliceFlag[string]{
			Name:        flagTerragruntModulesThatInclude,
			Destination: &opts.ModulesThatInclude,
			Usage:       "If flag is set, 'run-all' will only run the command against Terragrunt modules that include the specified file.",
		},
	}

	sort.Sort(cli.Flags(flags))

	// add `help` after the sort to put the flag at the end of the flag list in the help.
	flags.Add(
		&cli.BoolFlag{
			Name:    flagHelp,      // --help, -help
			Aliases: []string{"h"}, //  -h
			Usage:   "Show help",
		},
	)

	return flags
}
