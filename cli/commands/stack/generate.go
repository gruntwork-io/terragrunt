package stack

import (
	"context"
	"path/filepath"

	"github.com/gruntwork-io/terragrunt/config"
	"github.com/gruntwork-io/terragrunt/options"
)

func generateStack(ctx context.Context, opts *options.TerragruntOptions) error {
	opts.TerragruntStackConfigPath = filepath.Join(opts.WorkingDir, defaultStackFile)
	opts.Logger.Infof("Generating stack from %s", opts.TerragruntStackConfigPath)

	return config.GenerateStacks(ctx, opts)
}
