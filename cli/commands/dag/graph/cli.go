// Package graph implements the terragrunt dag graph command which generates a visual
// representation of the Terragrunt dependency graph in DOT language format.
package graph

import (
	"github.com/gruntwork-io/terragrunt/cli/commands/common/graph"
	"github.com/gruntwork-io/terragrunt/cli/commands/common/runall"
	"github.com/gruntwork-io/terragrunt/cli/commands/run"
	"github.com/gruntwork-io/terragrunt/cli/flags"
	"github.com/gruntwork-io/terragrunt/internal/cli"
	"github.com/gruntwork-io/terragrunt/internal/runner"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/pkg/log"
)

const (
	CommandName = "graph"
)

func NewCommand(l log.Logger, opts *options.TerragruntOptions, prefix flags.Prefix) *cli.Command {
	cmd := &cli.Command{
		Name:      CommandName,
		Usage:     "Graph the Directed Acyclic Graph (DAG) in DOT language.",
		UsageText: "terragrunt dag graph",
		Action: func(ctx *cli.Context) error {
			return Run(ctx, l, opts)
		},
	}

	// Add queue flags to respect TG_QUEUE_INCLUDE_EXTERNAL environment variable
	cmd.Flags = append(cmd.Flags, run.NewQueueFlags(l, opts, prefix)...)

	cmd = runall.WrapCommand(l, opts, cmd, run.Run, true)
	cmd = graph.WrapCommand(l, opts, cmd, run.Run, true)

	return cmd
}

func Run(ctx *cli.Context, l log.Logger, opts *options.TerragruntOptions) error {
	stack, err := runner.FindStackInSubfolders(ctx, l, opts)
	if err != nil {
		return err
	}

	if err := stack.GetStack().Units.WriteDot(l, opts.Writer, opts); err != nil {
		l.Warnf("Failed to graph dot: %v", err)
	}

	return nil
}
