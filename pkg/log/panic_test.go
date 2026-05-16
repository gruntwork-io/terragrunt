package log_test

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"testing"
	"time"

	"github.com/zclconf/go-cty/cty/function"

	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/pkg/log/format"
	"github.com/gruntwork-io/terragrunt/pkg/log/format/placeholders"
	tlogger "github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReportPanicWritesCrashLog(t *testing.T) {
	t.Parallel()

	when := time.Date(2026, 5, 15, 12, 30, 45, 0, time.UTC)
	fs := vfs.NewMemMapFS()
	r := newStubPanicReporter(fs, "/wd", when, 8080)
	logger, output := newPanicLogger()

	r.ReportPanic(logger, "1.7.9", "nil pointer dereference", []byte("stack-frames\nrunes"), []string{"terragrunt", "plan"})

	expectedPath := "/wd/terragrunt-crash-20260515T123045Z-8080.log"

	logOutput := output.String()
	assert.Contains(t, logOutput, "TERRAGRUNT CRASH")
	assert.Contains(t, logOutput, "A panic report has been saved to: "+expectedPath)
	assert.Contains(t, logOutput, log.PanicIssueURL)
	assert.NotContains(t, logOutput, "Unable to write crash report")

	body, err := vfs.ReadFile(fs, expectedPath)
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
	assert.Contains(t, content, "Working directory: /wd")
	assert.Contains(t, content, "Command line: terragrunt plan")
	assert.Contains(t, content, "Panic: nil pointer dereference")
	assert.Contains(t, content, "stack-frames")
}

func TestReportPanicFallsBackWhenWriteFails(t *testing.T) {
	t.Parallel()

	r := newStubPanicReporter(vfs.NewMemMapFS(), "/wd", time.Now().UTC(), os.Getpid())
	r.WriteFile = func(string, []byte, os.FileMode) error {
		return errors.New("disk full")
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

	fs := vfs.NewMemMapFS()
	when := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	r := newStubPanicReporter(fs, "/wd", when, 1)

	r.ReportPanic(tlogger.CreateLogger(), "", "", nil, []string{})

	body, err := vfs.ReadFile(fs, "/wd/"+"terragrunt-crash-20260102T030405Z-1.log")
	require.NoError(t, err)

	content := string(body)

	assert.Contains(t, content, "Terragrunt version: unknown")
	assert.Contains(t, content, "Panic: (no panic message)")
	assert.Contains(t, content, "Command line: (empty command line)")
	assert.Contains(t, content, "(no stack trace was available)")
}

func TestReportPanicFallsBackToTempDirWhenGetwdFails(t *testing.T) {
	t.Parallel()

	fs := vfs.NewMemMapFS()
	when := time.Now().UTC()
	pid := os.Getpid()
	r := newStubPanicReporter(fs, "/wd", when, pid)
	r.Getwd = func() (string, error) { return "", errors.New("denied") }

	r.ReportPanic(tlogger.CreateLogger(), "1.7.9", "divide by zero", []byte("stack"), nil)

	expectedPath := filepath.Join("/tmp", "terragrunt-crash-"+when.UTC().Format("20060102T150405Z")+"-"+strconv.Itoa(pid)+".log")
	_, err := vfs.ReadFile(fs, expectedPath)
	require.NoError(t, err, "expected crash file at %s", expectedPath)
}

func TestPanicSuppressingWriter(t *testing.T) {
	t.Parallel()

	t.Run("forwards regular payloads", func(t *testing.T) {
		t.Parallel()

		var inner bytes.Buffer

		w := log.NewPanicSuppressingWriter(&inner)
		n, err := w.Write([]byte("regular log line\n"))
		require.NoError(t, err)
		assert.Equal(t, len("regular log line\n"), n)
		assert.Equal(t, "regular log line\n", inner.String())
	})

	t.Run("drops panic-bearing payloads", func(t *testing.T) {
		t.Parallel()

		var inner bytes.Buffer

		w := log.NewPanicSuppressingWriter(&inner)
		payload := "Error: Error in function call\nCall to function \"run_cmd\" failed: panic in function implementation: nil deref\n"
		n, err := w.Write([]byte(payload))
		require.NoError(t, err)
		assert.Equal(t, len(payload), n)
		assert.Empty(t, inner.String())
	})
}

func TestPanicDetails(t *testing.T) {
	t.Parallel()

	t.Run("nil returns empty values", func(t *testing.T) {
		t.Parallel()

		msg, stack := log.PanicDetails(nil)
		assert.Empty(t, msg)
		assert.Nil(t, stack)
	})

	t.Run("cty function.PanicError splits value and stack", func(t *testing.T) {
		t.Parallel()

		err := function.PanicError{Value: "nil deref", Stack: []byte("cty stack frames")}
		msg, stack := log.PanicDetails(err)

		assert.Equal(t, "nil deref", msg)
		assert.Equal(t, []byte("cty stack frames"), stack)
	})

	t.Run("wrapped cty panic still extracts via errors.As", func(t *testing.T) {
		t.Parallel()

		inner := function.PanicError{Value: "boom", Stack: []byte("inner stack")}
		wrapped := fmt.Errorf("evaluating: %w", inner)

		msg, stack := log.PanicDetails(wrapped)
		assert.Equal(t, "boom", msg)
		assert.Equal(t, []byte("inner stack"), stack)
	})

	t.Run("non-cty panic falls back to err.Error", func(t *testing.T) {
		t.Parallel()

		msg, stack := log.PanicDetails(errors.New("plain panic message"))
		assert.Equal(t, "plain panic message", msg)
		assert.Nil(t, stack)
	})
}

func TestIsPanic(t *testing.T) {
	t.Parallel()

	t.Run("nil is not a panic", func(t *testing.T) {
		t.Parallel()

		assert.False(t, log.IsPanic(nil))
	})

	t.Run("plain wrapped error is not a panic", func(t *testing.T) {
		t.Parallel()

		assert.False(t, log.IsPanic(errors.New("regular failure")))
		assert.False(t, log.IsPanic(errors.New("panic but not from runtime")))
		assert.False(t, log.IsPanic(errors.New("user requested panic shutdown")))
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

		err := errors.New("panic: simulated\n\ngoroutine 1 [running]:\nruntime/debug.Stack()\npanic({0x...})\n\t/usr/local/go/src/runtime/panic.go:860")

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

func newStubPanicReporter(fs vfs.FS, workDir string, now time.Time, pid int) *log.PanicReporter {
	return &log.PanicReporter{
		Now:       func() time.Time { return now },
		Getwd:     func() (string, error) { return workDir, nil },
		GetPID:    func() int { return pid },
		TempDir:   func() string { return "/tmp" },
		WriteFile: func(name string, data []byte, perm os.FileMode) error { return vfs.WriteFile(fs, name, data, perm) },
		BuildInfo: func() (string, bool) { return "deadbeef", true },
	}
}
