// Package configstack contains the logic for managing a stack of Terraform modules (i.e. folders with Terraform templates)
// that you can "spin up" or "spin down" in a single command.
package configstack

import (
	"context"

	"github.com/gruntwork-io/terragrunt/internal/experiment"

	"github.com/gruntwork-io/terragrunt/config/hclparse"
	"github.com/gruntwork-io/terragrunt/internal/report"
	"github.com/gruntwork-io/terragrunt/pkg/log"

	"github.com/gruntwork-io/terragrunt/config"
	"github.com/gruntwork-io/terragrunt/options"
)

// Stack is the abstraction for a stack of Terraform modules.
type Stack interface {
	String() string
	LogModuleDeployOrder(l log.Logger, terraformCommand string) error
	JSONModuleDeployOrder(terraformCommand string) (string, error)
	Graph(l log.Logger, opts *options.TerragruntOptions)
	Run(ctx context.Context, l log.Logger, opts *options.TerragruntOptions) error
	GetModuleRunGraph(terraformCommand string) ([]TerraformModules, error)
	ListStackDependentModules() map[string][]string
	Modules() TerraformModules
	FindModuleByPath(path string) *TerraformModule
	SetTerragruntConfig(config *config.TerragruntConfig)
	GetTerragruntConfig() *config.TerragruntConfig
	SetParseOptions(parserOptions []hclparse.Option)
	GetParseOptions() []hclparse.Option
	SetReport(report *report.Report)
	GetReport() *report.Report
	Lock()
	Unlock()
}

// StackBuilder is the abstraction for building a Stack.
type StackBuilder interface {
	BuildStack(ctx context.Context, l log.Logger, terragruntOptions *options.TerragruntOptions, opts ...Option) (Stack, error)
}

// FindStackInSubfolders finds all the Terraform modules in the subfolders of the working directory of the given TerragruntOptions and
// assemble them into a Stack object that can be applied or destroyed in a single command
func FindStackInSubfolders(ctx context.Context, l log.Logger, terragruntOptions *options.TerragruntOptions, opts ...Option) (Stack, error) {
	if terragruntOptions.Experiments.Evaluate(experiment.RunnerPool) {
		l.Infof("Using RunnerPoolStackBuilder to build stack for %s", terragruntOptions.WorkingDir)

		builder := NewRunnerPoolStackBuilder()

		return builder.BuildStack(ctx, l, terragruntOptions, opts...)
	}

	builder := &DefaultStackBuilder{}

	return builder.BuildStack(ctx, l, terragruntOptions, opts...)
}
