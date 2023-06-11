package command

import (
	"github.com/gruntwork-io/terragrunt/configstack"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/pkg/cli"
)

const (
	cmdGraphDependencies = "graph-dependencies"
)

func NewGraphDependenciesCommand(opts *options.TerragruntOptions) *cli.Command {
	command := &cli.Command{
		Name:   "graph-dependencies",
		Usage:  "Prints the terragrunt dependency graph to stdout.",
		Action: func(ctx *cli.Context) error { return runGraphDependencies(opts) },
	}

	return command
}

// Run graph dependencies prints the dependency graph to stdout
func runGraphDependencies(terragruntOptions *options.TerragruntOptions) error {
	stack, err := configstack.FindStackInSubfolders(terragruntOptions, nil)
	if err != nil {
		return err
	}

	// Exit early if the operation wanted is to get the graph
	stack.Graph(terragruntOptions)
	return nil
}
