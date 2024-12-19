package stack

import (
	"context"

	"github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/gruntwork-io/terragrunt/options"
)

const (
	generate = "generate"
)

func Run(ctx context.Context, opts *options.TerragruntOptions, subCommand string) error {
	if subCommand == "" {
		return errors.New("No subCommand specified")
	}

	switch subCommand {
	case generate:
		{
			return generateStack(ctx, opts)
		}
	}

	return nil
}

func generateStack(ctx context.Context, opts *options.TerragruntOptions) error {

	return nil
}
