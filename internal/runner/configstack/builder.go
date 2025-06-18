package configstack

import (
	"context"

	"github.com/gruntwork-io/terragrunt/internal/runner/common"

	"github.com/gruntwork-io/terragrunt/config"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/telemetry"
)

// ConfigStackBuilder implements StackBuilder for Runner
type ConfigStackBuilder struct{}

// BuildStack builds a new Runner.
func (b *ConfigStackBuilder) BuildStack(ctx context.Context, l log.Logger, terragruntOptions *options.TerragruntOptions, opts ...common.Option) (common.StackRunner, error) {
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

	stack := NewRunner(l, terragruntOptions, opts...)
	if err := stack.createStackForTerragruntConfigPaths(ctx, l, terragruntConfigFiles); err != nil {
		return nil, err
	}

	return stack, nil
}
