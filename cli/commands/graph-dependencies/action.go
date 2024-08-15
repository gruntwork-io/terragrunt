package graphdependencies

import (
	"context"
	"fmt"

	"github.com/gruntwork-io/terragrunt/configstack"
	"github.com/gruntwork-io/terragrunt/options"
)

// Run graph dependencies prints the dependency graph to stdout.
func Run(ctx context.Context, opts *options.TerragruntOptions) error {
	stack, err := configstack.FindStackInSubfolders(ctx, opts)
	if err != nil {
		return fmt.Errorf("error finding stack: %w", err)
	}

	// Exit early if the operation wanted is to get the graph
	stack.Graph(opts)

	return nil
}
