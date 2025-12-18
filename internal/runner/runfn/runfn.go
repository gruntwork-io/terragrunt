// Package runfn provides a global function reference for running terragrunt commands.
// This exists to break import cycles between packages that need to call the run function.
package runfn

import (
	"context"

	"github.com/gruntwork-io/terragrunt/internal/report"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/pkg/log"
)

// Func is the function signature for running terragrunt commands.
type Func func(ctx context.Context, l log.Logger, opts *options.TerragruntOptions, r *report.Report) error

// Run is the global run function that will be set by the run package during initialization.
var Run Func
