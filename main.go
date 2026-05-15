package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"strconv"
	"strings"
	"time"

	"github.com/gruntwork-io/terragrunt/internal/cli"
	"github.com/gruntwork-io/terragrunt/internal/cli/flags/global"
	"github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/gruntwork-io/terragrunt/internal/shell"
	"github.com/gruntwork-io/terragrunt/internal/tf"
	"github.com/gruntwork-io/terragrunt/internal/util"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/pkg/log/format"
	"github.com/gruntwork-io/terragrunt/pkg/options"
)

const (
	// terragruntIssueURL is shown to the user in the panic banner so crashes
	// have a single canonical reporting location.
	terragruntIssueURL = "https://github.com/gruntwork-io/terragrunt/issues"

	// crashLogPrefix names the crash log file. The full filename is
	// "<prefix>-<RFC3339-seconds>-<pid>.log" so concurrent runs don't collide.
	crashLogPrefix = "terragrunt-crash"
	// crashLogFileTimeLayout uses second precision — nanoseconds add no value
	// and make filenames unwieldy.
	crashLogFileTimeLayout = "20060102T150405Z"

	// crashFileMode is u=rw because the crash log may surface env hints, cwd,
	// and command-line arguments. Adjust upward only if a CI artifact uploader
	// runs as a different user than the runner.
	crashFileMode os.FileMode = 0o600

	// panicExitCode mirrors OpenTofu's choice of 11 — distinct from
	// Terraform's detailed exit codes (0/1/2) and matches SIGSEGV's signal
	// number, which most runtime panics morally resemble.
	panicExitCode = 11

	// allGoroutineStackBufSize caps the buffer used for runtime.Stack(_, true).
	// 1 MiB is enough for thousands of goroutines without runaway allocation.
	allGoroutineStackBufSize = 1 << 20

	panicOutput = `
***************************** TERRAGRUNT CRASH *****************************

Terragrunt crashed! This is always indicative of a bug within Terragrunt.
Please report the crash with Terragrunt[1] so that we can fix this.

When reporting bugs, please include your Terragrunt version and the panic
report file, and any additional information which may help replicate the
issue.

[1]: %s

***************************** TERRAGRUNT CRASH *****************************
`

	crashLogTemplate = `Terragrunt panic report
======================

Timestamp: %s
Terragrunt version: %s
Build commit: %s
Build modified: %t
Go runtime: %s
GOOS/GOARCH: %s/%s
NumCPU: %d
GOMAXPROCS: %d
NumGoroutine: %d
PID: %d
Working directory: %s
Command line: %s

Panic: %s

Stack trace:
%s

All goroutines:
%s
`
)

// terragruntApp groups the side-effecting hooks the panic path depends on so
// they can be stubbed in tests without process-level state.
type terragruntApp struct {
	now              func() time.Time
	getwd            func() (string, error)
	getPID           func() int
	writeLog         func(string, []byte, os.FileMode) error
	allGoroutineDump func() string
	buildInfo        func() (commit string, modified bool)
}

func main() {
	app := newTerragruntApp()

	exitCode := tf.NewDetailedExitCodeMap()
	opts := options.NewTerragruntOptions()

	l := log.New(
		log.WithOutput(opts.Writers.ErrWriter),
		log.WithLevel(options.DefaultLogLevel),
		log.WithFormatter(format.NewFormatter(format.NewPrettyFormatPlaceholders())),
	)

	if err := global.NewLogLevelFlag(l, opts, nil).Parse(os.Args); err != nil {
		l.Errorf("An error has occurred: %v", err)
		os.Exit(1)
	}

	// errors.Recover MUST be the deferred function itself — wrapping it inside
	// another deferred closure makes its internal recover() return nil and
	// silently swallow panics.
	defer errors.Recover(func(cause error) {
		app.reportPanic(l, cause, opts)
		os.Exit(panicExitCode)
	})

	cliApp := cli.NewApp(l, opts)
	ctx := setupContext(l, exitCode)
	err := cliApp.RunContext(ctx, os.Args)

	app.checkForErrorsAndExit(l, app.finalExitCode(exitCode, opts), opts)(err)
}

// Private helper functions

func newTerragruntApp() *terragruntApp {
	return &terragruntApp{
		now:              time.Now,
		getwd:            os.Getwd,
		getPID:           os.Getpid,
		writeLog:         os.WriteFile,
		allGoroutineDump: dumpAllGoroutines,
		buildInfo:        readBuildInfo,
	}
}

func (app *terragruntApp) finalExitCode(exitCode *tf.DetailedExitCodeMap, opts *options.TerragruntOptions) int {
	if opts.TerraformCliArgs.Contains(tf.FlagNameDetailedExitCode) {
		return exitCode.GetFinalDetailedExitCode()
	}

	return exitCode.GetFinalExitCode()
}

func (app *terragruntApp) checkForErrorsAndExit(l log.Logger, exitCode int, opts *options.TerragruntOptions) func(error) {
	return func(err error) {
		if err == nil {
			os.Exit(exitCode)
		}

		if errors.IsFunctionPanic(err) {
			app.reportPanic(l, err, opts)
			os.Exit(panicExitCode)
		}

		exitCoder, exitCodeErr := util.GetExitCode(err)
		if exitCodeErr != nil {
			exitCoder = 1
		}

		l.Error(err.Error())

		if errStack := errors.ErrorStack(err); errStack != "" {
			l.Trace(errStack)
		}

		if explain := shell.ExplainError(err); len(explain) > 0 {
			l.Errorf("Suggested fixes: \n%s", explain)
		}

		os.Exit(exitCoder)
	}
}

func (app *terragruntApp) reportPanic(l log.Logger, err error, opts *options.TerragruntOptions) {
	if err == nil {
		return
	}

	crashLogPath, crashLog, writeErr := app.writeCrashLog(err, opts, os.Args)

	l.Error(fmt.Sprintf(panicOutput, terragruntIssueURL))

	if writeErr != nil {
		app.reportPanicWriteFailure(l, writeErr, crashLog)
		return
	}

	l.Errorf("A panic report has been saved to: %s", crashLogPath)
	l.Errorf("Please report this issue at %s and attach the panic report.", terragruntIssueURL)
}

func (app *terragruntApp) reportPanicWriteFailure(l log.Logger, writeErr error, crashLog string) {
	l.Errorf("Unable to write crash report: %v", writeErr)
	l.Errorf("Please report this issue at %s and include the crash report output below.", terragruntIssueURL)
	l.Error(crashLog)
}

func (app *terragruntApp) writeCrashLog(err error, opts *options.TerragruntOptions, commandLine []string) (string, string, error) {
	now := app.now()
	pid := app.getPID()
	workingDir := app.panicReportWorkingDir()
	crashLogPath := filepath.Join(workingDir, app.formatCrashLogPath(now, pid))

	content := app.formatCrashLog(err, opts, commandLine, now, workingDir, pid)
	if writeErr := app.writeLog(crashLogPath, []byte(content), crashFileMode); writeErr != nil {
		return "", content, writeErr
	}

	return crashLogPath, content, nil
}

func (app *terragruntApp) panicReportWorkingDir() string {
	workingDir, err := app.getwd()
	if err == nil {
		return workingDir
	}

	return os.TempDir()
}

func (app *terragruntApp) formatCrashLog(err error, opts *options.TerragruntOptions, commandLine []string, when time.Time, workingDir string, pid int) string {
	terragruntVersion := "unknown"
	if opts != nil && opts.TerragruntVersion != nil {
		terragruntVersion = opts.TerragruntVersion.String()
	}

	commit, modified := "unknown", false
	if app.buildInfo != nil {
		commit, modified = app.buildInfo()
	}

	allRoutines := "(unavailable)"
	if app.allGoroutineDump != nil {
		allRoutines = app.allGoroutineDump()
	}

	errMessage := "(no error)"
	errStack := "(no stack trace was available)"

	if err != nil {
		errMessage = err.Error()

		if stack := errors.ErrorStack(err); stack != "" {
			errStack = stack
		}
	}

	command := strings.Join(commandLine, " ")
	if command == "" {
		command = "(empty command line)"
	}

	return fmt.Sprintf(
		crashLogTemplate,
		when.UTC().Format(time.RFC3339Nano),
		terragruntVersion,
		commit,
		modified,
		runtime.Version(),
		runtime.GOOS,
		runtime.GOARCH,
		runtime.NumCPU(),
		runtime.GOMAXPROCS(0),
		runtime.NumGoroutine(),
		pid,
		workingDir,
		command,
		errMessage,
		errStack,
		allRoutines,
	)
}

func (app *terragruntApp) formatCrashLogPath(when time.Time, pid int) string {
	return crashLogPrefix + "-" + when.UTC().Format(crashLogFileTimeLayout) + "-" + strconv.Itoa(pid) + ".log"
}

func dumpAllGoroutines() string {
	buf := make([]byte, allGoroutineStackBufSize)
	n := runtime.Stack(buf, true)

	return string(buf[:n])
}

func readBuildInfo() (string, bool) {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return "unknown", false
	}

	commit := "unknown"
	modified := false

	for _, s := range info.Settings {
		switch s.Key {
		case "vcs.revision":
			if s.Value != "" {
				commit = s.Value
			}
		case "vcs.modified":
			modified = s.Value == "true"
		}
	}

	return commit, modified
}

func setupContext(l log.Logger, exitCode *tf.DetailedExitCodeMap) context.Context {
	ctx := context.Background()
	ctx = tf.ContextWithDetailedExitCode(ctx, exitCode)

	return log.ContextWithLogger(ctx, l)
}
