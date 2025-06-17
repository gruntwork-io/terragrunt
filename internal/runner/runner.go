// Package runner provides logic for applying Stacks and Units Terragrunt.
package runner

import (
	"context"

	configstack2 "github.com/gruntwork-io/terragrunt/internal/runner/configstack"
	"github.com/gruntwork-io/terragrunt/internal/runner/model"

	"github.com/gruntwork-io/terragrunt/internal/runner/runnerpool"

	"github.com/gruntwork-io/terragrunt/internal/experiment"
	"github.com/gruntwork-io/terragrunt/pkg/log"

	"github.com/gruntwork-io/terragrunt/options"
)

// FindStackInSubfolders finds all the Terraform modules in the subfolders of the working directory of the given TerragruntOptions and
// assemble them into a Stack object that can be applied or destroyed in a single command
func FindStackInSubfolders(ctx context.Context, l log.Logger, terragruntOptions *options.TerragruntOptions, opts ...model.Option) (model.Stack, error) {
	if terragruntOptions.Experiments.Evaluate(experiment.RunnerPool) {
		l.Infof("Using RunnerPoolStackBuilder to build stack for %s", terragruntOptions.WorkingDir)

		builder := runnerpool.NewRunnerPoolStackBuilder()

		return builder.BuildStack(ctx, l, terragruntOptions, opts...)
	}

	builder := &configstack2.DefaultStackBuilder{}

	return builder.BuildStack(ctx, l, terragruntOptions, opts...)
}
