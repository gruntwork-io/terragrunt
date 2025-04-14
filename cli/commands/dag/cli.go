// Package dag implements the dag command to interact with the Directed Acyclic Graph (DAG).
// It provides functionality to visualize and analyze dependencies between Terragrunt configurations.
package dag

import (
	"github.com/gruntwork-io/terragrunt/cli/commands/dag/graph"
	"github.com/gruntwork-io/terragrunt/cli/flags"
	"github.com/gruntwork-io/terragrunt/internal/cli"
	"github.com/gruntwork-io/terragrunt/options"
)

const (
	CommandName = "dag"
)

func NewCommand(opts *options.TerragruntOptions) *cli.Command {
	prefix := flags.Prefix{CommandName}

	return &cli.Command{
		Name:  CommandName,
		Usage: "Interact with the Directed Acyclic Graph (DAG).",
		Subcommands: cli.Commands{
			graph.NewCommand(opts, prefix),
		},
		ErrorOnUndefinedFlag: true,
		Action:               cli.ShowCommandHelp,
	}
}
