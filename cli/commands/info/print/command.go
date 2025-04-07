package print

import (
	"github.com/gruntwork-io/terragrunt/cli/flags"
	"github.com/gruntwork-io/terragrunt/internal/cli"
	"github.com/gruntwork-io/terragrunt/options"
)

const (
	CommandName = "print"
)

func NewListFlags(_ *options.TerragruntOptions, _ flags.Prefix) cli.Flags {
	return cli.Flags{}
}

func NewCommand(opts *options.TerragruntOptions, prefix flags.Prefix) *cli.Command {
	prefix = prefix.Append(CommandName)

	return &cli.Command{
		Name:                 CommandName,
		Usage:                "Print out a short description of Terragrunt context.",
		UsageText:            "terragrunt info print",
		Flags:                NewListFlags(opts, prefix),
		ErrorOnUndefinedFlag: true,
		Action:               infoAction(opts),
	}
}

func infoAction(opts *options.TerragruntOptions) func(ctx *cli.Context) error {

	return nil
}
