package graphdependencies

import (
	"github.com/gruntwork-io/terragrunt/configstack"
	"github.com/gruntwork-io/terragrunt/options"
)

// Run graph dependencies prints the dependency graph to stdout
func Run(terragruntOptions *options.TerragruntOptions) error {
	stack, err := configstack.FindStackInSubfolders(terragruntOptions, nil)
	if err != nil {
		return err
	}

	// Exit early if the operation wanted is to get the graph
	stack.Graph(terragruntOptions)
	return nil
}
