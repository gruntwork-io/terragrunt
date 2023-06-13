package awsproviderpatch

import (
	"github.com/gruntwork-io/terragrunt/cli/flags"
	"github.com/gruntwork-io/terragrunt/errors"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/pkg/cli"
)

const (
	CommandName = "aws-provider-patch"
)

var (
	TerragruntFlagNames = []string{
		flags.FlagNameTerragruntConfig,
		flags.FlagNameTerragruntTFPath,
		flags.FlagNameTerragruntNoAutoInit,
		flags.FlagNameTerragruntNoAutoRetry,
		flags.FlagNameTerragruntNoAutoApprove,
		flags.FlagNameTerragruntNonInteractive,
		flags.FlagNameTerragruntWorkingDir,
		flags.FlagNameTerragruntDownloadDir,
		flags.FlagNameTerragruntSource,
		flags.FlagNameTerragruntSourceMap,
		flags.FlagNameTerragruntSourceUpdate,
		flags.FlagNameTerragruntIAMRole,
		flags.FlagNameTerragruntIAMAssumeRoleDuration,
		flags.FlagNameTerragruntIAMAssumeRoleSessionName,
		flags.FlagNameTerragruntIgnoreDependencyErrors,
		flags.FlagNameTerragruntIgnoreDependencyOrder,
		flags.FlagNameTerragruntIgnoreExternalDependencies,
		flags.FlagNameTerragruntIncludeExternalDependencies,
		flags.FlagNameTerragruntExcludeDir,
		flags.FlagNameTerragruntIncludeDir,
		flags.FlagNameTerragruntStrictInclude,
		flags.FlagNameTerragruntParallelism,
		flags.FlagNameTerragruntCheck,
		flags.FlagNameTerragruntDiff,
		flags.FlagNameTerragruntDebug,
		flags.FlagNameTerragruntLogLevel,
		flags.FlagNameTerragruntNoColor,
		flags.FlagNameTerragruntModulesThatInclude,
		flags.FlagNameTerragruntFetchDependencyOutputFromState,
		flags.FlagNameTerragruntUsePartialParseConfigCache,
		flags.FlagNameTerragruntIncludeModulePrefix,

		flags.FlagNameTerragruntOverrideAttr,
	}
)

func NewCommand(opts *options.TerragruntOptions) *cli.Command {
	return &cli.Command{
		Name:   CommandName,
		Usage:  "Overwrite settings on nested AWS providers to work around a Terraform bug (issue #13018).",
		Flags:  flags.NewFlags(opts).Filter(TerragruntFlagNames),
		Before: func(ctx *cli.Context) error { return ctx.App.Before(ctx) },
		Action: Action(opts),
	}
}

func Action(opts *options.TerragruntOptions) func(ctx *cli.Context) error {
	return func(ctx *cli.Context) error {
		if len(opts.AwsProviderPatchOverrides) == 0 {
			return errors.WithStackTrace(MissingOverrideAttrError(flags.FlagNameTerragruntOverrideAttr))
		}

		return Run(opts)
	}
}
