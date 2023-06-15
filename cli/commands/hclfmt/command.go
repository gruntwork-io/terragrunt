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
	TerragruntFlagNames = append(flags.CommonFlagNames,
		flags.FlagNameTerragruntConfig,
		flags.FlagNameTerragruntHCLFmt,
		flags.FlagNameTerragruntCheck,
		flags.FlagNameTerragruntDiff,
	)
)

func NewCommand(opts *options.TerragruntOptions) *cli.Command {
	return &cli.Command{
		Name:   CommandName,
		Usage:  "Recursively find hcl files and rewrite them into a canonical format.",
		Flags:  flags.NewFlags(opts).Filter(TerragruntFlagNames),
		Before: func(ctx *cli.Context) error { return ctx.App.Before(ctx) },
		Action: func(ctx *cli.Context) error { return Run(opts.FromContext(ctx)) },
	}
}
