package stack

import (
	"context"

	"github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/gruntwork-io/terragrunt/options"
)

func Run(ctx context.Context, opts *options.TerragruntOptions, command string) error {
	if command == "" {
		return errors.New("No terraform command specified")
	}

	return nil
}
