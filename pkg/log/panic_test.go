package log_test

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
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReportPanicWritesCrashLog(t *testing.T) {
	t.Parallel()

	tmp := t.TempDir()
	when := time.Date(2026, 5, 15, 12, 30, 45, 0, time.UTC)
	logger, output := newPanicLogger()

	r := newStubPanicReporter(tmp, when, 8080)
	r.ReportPanic(logger, "1.7.9", "nil pointer dereference", []byte("stack-frames\nrunes"), []string{"terragrunt", "plan"})

	expectedPath := filepath.Join(tmp, "terragrunt-crash-20260515T123045Z-8080.log")

	logOutput := output.String()
	assert.Contains(t, logOutput, "TERRAGRUNT CRASH")
	assert.Contains(t, logOutput, "A panic report has been saved to: "+expectedPath)
	assert.Contains(t, logOutput, log.PanicIssueURL)
	assert.NotContains(t, logOutput, "Unable to write crash report")

	body, err := os.ReadFile(expectedPath)
	require.NoError(t, err)

	content := string(body)

	assert.Contains(t, content, "Terragrunt panic report")
	assert.Contains(t, content, "Terragrunt version: 1.7.9")
	assert.Contains(t, content, "Build commit: deadbeef")
	assert.Contains(t, content, "Build modified: true")
	assert.Contains(t, content, "GOOS/GOARCH: "+runtime.GOOS+"/"+runtime.GOARCH)
	assert.Contains(t, content, "NumCPU: ")
	assert.Contains(t, content, "GOMAXPROCS: ")
	assert.Contains(t, content, "NumGoroutine: ")
	assert.Contains(t, content, "Working directory: "+tmp)
	assert.Contains(t, content, "Command line: terragrunt plan")
	assert.Contains(t, content, "Panic: nil pointer dereference")
	assert.Contains(t, content, "stack-frames")
}

func TestReportPanicFallsBackWhenWriteFails(t *testing.T) {
	t.Parallel()

	r := newStubPanicReporter(t.TempDir(), time.Now().UTC(), os.Getpid())
	r.WriteFile = func(string, []byte, os.FileMode) error {
		return stdErrors.New("disk full")
	}

	logger, output := newPanicLogger()
	r.ReportPanic(logger, "1.7.9", "slice bounds out of range", []byte("stack"), []string{"terragrunt"})

	logOutput := output.String()
	assert.Contains(t, logOutput, "Unable to write crash report: disk full")
	assert.Contains(t, logOutput, "Panic: slice bounds out of range")
	assert.Contains(t, logOutput, "TERRAGRUNT CRASH")
	assert.Contains(t, logOutput, log.PanicIssueURL)
}

func TestReportPanicFallbacksOnEmptyInputs(t *testing.T) {
	t.Parallel()

	tmp := t.TempDir()
	r := newStubPanicReporter(tmp, time.Now().UTC(), 1)

	logger, _ := newPanicLogger()
	r.ReportPanic(logger, "", "", nil, []string{})

	entries, err := os.ReadDir(tmp)
	require.NoError(t, err)
	require.Len(t, entries, 1)

	body, err := os.ReadFile(filepath.Join(tmp, entries[0].Name()))
	require.NoError(t, err)

	content := string(body)

	assert.Contains(t, content, "Terragrunt version: unknown")
	assert.Contains(t, content, "Panic: (no panic message)")
	assert.Contains(t, content, "Command line: (empty command line)")
	assert.Contains(t, content, "(no stack trace was available)")
}

func TestReportPanicFallsBackToTempDirWhenGetwdFails(t *testing.T) {
	t.Parallel()

	r := newStubPanicReporter(t.TempDir(), time.Now().UTC(), 1)
	r.Getwd = func() (string, error) { return "", stdErrors.New("denied") }

	logger, _ := newPanicLogger()
	r.ReportPanic(logger, "1.7.9", "divide by zero", []byte("stack"), nil)

	entries, err := os.ReadDir(os.TempDir())
	require.NoError(t, err)

	var found bool

	for _, e := range entries {
		if strings.HasPrefix(e.Name(), "terragrunt-crash-") {
			found = true

			break
		}
	}

	assert.True(t, found, "expected at least one terragrunt-crash- file under TempDir")
}

func TestIsPanic(t *testing.T) {
	t.Parallel()

	t.Run("nil is not a panic", func(t *testing.T) {
		t.Parallel()

		assert.False(t, log.IsPanic(nil))
	})

	t.Run("plain wrapped error is not a panic", func(t *testing.T) {
		t.Parallel()

		assert.False(t, log.IsPanic(stdErrors.New("regular failure")))
		assert.False(t, log.IsPanic(stdErrors.New("panic but not from runtime")))
	})

	t.Run("matches cty function.PanicError by type", func(t *testing.T) {
		t.Parallel()

		err := function.PanicError{
			Value: "nil deref in cty function",
			Stack: []byte("cty stack"),
		}

		assert.True(t, log.IsPanic(err))
		assert.True(t, log.IsPanic(fmt.Errorf("wrapped: %w", err)))
	})

	t.Run("matches an error whose message contains a runtime panic frame", func(t *testing.T) {
		t.Parallel()

		err := stdErrors.New("panic: simulated\n\ngoroutine 1 [running]:\nruntime/debug.Stack()\npanic({0x...})\n\t/usr/local/go/src/runtime/panic.go:860")

		assert.True(t, log.IsPanic(err))
	})

	t.Run("matches an error whose ErrorStack contains a runtime panic frame", func(t *testing.T) {
		t.Parallel()

		err := stackedError{
			msg:   "wrapping a panic",
			stack: "goroutine 1:\nruntime/panic.go:860\npanic({0x...})\n",
		}

		assert.True(t, log.IsPanic(err))
	})
}

// Private helper functions

type stackedError struct {
	msg   string
	stack string
}

func (e stackedError) Error() string      { return e.msg }
func (e stackedError) ErrorStack() string { return e.stack }

func newPanicLogger() (log.Logger, *bytes.Buffer) {
	buf := new(bytes.Buffer)
	formatter := format.NewFormatter(placeholders.Placeholders{placeholders.Message()})

	return log.New(log.WithOutput(buf), log.WithLevel(log.InfoLevel), log.WithFormatter(formatter)), buf
}

func newStubPanicReporter(workDir string, now time.Time, pid int) *log.PanicReporter {
	return &log.PanicReporter{
		Now:       func() time.Time { return now },
		Getwd:     func() (string, error) { return workDir, nil },
		GetPID:    func() int { return pid },
		WriteFile: os.WriteFile,
		BuildInfo: func() (string, bool) { return "deadbeef", true },
	}
}
