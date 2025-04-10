package graph

import (
	"github.com/gruntwork-io/terragrunt/cli/commands/common/graph"
	"github.com/gruntwork-io/terragrunt/cli/commands/common/runall"
	"github.com/gruntwork-io/terragrunt/cli/flags"
	"github.com/gruntwork-io/terragrunt/configstack"
	"github.com/gruntwork-io/terragrunt/internal/cli"
	"github.com/gruntwork-io/terragrunt/options"
)

const (
	CommandName = "graph"
)

func NewCommand(opts *options.TerragruntOptions, prefix flags.Prefix) *cli.Command {
	prefix = prefix.Append(CommandName)

	cmd := &cli.Command{
		Name:                 CommandName,
		Usage:                "Graph the Directed Acyclic Graph (DAG) in DOT language.",
		UsageText:            "terragrunt dag graph",
		ErrorOnUndefinedFlag: true,
		Action: func(ctx *cli.Context) error {
			return Run(ctx, opts)
		},
	}

	cmd = runall.WrapCommand(opts, cmd)
	cmd = graph.WrapCommand(opts, cmd)

	return cmd
}

func Run(ctx *cli.Context, opts *options.TerragruntOptions) error {
	stack, err := configstack.FindStackInSubfolders(ctx, opts)
	if err != nil {
		return err
	}

	stack.Graph(opts)

	return nil
}
