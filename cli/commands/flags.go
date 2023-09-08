package commands

import (
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/pkg/cli"
	"github.com/gruntwork-io/terragrunt/shell"
	"github.com/gruntwork-io/terragrunt/util"
)

const (
	FlagNameTerragruntConfig                         = "terragrunt-config"
	FlagNameTerragruntTFPath                         = "terragrunt-tfpath"
	FlagNameTerragruntNoAutoInit                     = "terragrunt-no-auto-init"
	FlagNameTerragruntNoAutoRetry                    = "terragrunt-no-auto-retry"
	FlagNameTerragruntNoAutoApprove                  = "terragrunt-no-auto-approve"
	FlagNameTerragruntNonInteractive                 = "terragrunt-non-interactive"
	FlagNameTerragruntWorkingDir                     = "terragrunt-working-dir"
	FlagNameTerragruntDownloadDir                    = "terragrunt-download-dir"
	FlagNameTerragruntSource                         = "terragrunt-source"
	FlagNameTerragruntSourceMap                      = "terragrunt-source-map"
	FlagNameTerragruntSourceUpdate                   = "terragrunt-source-update"
	FlagNameTerragruntIAMRole                        = "terragrunt-iam-role"
	FlagNameTerragruntIAMAssumeRoleDuration          = "terragrunt-iam-assume-role-duration"
	FlagNameTerragruntIAMAssumeRoleSessionName       = "terragrunt-iam-assume-role-session-name"
	FlagNameTerragruntIgnoreDependencyErrors         = "terragrunt-ignore-dependency-errors"
	FlagNameTerragruntIgnoreDependencyOrder          = "terragrunt-ignore-dependency-order"
	FlagNameTerragruntIgnoreExternalDependencies     = "terragrunt-ignore-external-dependencies"
	FlagNameTerragruntIncludeExternalDependencies    = "terragrunt-include-external-dependencies"
	FlagNameTerragruntExcludeDir                     = "terragrunt-exclude-dir"
	FlagNameTerragruntIncludeDir                     = "terragrunt-include-dir"
	FlagNameTerragruntStrictInclude                  = "terragrunt-strict-include"
	FlagNameTerragruntParallelism                    = "terragrunt-parallelism"
	FlagNameTerragruntDebug                          = "terragrunt-debug"
	FlagNameTerragruntLogLevel                       = "terragrunt-log-level"
	FlagNameTerragruntNoColor                        = "terragrunt-no-color"
	FlagNameTerragruntModulesThatInclude             = "terragrunt-modules-that-include"
	FlagNameTerragruntFetchDependencyOutputFromState = "terragrunt-fetch-dependency-output-from-state"
	FlagNameTerragruntUsePartialParseConfigCache     = "terragrunt-use-partial-parse-config-cache"
	FlagNameTerragruntIncludeModulePrefix            = "terragrunt-include-module-prefix"
	FlagNameTerragruntFailOnStateBucketCreation      = "terragrunt-fail-on-state-bucket-creation"
	FlagNameTerragruntDisableBucketUpdate            = "terragrunt-disable-bucket-update"
	FlagNameTerragruntDisableCommandValidation       = "terragrunt-disable-command-validation"

	FlagNameHelp    = "help"
	FlagNameVersion = "version"
)

// NewGlobalFlags creates and returns global flags.
func NewGlobalFlags(opts *options.TerragruntOptions) cli.Flags {
	flags := cli.Flags{
		&cli.GenericFlag[string]{
			Name:        FlagNameTerragruntConfig,
			EnvVar:      "TERRAGRUNT_CONFIG",
			Usage:       "The path to the Terragrunt config file. Default is terragrunt.hcl.",
			Destination: &opts.TerragruntConfigPath,
		},
		&cli.GenericFlag[string]{
			Name:        FlagNameTerragruntTFPath,
			EnvVar:      "TERRAGRUNT_TFPATH",
			Usage:       "Path to the Terraform binary. Default is terraform (on PATH).",
			Destination: &opts.TerraformPath,
		},
		&cli.BoolFlag{
			Name:        FlagNameTerragruntNoAutoInit,
			EnvVar:      "TERRAGRUNT_NO_AUTO_INIT",
			Usage:       "Don't automatically run 'terraform init' during other terragrunt commands. You must run 'terragrunt init' manually.",
			Negative:    true,
			Destination: &opts.AutoInit,
		},
		&cli.BoolFlag{
			Name:        FlagNameTerragruntNoAutoRetry,
			Destination: &opts.AutoRetry,
			EnvVar:      "TERRAGRUNT_NO_AUTO_RETRY",
			Usage:       "Don't automatically re-run command in case of transient errors.",
			Negative:    true,
		},
		&cli.BoolFlag{
			Name:        FlagNameTerragruntNoAutoApprove,
			Destination: &opts.RunAllAutoApprove,
			EnvVar:      "TERRAGRUNT_NO_AUTO_APPROVE",
			Usage:       "Don't automatically append `-auto-approve` to the underlying Terraform commands run with 'run-all'.",
			Negative:    true,
		},
		&cli.BoolFlag{
			Name:        FlagNameTerragruntNonInteractive,
			Destination: &opts.NonInteractive,
			EnvVar:      "TERRAGRUNT_NON_INTERACTIVE",
			Usage:       `Assume "yes" for all prompts.`,
		},
		&cli.GenericFlag[string]{
			Name:        FlagNameTerragruntWorkingDir,
			Destination: &opts.WorkingDir,
			EnvVar:      "TERRAGRUNT_WORKING_DIR",
			Usage:       "The path to the Terraform templates. Default is current directory.",
		},
		&cli.GenericFlag[string]{
			Name:        FlagNameTerragruntDownloadDir,
			Destination: &opts.DownloadDir,
			EnvVar:      "TERRAGRUNT_DOWNLOAD",
			Usage:       "The path where to download Terraform code. Default is .terragrunt-cache in the working directory.",
		},
		&cli.GenericFlag[string]{
			Name:        FlagNameTerragruntSource,
			Destination: &opts.Source,
			EnvVar:      "TERRAGRUNT_SOURCE",
			Usage:       "Download Terraform configurations from the specified source into a temporary folder, and run Terraform in that temporary folder.",
		},
		&cli.BoolFlag{
			Name:        FlagNameTerragruntSourceUpdate,
			Destination: &opts.SourceUpdate,
			EnvVar:      "TERRAGRUNT_SOURCE_UPDATE",
			Usage:       "Delete the contents of the temporary folder to clear out any old, cached source code before downloading new source code into it.",
		},
		&cli.MapFlag[string, string]{
			Name:        FlagNameTerragruntSourceMap,
			Destination: &opts.SourceMap,
			EnvVar:      "TERRAGRUNT_SOURCE_MAP",
			Usage:       "Replace any source URL (including the source URL of a config pulled in with dependency blocks) that has root source with dest.",
			Splitter:    util.SplitUrls,
		},
		&cli.GenericFlag[string]{
			Name:        FlagNameTerragruntIAMRole,
			Destination: &opts.IAMRoleOptions.RoleARN,
			EnvVar:      "TERRAGRUNT_IAM_ROLE",
			Usage:       "Assume the specified IAM role before executing Terraform. Can also be set via the TERRAGRUNT_IAM_ROLE environment variable.",
		},
		&cli.GenericFlag[int64]{
			Name:        FlagNameTerragruntIAMAssumeRoleDuration,
			Destination: &opts.IAMRoleOptions.AssumeRoleDuration,
			EnvVar:      "TERRAGRUNT_IAM_ASSUME_ROLE_DURATION",
			Usage:       "Session duration for IAM Assume Role session. Can also be set via the TERRAGRUNT_IAM_ASSUME_ROLE_DURATION environment variable.",
		},
		&cli.GenericFlag[string]{
			Name:        FlagNameTerragruntIAMAssumeRoleSessionName,
			Destination: &opts.IAMRoleOptions.AssumeRoleSessionName,
			EnvVar:      "TERRAGRUNT_IAM_ASSUME_ROLE_SESSION_NAME",
			Usage:       "Name for the IAM Assummed Role session. Can also be set via TERRAGRUNT_IAM_ASSUME_ROLE_SESSION_NAME environment variable.",
		},
		&cli.BoolFlag{
			Name:        FlagNameTerragruntIgnoreDependencyErrors,
			Destination: &opts.IgnoreDependencyErrors,
			Usage:       "*-all commands continue processing components even if a dependency fails.",
		},
		&cli.BoolFlag{
			Name:        FlagNameTerragruntIgnoreDependencyOrder,
			Destination: &opts.IgnoreDependencyOrder,
			Usage:       "*-all commands will be run disregarding the dependencies",
		},
		&cli.BoolFlag{
			Name:        FlagNameTerragruntIgnoreExternalDependencies,
			Destination: &opts.IgnoreExternalDependencies,
			Usage:       "*-all commands will not attempt to include external dependencies",
		},
		&cli.BoolFlag{
			Name:        FlagNameTerragruntIncludeExternalDependencies,
			Destination: &opts.IncludeExternalDependencies,
			EnvVar:      "TERRAGRUNT_INCLUDE_EXTERNAL_DEPENDENCIES",
			Usage:       "*-all commands will include external dependencies",
		},
		&cli.GenericFlag[int]{
			Name:        FlagNameTerragruntParallelism,
			Destination: &opts.Parallelism,
			EnvVar:      "TERRAGRUNT_PARALLELISM",
			Usage:       "*-all commands parallelism set to at most N modules",
		},
		&cli.SliceFlag[string]{
			Name:        FlagNameTerragruntExcludeDir,
			Destination: &opts.ExcludeDirs,
			EnvVar:      "TERRAGRUNT_EXCLUDE_DIR",
			Usage:       "Unix-style glob of directories to exclude when running *-all commands.",
		},
		&cli.SliceFlag[string]{
			Name:        FlagNameTerragruntIncludeDir,
			Destination: &opts.IncludeDirs,
			Usage:       "Unix-style glob of directories to include when running *-all commands",
		},
		&cli.BoolFlag{
			Name:        FlagNameTerragruntDebug,
			Destination: &opts.Debug,
			EnvVar:      "TERRAGRUNT_DEBUG",
			Usage:       "Write terragrunt-debug.tfvars to working folder to help root-cause issues.",
		},
		&cli.GenericFlag[string]{
			Name:        FlagNameTerragruntLogLevel,
			Destination: &opts.LogLevelStr,
			EnvVar:      "TERRAGRUNT_LOG_LEVEL",
			Usage:       "Sets the logging level for Terragrunt. Supported levels: panic, fatal, error, warn, info, debug, trace.",
		},
		&cli.BoolFlag{
			Name:        FlagNameTerragruntNoColor,
			Destination: &opts.DisableLogColors,
			EnvVar:      "TERRAGRUNT_NO_COLOR",
			Usage:       "If specified, Terragrunt output won't contain any color.",
		},
		&cli.BoolFlag{
			Name:        FlagNameTerragruntUsePartialParseConfigCache,
			Destination: &opts.UsePartialParseConfigCache,
			EnvVar:      "TERRAGRUNT_USE_PARTIAL_PARSE_CONFIG_CACHE",
			Usage:       "Enables caching of includes during partial parsing operations. Will also be used for the --terragrunt-iam-role option if provided.",
		},
		&cli.BoolFlag{
			Name:        FlagNameTerragruntFetchDependencyOutputFromState,
			Destination: &opts.FetchDependencyOutputFromState,
			EnvVar:      "TERRAGRUNT_FETCH_DEPENDENCY_OUTPUT_FROM_STATE",
			Usage:       "The option fetchs dependency output directly from the state file instead of init dependencies and running terraform on them.",
		},
		&cli.BoolFlag{
			Name:        FlagNameTerragruntIncludeModulePrefix,
			Destination: &opts.IncludeModulePrefix,
			EnvVar:      "TERRAGRUNT_INCLUDE_MODULE_PREFIX",
			Usage:       "When this flag is set output from Terraform sub-commands is prefixed with module path.",
		},
		&cli.BoolFlag{
			Name:        FlagNameTerragruntStrictInclude,
			Destination: &opts.StrictInclude,
			Usage:       "If flag is set, only modules under the directories passed in with '--terragrunt-include-dir' will be included.",
		},
		&cli.SliceFlag[string]{
			Name:        FlagNameTerragruntModulesThatInclude,
			Destination: &opts.ModulesThatInclude,
			Usage:       "If flag is set, 'run-all' will only run the command against Terragrunt modules that include the specified file.",
		},
		&cli.BoolFlag{
			Name:        FlagNameTerragruntFailOnStateBucketCreation,
			Destination: &opts.FailIfBucketCreationRequired,
			EnvVar:      "TERRAGRUNT_FAIL_ON_STATE_BUCKET_CREATION",
			Usage:       "When this flag is set Terragrunt will fail if the remote state bucket needs to be created.",
		},
		&cli.BoolFlag{
			Name:        FlagNameTerragruntDisableBucketUpdate,
			Destination: &opts.DisableBucketUpdate,
			EnvVar:      "TERRAGRUNT_DISABLE_BUCKET_UPDATE",
			Usage:       "When this flag is set Terragrunt will not update the remote state bucket.",
		},
		&cli.BoolFlag{
			Name:        FlagNameTerragruntDisableCommandValidation,
			Destination: &opts.DisableCommandValidation,
			EnvVar:      "TERRAGRUNT_DISABLE_COMMAND_VALIDATION",
			Usage:       "When this flag is set, Terragrunt will not validate the terraform command.",
		},
	}

	flags.Sort()

	// add auxiliary flags after sorting to put the flag at the end of the flag list in the help.
	flags.Add(
		NewHelpFlag(opts),
		NewVersionFlag(opts),
	)

	return flags
}

func NewHelpFlag(opts *options.TerragruntOptions) cli.Flag {
	return &cli.BoolFlag{
		Name:    FlagNameHelp,  // --help, -help
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
				if _, ok := err.(cli.InvalidCommandNameError); ok {
					terraformHelpCmd := append([]string{cmdName, "-help"}, ctx.Args().Tail()...)
					return shell.RunTerraformCommand(opts, terraformHelpCmd...)
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
		Name:    FlagNameVersion, // --version, -version
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
