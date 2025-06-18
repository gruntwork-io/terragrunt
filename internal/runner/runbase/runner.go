// Package runbase provide base components for implementing runners.
package runbase

import (
	"context"

	"github.com/gruntwork-io/terragrunt/config"
	"github.com/gruntwork-io/terragrunt/config/hclparse"
	"github.com/gruntwork-io/terragrunt/internal/report"

	"github.com/gruntwork-io/terragrunt/pkg/log"

	"github.com/gruntwork-io/terragrunt/options"
)

// StackRunner is the abstraction for running stack of units.
type StackRunner interface {
	Run(ctx context.Context, l log.Logger, opts *options.TerragruntOptions) error

	LogUnitDeployOrder(l log.Logger, terraformCommand string) error
	JSONUnitDeployOrder(terraformCommand string) (string, error)
	ListStackDependentUnits() map[string][]string
	GetStack() *Stack
	SetTerragruntConfig(config *config.TerragruntConfig)
	SetParseOptions(parserOptions []hclparse.Option)
	SetReport(report *report.Report)
}
