package outputmodulegroups

import (
	"context"
	"fmt"

	"github.com/gruntwork-io/terragrunt/configstack"
	"github.com/gruntwork-io/terragrunt/options"
)

func Run(ctx context.Context, opts *options.TerragruntOptions) error {
	stack, err := configstack.FindStackInSubfolders(ctx, opts, nil)
	if err != nil {
		return err
	}

	js, err := stack.JsonModuleDeployOrder(opts.TerraformCommand)
	if err != nil {
		return err
	}

	_, err = fmt.Fprintf(opts.Writer, "%s\n", js)
	if err != nil {
		return err
	}

	return nil
}
