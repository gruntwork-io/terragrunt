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
)

var (
	panicNow      = time.Now
	panicGetwd    = os.Getwd
	panicGetPID   = os.Getpid
	panicWriteLog = os.WriteFile
)

// The main entrypoint for Terragrunt
func main() {
	exitCode := tf.NewDetailedExitCodeMap()

	opts := options.NewTerragruntOptions()

	l := log.New(
		log.WithOutput(opts.Writers.ErrWriter),
		log.WithLevel(options.DefaultLogLevel),
		log.WithFormatter(format.NewFormatter(format.NewPrettyFormatPlaceholders())),
	)

	// Immediately parse the `TG_LOG_LEVEL` environment variable, e.g. to set the TRACE level.
	if err := global.NewLogLevelFlag(l, opts, nil).Parse(os.Args); err != nil {
		l.Error(err.Error())
		os.Exit(1)
	}

	defer func() {
		if opts.TerraformCliArgs.Contains(tf.FlagNameDetailedExitCode) {
			errors.Recover(checkForPanicAndExit(l, exitCode.GetFinalDetailedExitCode(), opts))
			return
		}

		errors.Recover(checkForPanicAndExit(l, exitCode.GetFinalExitCode(), opts))
	}()

	app := cli.NewApp(l, opts)

	ctx := setupContext(l, exitCode)
	err := app.RunContext(ctx, os.Args)

	if opts.TerraformCliArgs.Contains(tf.FlagNameDetailedExitCode) {
		checkForErrorsAndExit(l, exitCode.GetFinalDetailedExitCode())(err)

		return
	}

	checkForErrorsAndExit(l, exitCode.GetFinalExitCode())(err)
}

// If there is an error, display it in the console and exit with a non-zero exit code. Otherwise, exit 0.
func checkForErrorsAndExit(l log.Logger, exitCode int) func(error) {
	return func(err error) {
		if err == nil {
			os.Exit(exitCode)
		}

		l.Error(err.Error())

		if errStack := errors.ErrorStack(err); errStack != "" {
			l.Trace(errStack)
		}

		// exit with the underlying error code
		exitCoder, exitCodeErr := util.GetExitCode(err)
		if exitCodeErr != nil {
			exitCoder = 1
		}

		if explain := shell.ExplainError(err); len(explain) > 0 {
			l.Errorf("Suggested fixes: \n%s", explain)
		}

		os.Exit(exitCoder)
	}
}

// checkForPanicAndExit handles a captured panic, writes a crash report if possible,
// and then exits with the normal Terragrunt error handling path.
func checkForPanicAndExit(l log.Logger, exitCode int, opts *options.TerragruntOptions) func(error) {
	return func(err error) {
		if err != nil {
			reportPanic(l, err, opts)
		}

		checkForErrorsAndExit(l, exitCode)(err)
	}
}

func reportPanic(l log.Logger, err error, opts *options.TerragruntOptions) {
	crashLogPath, crashLog, writeErr := writeCrashLog(err, opts, os.Args)

	l.Error(fmt.Sprintf(panicOutput, terragruntIssueURL))

	if err != nil {
		l.Error(err)
	}

	if writeErr == nil {
		l.Errorf("A panic report has been saved to: %s", crashLogPath)
		l.Errorf("Please report this issue at %s and attach the panic report.", terragruntIssueURL)
	} else {
		l.Errorf("Unable to write crash report: %v", writeErr)
		l.Errorf("Please report this issue at %s and include the crash report output below.", terragruntIssueURL)
		l.Error(crashLog)
	}
}

func writeCrashLog(err error, opts *options.TerragruntOptions, commandLine []string) (string, string, error) {
	now := panicNow()
	pid := panicGetPID()
	workingDir := panicReportWorkingDir()
	crashLogPath := filepath.Join(workingDir, formatCrashLogPath(now, pid))

	content := formatCrashLog(err, opts, commandLine, now, workingDir, pid)
	if err := panicWriteLog(crashLogPath, []byte(content), crashFileMode); err != nil {
		return "", content, err
	}

	return crashLogPath, content, nil
}

func panicReportWorkingDir() string {
	workingDir, err := panicGetwd()
	if err == nil {
		return workingDir
	}

	return os.TempDir()
}

func formatCrashLog(err error, opts *options.TerragruntOptions, commandLine []string, when time.Time, workingDir string, pid int) string {
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

	return "Terragrunt panic report\n" +
		"======================\n\n" +
		"Timestamp: " + when.Format(time.RFC3339Nano) + "\n" +
		"Terragrunt version: " + terragruntVersion + "\n" +
		"Go runtime: " + runtime.Version() + "\n" +
		"GOOS/GOARCH: " + runtime.GOOS + "/" + runtime.GOARCH + "\n" +
		"PID: " + strconv.Itoa(pid) + "\n" +
		"Working directory: " + workingDir + "\n" +
		"Command line: " + command + "\n\n" +
		"Panic: " + err.Error() + "\n\n" +
		"Stack trace:\n" + errStack + "\n"
}

func formatCrashLogPath(when time.Time, pid int) string {
	return crashLogPrefix + "-" + when.Format("20060102T150405.000000000") + "-" + strconv.Itoa(pid) + ".log"
}

func setupContext(l log.Logger, exitCode *tf.DetailedExitCodeMap) context.Context {
	ctx := context.Background()
	ctx = tf.ContextWithDetailedExitCode(ctx, exitCode)

	return log.ContextWithLogger(ctx, l)
}
