// Package configstack contains the logic for managing a stack of Terraform modules (i.e. folders with Terraform templates)
// that you can "spin up" or "spin down" in a single command.
package stack

import (
	"context"

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
	GetModuleRunGraph(terraformCommand string) ([]Units, error)
	ListStackDependentModules() map[string][]string
	Modules() Units
	FindModuleByPath(path string) *Unit
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
