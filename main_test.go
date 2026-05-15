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

	app := &terragruntApp{}
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
	assert.Contains(t, output, "GOOS/GOARCH: "+runtime.GOOS+"/"+runtime.GOARCH)
	assert.Contains(t, output, "Working directory: "+workDir)
	assert.Contains(t, output, "Command line: terragrunt run all")
	assert.Contains(t, output, "Panic: boom")
	assert.Contains(t, output, "Stack trace:")
}

func TestFormatCrashLogUsesEmptyCommandLineFallback(t *testing.T) {
	t.Parallel()

	app := &terragruntApp{}
	when := time.Now().UTC()
	output := app.formatCrashLog(stdErrors.New("boom"), &options.TerragruntOptions{}, []string{}, when, filepath.Join(os.TempDir(), "terragrunt-test-workdir"), 999)

	assert.Contains(t, output, "Command line: (empty command line)")
}

func TestWriteCrashLogCreatesFile(t *testing.T) {
	t.Parallel()

	tmp := t.TempDir()
	panicWhen := time.Now().UTC()
	app := &terragruntApp{
		getwd: func() (string, error) {
			return tmp, nil
		},
		now: func() time.Time {
			return panicWhen
		},
		getPID: func() int {
			return 2026
		},
		writeLog: os.WriteFile,
	}

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
	assert.True(t, strings.HasPrefix(filepath.Base(logPath), "terragrunt-crash-"))
}

func TestWriteCrashLogFallsBackToTempDirWhenGetwdFails(t *testing.T) {
	t.Parallel()

	panicWhen := time.Now().UTC()
	app := &terragruntApp{
		getwd: func() (string, error) {
			return "", stdErrors.New("boom")
		},
		now: func() time.Time {
			return panicWhen
		},
		getPID: func() int {
			return 1010
		},
		writeLog: os.WriteFile,
	}

	assert.Equal(t, os.TempDir(), app.panicReportWorkingDir())

	logPath, _, writeErr := app.writeCrashLog(stdErrors.New("boom"), &options.TerragruntOptions{}, []string{})
	require.NoError(t, writeErr)
	assert.Equal(t, os.TempDir(), filepath.Dir(logPath))
}

func TestReportPanicFallsBackIfCrashLogCannotBeWritten(t *testing.T) {
	t.Parallel()

	app := &terragruntApp{
		now:    time.Now,
		getwd:  os.Getwd,
		getPID: os.Getpid,
		writeLog: func(string, []byte, os.FileMode) error {
			return stdErrors.New("disk full")
		},
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
	app := &terragruntApp{
		getwd: func() (string, error) {
			return tmp, nil
		},
		now: func() time.Time {
			return panicWhen
		},
		getPID: func() int {
			return 8080
		},
		writeLog: os.WriteFile,
	}

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

// Private helper functions

func newTestLogger() (log.Logger, *bytes.Buffer) {
	formatter := format.NewFormatter(placeholders.Placeholders{placeholders.Message()})
	output := new(bytes.Buffer)
	logger := log.New(log.WithOutput(output), log.WithLevel(log.InfoLevel), log.WithFormatter(formatter))

	return logger, output
}
