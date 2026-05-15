package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
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
	terragruntIssueURL = "https://github.com/gruntwork-io/terragrunt/issues"
	crashLogPrefix     = "terragrunt-crash"
	crashFileMode      = 0o600
	panicOutput        = `
***************************** TERRAGRUNT CRASH *****************************

Terragrunt crashed! This is always indicative of a bug within Terragrunt.
Please report the crash with Terragrunt[1] so that we can fix this.

When reporting bugs, please include your Terragrunt version and the panic report
file, and any additional information which may help replicate the issue.

[1]: %s

***************************** TERRAGRUNT CRASH *****************************
`
	crashLogTemplate = `Terragrunt panic report
======================

Timestamp: %s
Terragrunt version: %s
Go runtime: %s
GOOS/GOARCH: %s/%s
PID: %d
Working directory: %s
Command line: %s

Panic: %s

Stack trace:
%s
`
)

type terragruntApp struct {
	now      func() time.Time
	getwd    func() (string, error)
	getPID   func() int
	writeLog func(string, []byte, os.FileMode) error
}

func main() {
	app := &terragruntApp{
		now:      time.Now,
		getwd:    os.Getwd,
		getPID:   os.Getpid,
		writeLog: os.WriteFile,
	}

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

	defer func() {
		finalExitCode := exitCode.GetFinalExitCode()
		if opts.TerraformCliArgs.Contains(tf.FlagNameDetailedExitCode) {
			finalExitCode = exitCode.GetFinalDetailedExitCode()
		}

		errors.Recover(app.checkForPanicAndExit(l, finalExitCode, opts))
	}()

	cliApp := cli.NewApp(l, opts)
	ctx := setupContext(l, exitCode)
	err := cliApp.RunContext(ctx, os.Args)

	finalExitCode := exitCode.GetFinalExitCode()
	if opts.TerraformCliArgs.Contains(tf.FlagNameDetailedExitCode) {
		finalExitCode = exitCode.GetFinalDetailedExitCode()
	}

	app.checkForErrorsAndExit(l, finalExitCode, opts)(err)
}

// Private helper functions

func (app *terragruntApp) checkForErrorsAndExit(l log.Logger, exitCode int, opts *options.TerragruntOptions) func(error) {
	return func(err error) {
		if err == nil {
			os.Exit(exitCode)
		}

		exitCoder, exitCodeErr := util.GetExitCode(err)
		if exitCodeErr != nil {
			exitCoder = 1
		}

		if errors.IsFunctionPanic(err) {
			app.reportPanic(l, err, opts)
			os.Exit(exitCoder)
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

func (app *terragruntApp) checkForPanicAndExit(l log.Logger, exitCode int, opts *options.TerragruntOptions) func(error) {
	return app.checkForErrorsAndExit(l, exitCode, opts)
}

func (app *terragruntApp) reportPanic(l log.Logger, err error, opts *options.TerragruntOptions) {
	crashLogPath, crashLog, writeErr := app.writeCrashLog(err, opts, os.Args)

	l.Error(fmt.Sprintf(panicOutput, terragruntIssueURL))

	if err != nil {
		l.Error(err)
	}

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
	if err := app.writeLog(crashLogPath, []byte(content), crashFileMode); err != nil {
		return "", content, err
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

	errStack := errors.ErrorStack(err)
	if errStack == "" {
		errStack = "(no stack trace was available)"
	}

	command := strings.Join(commandLine, " ")
	if command == "" {
		command = "(empty command line)"
	}

	return fmt.Sprintf(
		crashLogTemplate,
		when.Format(time.RFC3339Nano),
		terragruntVersion,
		runtime.Version(),
		runtime.GOOS,
		runtime.GOARCH,
		pid,
		workingDir,
		command,
		err.Error(),
		errStack,
	)
}

func (app *terragruntApp) formatCrashLogPath(when time.Time, pid int) string {
	return crashLogPrefix + "-" + when.Format("20060102T150405.000000000") + "-" + strconv.Itoa(pid) + ".log"
}

func setupContext(l log.Logger, exitCode *tf.DetailedExitCodeMap) context.Context {
	ctx := context.Background()
	ctx = tf.ContextWithDetailedExitCode(ctx, exitCode)

	return log.ContextWithLogger(ctx, l)
}
