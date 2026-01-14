package run

import (
	"context"

	"github.com/gruntwork-io/terragrunt/internal/report"
	"github.com/gruntwork-io/terragrunt/internal/runner/runcfg"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/pkg/options"
)

// Runner implements runcfg.TerragruntRunner by wrapping the Run function.
type Runner struct{}

// NewRunner creates a new TerragruntRunner.
func NewRunner() *Runner {
	return &Runner{}
}

// Run implements runcfg.TerragruntRunner.
func (r *Runner) Run(ctx context.Context, l log.Logger, opts *options.TerragruntOptions, rep runcfg.Report) error {
	// Convert the interface to the concrete type if needed
	var execReport *report.Report
	if rep != nil {
		if rpt, ok := rep.(*report.Report); ok {
			execReport = rpt
		}
	}

	if execReport == nil {
		execReport = report.NewReport()
	}

	return Run(ctx, l, opts, execReport)
}

// Ensure Runner implements the interface
var _ runcfg.TerragruntRunner = (*Runner)(nil)
