package awsproviderpatch

import (
	"github.com/gruntwork-io/terragrunt/cli/commands"
	"github.com/gruntwork-io/terragrunt/cli/flags"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/pkg/cli"
)

const (
	CommandName = "aws-provider-patch"
)

var (
	TerragruntFlagNames = []string{
		flags.FlagNameTerragruntOverrideAttr,
	}
)

func NewCommand(opts *options.TerragruntOptions) *cli.Command {
	return &cli.Command{
		Name:   CommandName,
		Usage:  "Overwrite settings on nested AWS providers to work around a Terraform bug (issue #13018).",
		Flags:  flags.NewFlags(opts).Filter(TerragruntFlagNames),
		Action: commands.Action(opts, Run),
	}
}
