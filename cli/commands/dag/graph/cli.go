// Package graph implements the terragrunt dag graph command which generates a visual
// representation of the Terragrunt dependency graph in DOT language format.
package graph

import (
	"github.com/gruntwork-io/terragrunt/cli/commands/common/graph"
	"github.com/gruntwork-io/terragrunt/cli/commands/common/runall"
	"github.com/gruntwork-io/terragrunt/cli/commands/run"
	"github.com/gruntwork-io/terragrunt/cli/flags"
	"github.com/gruntwork-io/terragrunt/configstack"
	"github.com/gruntwork-io/terragrunt/internal/cli"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/pkg/log"
)

const (
	CommandName = "graph"
)

func NewCommand(l log.Logger, opts *options.TerragruntOptions, _ flags.Prefix) *cli.Command {
	cmd := &cli.Command{
		Name:      CommandName,
		Usage:     "Graph the Directed Acyclic Graph (DAG) in DOT language.",
		UsageText: "terragrunt dag graph",
		Action: func(ctx *cli.Context) error {
			return Run(ctx, l, opts)
		},
	}

	cmd = runall.WrapCommand(l, opts, cmd, run.Run)
	cmd = graph.WrapCommand(l, opts, cmd, run.Run)

	return cmd
}

func Run(ctx *cli.Context, l log.Logger, opts *options.TerragruntOptions) error {
	stack, err := configstack.FindStackInSubfolders(ctx, l, opts)
	if err != nil {
		return err
	}

	stack.Graph(l, opts)

	return nil
}
