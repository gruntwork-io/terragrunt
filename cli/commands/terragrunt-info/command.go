package terragruntinfo

import (
	"github.com/gruntwork-io/terragrunt/cli/commands"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/pkg/cli"
)

const (
	CommandName = "terragrunt-info"
)

func NewCommand(opts *options.TerragruntOptions) *cli.Command {
	return &cli.Command{
		Name:   CommandName,
		Usage:  "Emits limited terragrunt state on stdout and exits.",
		Action: commands.Action(opts, Run),
	}
}
