package hclfmt

import (
	"github.com/gruntwork-io/terragrunt/cli/flags"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/pkg/cli"
)

const (
	CommandName = "hclfmt"
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

		flags.FlagNameTerragruntHCLFmt,
	}
)

func NewCommand(opts *options.TerragruntOptions) *cli.Command {
	return &cli.Command{
		Name:   CommandName,
		Usage:  "Recursively find hcl files and rewrite them into a canonical format.",
		Flags:  flags.NewFlags(opts).Filter(TerragruntFlagNames),
		Before: func(ctx *cli.Context) error { return ctx.App.Before(ctx) },
		Action: func(ctx *cli.Context) error { return Run(opts) },
	}
}
