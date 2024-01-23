package destroy_graph

import (
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/pkg/cli"
)

const (
	CommandName = "destroy-graph"
)

func NewFlags(opts *options.TerragruntOptions) cli.Flags {
	return cli.Flags{
		&cli.GenericFlag[string]{
			Name:        "destroy-graph-root",
			Destination: &opts.DestroyGraphRoot,
			Usage:       "Destroy graph root directory.",
		},
	}
}

func NewCommand(opts *options.TerragruntOptions) *cli.Command {
	return &cli.Command{
		Name:                   CommandName,
		Usage:                  "Destroy dependent modules and module itself.",
		DisallowUndefinedFlags: true,
		Flags:                  NewFlags(opts).Sort(),
		Action: func(ctx *cli.Context) error {
			return Run(opts.OptionsFromContext(ctx))
		},
	}
}
