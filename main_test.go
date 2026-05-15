package main

import (
	"bytes"
	stdErrors "errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/gruntwork-io/terragrunt/internal/errors"
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
	terragruntVersion := version.Must(version.NewVersion("1.7.9"))
	opts := &options.TerragruntOptions{
		TerragruntVersion: terragruntVersion,
	}
	when := time.Now().UTC()
	workDir := filepath.Join(os.TempDir(), "terragrunt-test-workdir")

	output := app.formatCrashLog(stdErrors.New("boom"), opts, []string{"terragrunt", "run", "all"}, when, workDir, 999)

	assert.Contains(t, output, "Terragrunt panic report")
	assert.Contains(t, output, "Timestamp: "+when.Format(time.RFC3339Nano))
	assert.Contains(t, output, "Terragrunt version: 1.7.9")
	assert.Contains(t, output, "Build commit: deadbeef")
	assert.Contains(t, output, "Build modified: true")
	assert.Contains(t, output, "GOOS/GOARCH: "+runtime.GOOS+"/"+runtime.GOARCH)
	assert.Contains(t, output, "NumGoroutine: ")
	assert.Contains(t, output, "NumCPU: ")
	assert.Contains(t, output, "GOMAXPROCS: ")
	assert.Contains(t, output, "Working directory: "+workDir)
	assert.Contains(t, output, "Command line: terragrunt run all")
	assert.Contains(t, output, "Panic: boom")
	assert.Contains(t, output, "Stack trace:")
	assert.Contains(t, output, "All goroutines:")
	assert.Contains(t, output, "stub-goroutine-dump")
}

func TestFormatCrashLogUsesEmptyCommandLineFallback(t *testing.T) {
	t.Parallel()

	app := newStubApp(t.TempDir(), time.Now().UTC(), 999)
	when := time.Now().UTC()
	output := app.formatCrashLog(stdErrors.New("boom"), &options.TerragruntOptions{}, []string{}, when, filepath.Join(os.TempDir(), "terragrunt-test-workdir"), 999)

	assert.Contains(t, output, "Command line: (empty command line)")
}

func TestFormatCrashLogHandlesNilError(t *testing.T) {
	t.Parallel()

	app := newStubApp(t.TempDir(), time.Now().UTC(), 999)
	output := app.formatCrashLog(nil, &options.TerragruntOptions{}, []string{"terragrunt"}, time.Now().UTC(), os.TempDir(), 1)

	assert.Contains(t, output, "Panic: (no error)")
	assert.Contains(t, output, "(no stack trace was available)")
}

func TestWriteCrashLogCreatesFile(t *testing.T) {
	t.Parallel()

	tmp := t.TempDir()
	panicWhen := time.Now().UTC()
	app := newStubApp(tmp, panicWhen, 2026)

	err := errors.New("boom")
	opts := &options.TerragruntOptions{
		TerragruntVersion: version.Must(version.NewVersion("1.7.9")),
	}

	logPath, logContent, writeErr := app.writeCrashLog(err, opts, []string{"terragrunt", "plan"})
	require.NoError(t, writeErr)
	require.NotEmpty(t, logContent)
	assert.Equal(t, filepath.Join(tmp, app.formatCrashLogPath(panicWhen, 2026)), logPath)

	body, readErr := os.ReadFile(logPath)
	require.NoError(t, readErr)
	assert.Contains(t, string(body), "Terragrunt version: 1.7.9")
	assert.Contains(t, string(body), "Command line: terragrunt plan")
	assert.Contains(t, string(body), "Panic: boom")
	assert.Contains(t, string(body), "Stack trace:")
	assert.Contains(t, string(body), "All goroutines:")
	assert.True(t, strings.HasPrefix(filepath.Base(logPath), "terragrunt-crash-"))
	assert.True(t, strings.HasSuffix(filepath.Base(logPath), ".log"))
}

func TestFormatCrashLogPathUsesSecondPrecisionTimestamp(t *testing.T) {
	t.Parallel()

	app := newStubApp(t.TempDir(), time.Now().UTC(), 1)
	when := time.Date(2026, 5, 15, 12, 30, 45, 123_456_789, time.UTC)
	got := app.formatCrashLogPath(when, 4242)

	assert.Equal(t, "terragrunt-crash-20260515T123045Z-4242.log", got)
}

func TestWriteCrashLogFallsBackToTempDirWhenGetwdFails(t *testing.T) {
	t.Parallel()

	panicWhen := time.Now().UTC()
	app := newStubApp(t.TempDir(), panicWhen, 1010)
	app.getwd = func() (string, error) {
		return "", stdErrors.New("boom")
	}

	assert.Equal(t, os.TempDir(), app.panicReportWorkingDir())

	logPath, _, writeErr := app.writeCrashLog(stdErrors.New("boom"), &options.TerragruntOptions{}, []string{})
	require.NoError(t, writeErr)
	assert.Equal(t, os.TempDir(), filepath.Dir(logPath))
}

func TestReportPanicFallsBackIfCrashLogCannotBeWritten(t *testing.T) {
	t.Parallel()

	app := newStubApp(t.TempDir(), time.Now().UTC(), os.Getpid())
	app.writeLog = func(string, []byte, os.FileMode) error {
		return stdErrors.New("disk full")
	}

	logger, output := newTestLogger()
	err := errors.New("unable to write crash log")

	app.reportPanic(logger, err, &options.TerragruntOptions{
		TerragruntVersion: version.Must(version.NewVersion("1.7.9")),
	})

	logOutput := output.String()
	assert.Contains(t, logOutput, "Unable to write crash report: disk full")
	assert.Contains(t, logOutput, terragruntIssueURL)
	assert.Contains(t, logOutput, "Please report this issue at")
	assert.Contains(t, logOutput, "Terragrunt panic report")
	assert.Contains(t, logOutput, "Terragrunt crashed!")
	assert.Contains(t, logOutput, "TERRAGRUNT CRASH")
	assert.Contains(t, logOutput, "Stack trace:")
}

func TestReportPanicWritesHelpfulMessage(t *testing.T) {
	t.Parallel()

	tmp := t.TempDir()
	panicWhen := time.Now().UTC()
	app := newStubApp(tmp, panicWhen, 8080)

	logger, output := newTestLogger()
	err := errors.New("capture message")

	app.reportPanic(logger, err, &options.TerragruntOptions{
		TerragruntVersion: version.Must(version.NewVersion("1.7.9")),
	})

	expectedPath := filepath.Join(tmp, app.formatCrashLogPath(panicWhen, 8080))
	logOutput := output.String()

	assert.Contains(t, logOutput, "A panic report has been saved to: "+expectedPath)
	assert.Contains(t, logOutput, terragruntIssueURL)
	assert.Contains(t, logOutput, "Terragrunt crashed!")
	assert.NotContains(t, logOutput, "Unable to write crash report")
	assert.Contains(t, logOutput, "TERRAGRUNT CRASH")
}

func TestReportPanicIsNoopOnNilError(t *testing.T) {
	t.Parallel()

	tmp := t.TempDir()
	app := newStubApp(tmp, time.Now().UTC(), 1)
	logger, output := newTestLogger()

	app.reportPanic(logger, nil, &options.TerragruntOptions{})

	assert.Empty(t, output.String())

	entries, err := os.ReadDir(tmp)
	require.NoError(t, err)
	assert.Empty(t, entries, "no crash log should be written for a nil panic")
}

// TestRecoverIntegrationFromDeferredPanic exercises the real production
// pattern: defer errors.Recover(handler) catches a panic raised during the
// surrounded scope. It is a regression test for the previous bug where the
// recover() was wrapped inside another deferred closure and silently
// returned nil, letting panics bypass the crash-report path entirely.
func TestRecoverIntegrationFromDeferredPanic(t *testing.T) {
	t.Parallel()

	tmp := t.TempDir()
	app := newStubApp(tmp, time.Now().UTC(), 9001)
	logger, output := newTestLogger()
	opts := &options.TerragruntOptions{
		TerragruntVersion: version.Must(version.NewVersion("1.7.9")),
	}

	func() {
		defer errors.Recover(func(cause error) {
			require.NotNil(t, cause)
			assert.True(t, errors.IsFunctionPanic(cause))
			app.reportPanic(logger, cause, opts)
		})

		panic("kaboom")
	}()

	logOutput := output.String()
	assert.Contains(t, logOutput, "TERRAGRUNT CRASH")
	assert.Contains(t, logOutput, "A panic report has been saved to:")

	entries, err := os.ReadDir(tmp)
	require.NoError(t, err)
	require.Len(t, entries, 1, "exactly one crash log should be produced")

	body, err := os.ReadFile(filepath.Join(tmp, entries[0].Name()))
	require.NoError(t, err)
	assert.Contains(t, string(body), "Panic: panic in function implementation: kaboom")
	assert.Contains(t, string(body), "Stack trace:")
	assert.Contains(t, string(body), "All goroutines:")
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
		now:      func() time.Time { return panicWhen },
		getwd:    func() (string, error) { return workDir, nil },
		getPID:   func() int { return pid },
		writeLog: os.WriteFile,
		allGoroutineDump: func() string {
			return "stub-goroutine-dump\n"
		},
		buildInfo: func() (string, bool) {
			return "deadbeef", true
		},
	}
}
