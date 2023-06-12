package graphdependencies

import (
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/pkg/cli"
)

const (
	CommandName = "graph-dependencies"
)

func NewCommand(opts *options.TerragruntOptions) *cli.Command {
	command := &cli.Command{
		Name:   CommandName,
		Usage:  "Prints the terragrunt dependency graph to stdout.",
		Action: func(ctx *cli.Context) error { return Run(opts) },
	}

	return command
}
