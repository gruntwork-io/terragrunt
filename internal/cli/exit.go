package cli

import (
	"context"

	"github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/gruntwork-io/terragrunt/internal/panicreport"
	"github.com/gruntwork-io/terragrunt/internal/runner/runall"
	"github.com/gruntwork-io/terragrunt/internal/shell"
	"github.com/gruntwork-io/terragrunt/internal/tf"
	"github.com/gruntwork-io/terragrunt/internal/util"
	"github.com/gruntwork-io/terragrunt/pkg/log"
)

// RunWithExitCode executes the CLI and returns the process exit code; em and reporter must be non-nil.
func (app *App) RunWithExitCode(args []string, em *tf.DetailedExitCodeMap, reporter *panicreport.Reporter) int {
	ctx := log.ContextWithLogger(context.Background(), app.l)
	ctx = tf.ContextWithDetailedExitCode(ctx, em)

	err := app.RunContext(ctx, args)
	detailed := app.opts.TerraformCliArgs.Contains(tf.FlagNameDetailedExitCode)

	return ExitCodeFor(app.l, args, app.opts.VersionString(), err, em.Final(detailed), reporter)
}

// ExitCodeFor maps a CLI run result to a process exit code; reporter must be non-nil.
func ExitCodeFor(l log.Logger, args []string, version string, err error, success int, reporter *panicreport.Reporter) int {
	if err == nil {
		return success
	}

	// User declined a destructive run --all prompt; exit silently like the pre-refactor entry point.
	if errors.Is(err, runall.ErrUserCancelled) {
		return 0
	}

	logRunError(l, args, version, err, reporter)

	if explain := shell.ExplainError(err); len(explain) > 0 {
		l.Errorf("Suggested fixes: \n%s", explain)
	}

	code, codeErr := util.GetExitCode(err)
	if codeErr != nil {
		return 1
	}

	return code
}

// logRunError emits the output a user sees when a run error occurs.
func logRunError(l log.Logger, args []string, version string, err error, reporter *panicreport.Reporter) {
	if panicreport.IsPanic(err) {
		msg, stack := panicreport.PanicDetails(err)
		reporter.ReportPanic(l, version, msg, stack, args)

		return
	}

	l.Error(err.Error())

	if errStack := errors.ErrorStack(err); errStack != "" {
		l.Trace(errStack)
	}
}
