package outputmodulegroups

import (
	"context"
	"fmt"

	"github.com/gruntwork-io/terragrunt/configstack"
	"github.com/gruntwork-io/terragrunt/options"
)

// Run is the entry point for the output-module-groups command.
func Run(ctx context.Context, opts *options.TerragruntOptions) error {
	stack, err := configstack.FindStackInSubfolders(ctx, opts)
	if err != nil {
		return fmt.Errorf("could not find stack in subfolders: %w", err)
	}

	js, err := stack.JSONModuleDeployOrder(opts.TerraformCommand)
	if err != nil {
		return fmt.Errorf("could not get JSON module deploy order: %w", err)
	}

	_, err = fmt.Fprintf(opts.Writer, "%s\n", js)
	if err != nil {
		return fmt.Errorf("could not write JSON module deploy order: %w", err)
	}

	return nil
}
