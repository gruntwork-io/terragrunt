package destroy_graph

import (
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/pkg/cli"
)

const (
	CommandName = "destroy-graph"
)

func NewCommand(opts *options.TerragruntOptions) *cli.Command {
	return &cli.Command{
		Name:                   CommandName,
		Usage:                  "Destroy dependent modules and module itself.",
		DisallowUndefinedFlags: true,
		Action: func(ctx *cli.Context) error {
			return Run(opts.OptionsFromContext(ctx))
		},
	}
}
