// Package configstack contains the logic for managing a stack of Terraform modules (i.e. folders with Terraform templates)
// that you can "spin up" or "spin down" in a single command.
package configstack

import (
	"context"

	"github.com/gruntwork-io/terragrunt/config/hclparse"
	"github.com/gruntwork-io/terragrunt/pkg/log"

	"github.com/gruntwork-io/terragrunt/config"
	"github.com/gruntwork-io/terragrunt/discovery"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/queue"
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
	Lock()
	Unlock()
}

// StackBuilder is the abstraction for building a Stack.
type StackBuilder interface {
	BuildStack(ctx context.Context, l log.Logger, terragruntOptions *options.TerragruntOptions, opts ...Option) (Stack, error)
}

// RunnerPoolStackBuilder implements StackBuilder for RunnerPoolStack
// This is a new builder that uses discovery and queue for abstract handling of run --all/--graph
// Not yet wired into FindStackInSubfolders.
type RunnerPoolStackBuilder struct{}

// BuildStack builds a new RunnerPoolStack using discovery and queue
func (b *RunnerPoolStackBuilder) BuildStack(ctx context.Context, l log.Logger, terragruntOptions *options.TerragruntOptions, opts ...Option) (Stack, error) {
	d := discovery.NewDiscovery(terragruntOptions.WorkingDir).
		WithSuppressParseErrors().
		WithDiscoverDependencies().
		WithDiscoverExternalDependencies()

	discovered, err := d.Discover(ctx, l, terragruntOptions)
	if err != nil {
		return nil, err
	}

	q, err := queue.NewQueue(discovered)
	if err != nil {
		return nil, err
	}

	stack := NewRunnerPoolStack(discovered, q, terragruntOptions)
	for _, opt := range opts {
		opt(stack)
	}

	return stack, nil
}

// FindStackInSubfolders finds all the Terraform modules in the subfolders of the working directory of the given TerragruntOptions and
// assemble them into a Stack object that can be applied or destroyed in a single command
func FindStackInSubfolders(ctx context.Context, l log.Logger, terragruntOptions *options.TerragruntOptions, opts ...Option) (Stack, error) {
	// here will be used different implementation of stack builder which will generate own stack implementation
	builder := &DefaultStackBuilder{}
	return builder.BuildStack(ctx, l, terragruntOptions, opts...)
}
