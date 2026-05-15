package cli

import (
	"context"
	"os"

	"github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/gruntwork-io/terragrunt/internal/panicreport"
	"github.com/gruntwork-io/terragrunt/internal/shell"
	"github.com/gruntwork-io/terragrunt/internal/tf"
	"github.com/gruntwork-io/terragrunt/internal/util"
	"github.com/gruntwork-io/terragrunt/pkg/log"
)

// RunAndExit runs the Terragrunt CLI with the supplied arguments, then
// terminates the process with the exit code computed by ExitCodeFor.
//
// Top-level panics from the surrounding scope must still be caught by a
// `defer reporter.PanicHandler(...)` in the caller; recover() in this
// method would not see them because RunAndExit is not the deferred function.
func (app *App) RunAndExit(args []string, em *tf.DetailedExitCodeMap, reporter *panicreport.Reporter) {
	ctx := log.ContextWithLogger(context.Background(), app.l)
	if em != nil {
		ctx = tf.ContextWithDetailedExitCode(ctx, em)
	}

	err := app.RunContext(ctx, args)
	detailed := app.opts.TerraformCliArgs.Contains(tf.FlagNameDetailedExitCode)

	os.Exit(ExitCodeFor(app.l, args, err, em.Final(detailed), reporter))
}

// ExitCodeFor maps a CLI run result to a process exit code:
//
//   - nil err returns success.
//   - Panics surfaced as returned errors (e.g. cty function.PanicError) are
//     routed through reporter to produce the crash banner + log file in
//     place of the raw error message.
//   - All other errors are logged with the message, the stack at trace
//     level, and any shell-error suggestions.
//
// In every error case, suggested fixes from shell.ExplainError are still
// shown (panics in user-supplied scripts may have explainable causes too)
// and the underlying exit-coder code is returned, defaulting to 1.
func ExitCodeFor(l log.Logger, args []string, err error, success int, reporter *panicreport.Reporter) int {
	if err == nil {
		return success
	}

	logRunError(l, args, err, reporter)

	if explain := shell.ExplainError(err); len(explain) > 0 {
		l.Errorf("Suggested fixes: \n%s", explain)
	}

	code, codeErr := util.GetExitCode(err)
	if codeErr != nil {
		return 1
	}

	return code
}

// logRunError emits the user-facing error output for a non-nil run error:
// the crash banner + log file for panic-shaped errors, or the standard
// message + trace stack otherwise.
func logRunError(l log.Logger, args []string, err error, reporter *panicreport.Reporter) {
	if panicreport.IsPanic(err) {
		reporter.ReportPanic(l, err.Error(), nil, args)
		return
	}

	l.Error(err.Error())

	if errStack := errors.ErrorStack(err); errStack != "" {
		l.Trace(errStack)
	}
}
