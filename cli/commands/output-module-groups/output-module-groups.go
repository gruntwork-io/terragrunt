package outputmodulegroups

import (
	"context"
	"fmt"

	"github.com/gruntwork-io/terragrunt/internal/runner"

	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/pkg/log"
)

func Run(ctx context.Context, l log.Logger, opts *options.TerragruntOptions) error {
	stack, err := runner.FindStackInSubfolders(ctx, l, opts)
	if err != nil {
		return err
	}

	js, err := stack.JSONUnitDeployOrder(opts.TerraformCommand)
	if err != nil {
		return err
	}

	_, err = fmt.Fprintf(opts.Writer, "%s\n", js)
	if err != nil {
		return err
	}

	return nil
}
