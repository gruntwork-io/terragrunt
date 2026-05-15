// Package dag implements the dag command to interact with the Directed Acyclic Graph (DAG).
// It provides functionality to visualize and analyze dependencies between Terragrunt configurations.
package dag

import (
	"github.com/gruntwork-io/terragrunt/internal/cli/commands/dag/graph"
	"github.com/gruntwork-io/terragrunt/internal/clihelper"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/pkg/options"
)

const (
	CommandName = "dag"
)

func NewCommand(l log.Logger, opts *options.TerragruntOptions, v *venv.Venv) *clihelper.Command {
	return &clihelper.Command{
		Name:  CommandName,
		Usage: "Interact with the Directed Acyclic Graph (DAG).",
		Subcommands: clihelper.Commands{
			graph.NewCommand(l, opts, v),
		},
		Action: clihelper.ShowCommandHelp,
	}
}
