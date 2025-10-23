// Package graph implements the terragrunt dag graph command which generates a visual
// representation of the Terragrunt dependency graph in DOT language format.
//
// DEPRECATED: This command is deprecated. Use 'terragrunt list --format=dot --dag --dependencies --external' instead.
package graph

import (
	"github.com/gruntwork-io/terragrunt/cli/commands/list"
	"github.com/gruntwork-io/terragrunt/cli/flags"
	"github.com/gruntwork-io/terragrunt/internal/cli"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/pkg/log"
)

const (
	CommandName = "graph"
)

func NewCommand(l log.Logger, opts *options.TerragruntOptions, _ flags.Prefix) *cli.Command {
	return &cli.Command{
		Name:      CommandName,
		Usage:     "Graph the Directed Acyclic Graph (DAG) in DOT language. DEPRECATED: Use 'list --format=dot --dag --dependencies --external' instead.",
		UsageText: "terragrunt dag graph",
		Action: func(ctx *cli.Context) error {
			return Run(ctx, l, opts)
		},
	}
}

func Run(ctx *cli.Context, l log.Logger, opts *options.TerragruntOptions) error {
	l.Warnf("The 'dag graph' command is deprecated. Please use 'terragrunt list --format=dot --dag --dependencies --external' instead.")

	// Create list options configured for DAG graph behavior
	listOpts := list.NewOptions(opts)
	listOpts.Format = list.FormatDot
	listOpts.Mode = list.ModeDAG
	listOpts.Dependencies = true
	listOpts.External = true
	listOpts.Hidden = true

	return list.Run(ctx, l, listOpts)
}
