package stack

import (
	"context"

	"github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/gruntwork-io/terragrunt/options"
)

func Run(ctx context.Context, opts *options.TerragruntOptions) error {
	if opts.TerraformCommand == "" {
		return errors.New("No terraform command specified")
	}

	return Run(ctx, opts)
}
