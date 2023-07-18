package hclfmt

import (
	"github.com/gruntwork-io/terragrunt/cli/commands"
	"github.com/gruntwork-io/terragrunt/cli/flags"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/pkg/cli"
)

const (
	CommandName = "hclfmt"
)

var (
	TerragruntFlagNames = []string{
		flags.FlagNameTerragruntHCLFmt,
		flags.FlagNameTerragruntCheck,
		flags.FlagNameTerragruntDiff,
	}
)

func NewCommand(opts *options.TerragruntOptions) *cli.Command {
	return &cli.Command{
		Name:   CommandName,
		Usage:  "Recursively find hcl files and rewrite them into a canonical format.",
		Flags:  flags.NewFlags(opts).Filter(TerragruntFlagNames),
		Action: commands.Action(opts, Run),
	}
}
