package validateinputs

import (
	"github.com/gruntwork-io/terragrunt/cli/flags"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/pkg/cli"
)

const (
	CommandName = "validate-inputs"
)

var (
	TerragruntFlagNames = []string{
		flags.FlagTerragruntStrictValidate,
	}
)

func NewCommand(opts *options.TerragruntOptions) *cli.Command {
	return &cli.Command{
		Name:   CommandName,
		Usage:  "Checks if the terragrunt configured inputs align with the terraform defined variables.",
		Flags:  flags.NewFlags(opts).Filter(TerragruntFlagNames),
		Action: func(ctx *cli.Context) error { return Run(opts.OptionsFromContext(ctx)) },
	}
}
