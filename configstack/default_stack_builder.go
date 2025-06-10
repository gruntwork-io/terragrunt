package configstack

import (
	"context"

	"github.com/gruntwork-io/terragrunt/config"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/telemetry"
)

// DefaultStackBuilder implements StackBuilder for DefaultStack
type DefaultStackBuilder struct{}

// BuildStack builds a new DefaultStack.
func (b *DefaultStackBuilder) BuildStack(ctx context.Context, l log.Logger, terragruntOptions *options.TerragruntOptions, opts ...Option) (Stack, error) {
	var terragruntConfigFiles []string

	err := telemetry.TelemeterFromContext(ctx).Collect(ctx, "find_files_in_path", map[string]any{
		"working_dir": terragruntOptions.WorkingDir,
	}, func(_ context.Context) error {
		result, err := config.FindConfigFilesInPath(terragruntOptions.WorkingDir, terragruntOptions)
		if err != nil {
			return err
		}

		terragruntConfigFiles = result

		return nil
	})

	if err != nil {
		return nil, err
	}

	stack := NewDefaultStack(l, terragruntOptions, opts...)
	if err := stack.createStackForTerragruntConfigPaths(ctx, l, terragruntConfigFiles); err != nil {
		return nil, err
	}

	return stack, nil
}
