// Package common provide base components for implementing runners.
package common

import (
	"context"

	"github.com/gruntwork-io/terragrunt/config"
	"github.com/gruntwork-io/terragrunt/config/hclparse"
	"github.com/gruntwork-io/terragrunt/internal/component"
	"github.com/gruntwork-io/terragrunt/internal/report"

	"github.com/gruntwork-io/terragrunt/pkg/log"

	"github.com/gruntwork-io/terragrunt/options"
)

// StackRunner is the abstraction for running stack of units.
type StackRunner interface {
	// Run - Execute all units in the stack according to the specified Terraform command and options.
	Run(ctx context.Context, l log.Logger, opts *options.TerragruntOptions) error
	// LogUnitDeployOrder Log the order in which units will be deployed for the given Terraform command.
	LogUnitDeployOrder(l log.Logger, terraformCommand string) error
	// JSONUnitDeployOrder Return the deployment order of units as a JSON string for the specified Terraform command.
	JSONUnitDeployOrder(terraformCommand string) (string, error)
	// ListStackDependentUnits Build and return a map of each unit to the list of units that depend on it.
	ListStackDependentUnits() map[string][]string
	// GetStack Retrieve the underlying Stack object managed by this runner.
	GetStack() *component.Stack
	// SetTerragruntConfig Set the child Terragrunt configuration for the stack.
	SetTerragruntConfig(config *config.TerragruntConfig)
	// SetParseOptions Set the parser options used for HCL parsing in the stack.
	SetParseOptions(parserOptions []hclparse.Option)
	// SetReport Attach a report object to the stack for collecting run data and summaries.
	SetReport(report *report.Report)
}
