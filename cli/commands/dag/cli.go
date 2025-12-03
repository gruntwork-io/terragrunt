// Package dag implements the dag command to interact with the Directed Acyclic Graph (DAG).
// It provides functionality to visualize and analyze dependencies between Terragrunt configurations.
package dag

import (
	"github.com/gruntwork-io/terragrunt/cli/commands/dag/graph"
	"github.com/gruntwork-io/terragrunt/internal/cli"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/pkg/log"
)

const (
	CommandName = "dag"
)

func NewCommand(l log.Logger, opts *options.TerragruntOptions) *cli.Command {
	return &cli.Command{
		Name:  CommandName,
		Usage: "Interact with the Directed Acyclic Graph (DAG).",
		Subcommands: cli.Commands{
			graph.NewCommand(l, opts),
		},
		Action: cli.ShowCommandHelp,
	}
}
