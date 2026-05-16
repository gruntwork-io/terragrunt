package cli

import (
	"context"
	"os"

	"github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/gruntwork-io/terragrunt/internal/shell"
	"github.com/gruntwork-io/terragrunt/internal/tf"
	"github.com/gruntwork-io/terragrunt/internal/util"
	"github.com/gruntwork-io/terragrunt/pkg/log"
)

// RunAndExit runs the CLI then os.Exits with ExitCodeFor; top-level panics must be caught by `defer reporter.PanicHandler(...)`.
func (app *App) RunAndExit(args []string, em *tf.DetailedExitCodeMap, reporter *log.PanicReporter) {
	ctx := log.ContextWithLogger(context.Background(), app.l)
	ctx = tf.ContextWithDetailedExitCode(ctx, em)

	err := app.RunContext(ctx, args)
	detailed := app.opts.TerraformCliArgs.Contains(tf.FlagNameDetailedExitCode)

	os.Exit(ExitCodeFor(app.l, args, app.opts.VersionString(), err, em.Final(detailed), reporter))
}

// ExitCodeFor maps a CLI run result to a process exit code; panic-shaped errors route through reporter.
func ExitCodeFor(l log.Logger, args []string, version string, err error, success int, reporter *log.PanicReporter) int {
	if err == nil {
		return success
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

// logRunError emits the user-facing output for a non-nil run error.
func logRunError(l log.Logger, args []string, version string, err error, reporter *log.PanicReporter) {
	if log.IsPanic(err) {
		msg, stack := log.PanicDetails(err)
		reporter.ReportPanic(l, version, msg, stack, args)

		return
	}

	l.Error(err.Error())

	if errStack := errors.ErrorStack(err); errStack != "" {
		l.Trace(errStack)
	}
}
