// Defines the Stack and StackBuilder interfaces for managing and building stacks of units in Terragrunt.
// Provides abstractions for stack operations, module orchestration, and configuration handling.
package common

import (
	"context"

	"github.com/gruntwork-io/terragrunt/config"
	"github.com/gruntwork-io/terragrunt/config/hclparse"
	"github.com/gruntwork-io/terragrunt/internal/report"

	"github.com/gruntwork-io/terragrunt/pkg/log"

	"github.com/gruntwork-io/terragrunt/options"
)

// StackRunner is the abstraction for a stack of Terraform modules.
type StackRunner interface {
	Run(ctx context.Context, l log.Logger, opts *options.TerragruntOptions) error
	FindModuleByPath(path string) *Unit
	LogModuleDeployOrder(l log.Logger, terraformCommand string) error
	JSONModuleDeployOrder(terraformCommand string) (string, error)
	Graph(l log.Logger, opts *options.TerragruntOptions)
	ListStackDependentModules() map[string][]string
	GetStack() *Stack
	SetTerragruntConfig(config *config.TerragruntConfig)
	SetParseOptions(parserOptions []hclparse.Option)
	SetReport(report *report.Report)
}

// StackRunnerBuilder is the abstraction for building a StackRunner.
type StackRunnerBuilder interface {
	Build(ctx context.Context, l log.Logger, terragruntOptions *options.TerragruntOptions, opts ...Option) (Stack, error)
}
