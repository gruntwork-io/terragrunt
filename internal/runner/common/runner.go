// Package common defines minimal runner interfaces to allow multiple implementations.
package common

import (
	"context"

	"github.com/gruntwork-io/terragrunt/internal/component"
	"github.com/gruntwork-io/terragrunt/internal/report"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/pkg/options"
)

// StackRunner is the abstraction for running a stack of units.
// Implemented by runnerpool.Runner and any alternate runner implementations.
type StackRunner interface {
	// Run executes all units in the stack according to the specified Terraform command and options.
	Run(ctx context.Context, l log.Logger, stackOpts *options.TerragruntOptions, rpt *report.Report) error
	// LogUnitDeployOrder logs the order in which units will be deployed for the given Terraform command.
	LogUnitDeployOrder(l log.Logger, opts *options.TerragruntOptions) error
	// JSONUnitDeployOrder returns the deployment order of units as a JSON string.
	JSONUnitDeployOrder(opts *options.TerragruntOptions) (string, error)
	// ListStackDependentUnits returns a map of each unit to the list of units that depend on it.
	ListStackDependentUnits() map[string][]string
	// GetStack retrieves the underlying Stack object managed by this runner.
	GetStack() *component.Stack
}
