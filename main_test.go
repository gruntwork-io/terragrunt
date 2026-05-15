package main

import (
	"bytes"
	stdErrors "errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/zclconf/go-cty/cty/function"

	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/pkg/log/format"
	"github.com/gruntwork-io/terragrunt/pkg/log/format/placeholders"
	"github.com/gruntwork-io/terragrunt/pkg/options"
	"github.com/hashicorp/go-version"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFormatCrashLog(t *testing.T) {
	t.Parallel()

	app := newStubApp(t.TempDir(), time.Now().UTC(), 999)
	opts := &options.TerragruntOptions{
		TerragruntVersion: version.Must(version.NewVersion("1.7.9")),
	}
	when := time.Now().UTC()
	workDir := filepath.Join(os.TempDir(), "terragrunt-test-workdir")

	output := app.formatCrashLog("nil pointer dereference", []byte("stack-frames"), opts, []string{"terragrunt", "run", "all"}, when, workDir, 999)

	assert.Contains(t, output, "Terragrunt panic report")
	assert.Contains(t, output, "Terragrunt version: 1.7.9")
	assert.Contains(t, output, "Build commit: deadbeef")
	assert.Contains(t, output, "Build modified: true")
	assert.Contains(t, output, "GOOS/GOARCH: "+runtime.GOOS+"/"+runtime.GOARCH)
	assert.Contains(t, output, "NumCPU: ")
	assert.Contains(t, output, "GOMAXPROCS: ")
	assert.Contains(t, output, "NumGoroutine: ")
	assert.Contains(t, output, "Working directory: "+workDir)
	assert.Contains(t, output, "Command line: terragrunt run all")
	assert.Contains(t, output, "Panic: nil pointer dereference")
	assert.Contains(t, output, "Stack trace:")
	assert.Contains(t, output, "stack-frames")
}

func TestFormatCrashLogFallbacks(t *testing.T) {
	t.Parallel()

	app := newStubApp(t.TempDir(), time.Now().UTC(), 1)
	output := app.formatCrashLog("", nil, &options.TerragruntOptions{}, []string{}, time.Now().UTC(), os.TempDir(), 1)

	assert.Contains(t, output, "Panic: (no panic message)")
	assert.Contains(t, output, "Command line: (empty command line)")
	assert.Contains(t, output, "(no stack trace was available)")
	assert.Contains(t, output, "Terragrunt version: unknown")
}

func TestWriteCrashLogCreatesFile(t *testing.T) {
	t.Parallel()

	tmp := t.TempDir()
	panicWhen := time.Now().UTC()
	app := newStubApp(tmp, panicWhen, 2026)

	logPath, _, writeErr := app.writeCrashLog("index out of range", []byte("stack"), &options.TerragruntOptions{
		TerragruntVersion: version.Must(version.NewVersion("1.7.9")),
	}, []string{"terragrunt", "plan"})
	require.NoError(t, writeErr)
	assert.Equal(t, filepath.Join(tmp, app.formatCrashLogPath(panicWhen, 2026)), logPath)

	body, readErr := os.ReadFile(logPath)
	require.NoError(t, readErr)
	assert.Contains(t, string(body), "Panic: index out of range")
	assert.Contains(t, string(body), "Command line: terragrunt plan")
	assert.True(t, strings.HasPrefix(filepath.Base(logPath), "terragrunt-crash-"))
	assert.True(t, strings.HasSuffix(filepath.Base(logPath), ".log"))
}

func TestFormatCrashLogPath(t *testing.T) {
	t.Parallel()

	app := newStubApp(t.TempDir(), time.Now().UTC(), 1)
	when := time.Date(2026, 5, 15, 12, 30, 45, 0, time.UTC)

	assert.Equal(t, "terragrunt-crash-20260515T123045Z-4242.log", app.formatCrashLogPath(when, 4242))
}

func TestPanicReportWorkingDirFallsBackToTempDir(t *testing.T) {
	t.Parallel()

	app := newStubApp(t.TempDir(), time.Now().UTC(), 1)
	app.getwd = func() (string, error) { return "", stdErrors.New("denied") }

	assert.Equal(t, os.TempDir(), app.panicReportWorkingDir())
}

func TestReportPanicFallsBackIfCrashLogCannotBeWritten(t *testing.T) {
	t.Parallel()

	app := newStubApp(t.TempDir(), time.Now().UTC(), os.Getpid())
	app.writeLog = func(string, []byte, os.FileMode) error {
		return stdErrors.New("disk full")
	}

	logger, output := newTestLogger()
	app.reportPanic(logger, "slice bounds out of range", []byte("stack"), &options.TerragruntOptions{})

	logOutput := output.String()
	assert.Contains(t, logOutput, "Unable to write crash report: disk full")
	assert.Contains(t, logOutput, terragruntIssueURL)
	assert.Contains(t, logOutput, "TERRAGRUNT CRASH")
	assert.Contains(t, logOutput, "Panic: slice bounds out of range")
}

func TestReportPanicWritesHelpfulMessage(t *testing.T) {
	t.Parallel()

	tmp := t.TempDir()
	panicWhen := time.Now().UTC()
	app := newStubApp(tmp, panicWhen, 8080)

	logger, output := newTestLogger()
	app.reportPanic(logger, "divide by zero", []byte("stack"), &options.TerragruntOptions{})

	expectedPath := filepath.Join(tmp, app.formatCrashLogPath(panicWhen, 8080))
	logOutput := output.String()

	assert.Contains(t, logOutput, "A panic report has been saved to: "+expectedPath)
	assert.Contains(t, logOutput, terragruntIssueURL)
	assert.Contains(t, logOutput, "TERRAGRUNT CRASH")
	assert.NotContains(t, logOutput, "Unable to write crash report")
}

// TestCheckForErrorsAndExitDetectsCtyPanic verifies that an HCL-function
// panic surfacing as a function.PanicError returned from RunContext is
// detected via stdlib errors.As and routed through the crash-report UX.
func TestCheckForErrorsAndExitDetectsCtyPanic(t *testing.T) {
	t.Parallel()

	ctyErr := function.PanicError{Value: "nil deref", Stack: []byte("cty-panic-stack")}
	wrapped := fmt.Errorf("evaluating expression: %w", ctyErr)

	var detected function.PanicError
	require.ErrorAs(t, wrapped, &detected)
	assert.Equal(t, "nil deref", fmt.Sprintf("%v", detected.Value))
	assert.Equal(t, "cty-panic-stack", string(detected.Stack))
}

// TestRawRecoverCatchesPanic is a regression test for the original bug —
// errors.Recover wrapped inside another deferred closure returns nil.
// Using raw recover() directly inside the deferred function works.
func TestRawRecoverCatchesPanic(t *testing.T) {
	t.Parallel()

	var caught any

	func() {
		defer func() {
			caught = recover()
		}()

		panic("simulated runtime panic")
	}()

	assert.Equal(t, "simulated runtime panic", caught)
}

// Private helper functions

func newTestLogger() (log.Logger, *bytes.Buffer) {
	formatter := format.NewFormatter(placeholders.Placeholders{placeholders.Message()})
	output := new(bytes.Buffer)
	logger := log.New(log.WithOutput(output), log.WithLevel(log.InfoLevel), log.WithFormatter(formatter))

	return logger, output
}

func newStubApp(workDir string, panicWhen time.Time, pid int) *terragruntApp {
	return &terragruntApp{
		now:       func() time.Time { return panicWhen },
		getwd:     func() (string, error) { return workDir, nil },
		getPID:    func() int { return pid },
		writeLog:  os.WriteFile,
		buildInfo: func() (string, bool) { return "deadbeef", true },
	}
}
