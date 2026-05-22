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
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
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
	assert.Contains(t, logOutput, "A panic report has been saved to:")
	assert.Contains(t, logOutput, expectedPath)
	assert.Contains(t, logOutput, "1. Your Terragrunt version: 1.7.9")
	assert.Contains(t, logOutput, "2. The full panic report file linked above.")
	assert.Contains(t, logOutput, "Full error details follow")
	assert.Contains(t, logOutput, log.PanicIssueURL)
	assert.NotContains(t, logOutput, "Failed to save a panic report file")

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
	assert.Contains(t, logOutput, "Failed to save a panic report file: disk full")
	assert.Contains(t, logOutput, "Panic: slice bounds out of range")
	assert.Contains(t, logOutput, "TERRAGRUNT CRASH")
	assert.Contains(t, logOutput, "1. Your Terragrunt version: 1.7.9")
	assert.Contains(t, logOutput, "2. The full panic report shown below.")
	assert.Contains(t, logOutput, "Full error details follow")
	assert.Contains(t, logOutput, log.PanicIssueURL)
}

func TestReportPanicFallbacksOnEmptyInputs(t *testing.T) {
	t.Parallel()

	fs := vfs.NewMemMapFS()
	when := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	r := newStubPanicReporter(fs, "/wd", when, 1)

	r.ReportPanic(logger.CreateLogger(), "", "", nil, []string{})

	body, err := vfs.ReadFile(fs, "/wd/"+"terragrunt-crash-20260102T030405Z-1.log")
	require.NoError(t, err)

	content := string(body)

	assert.Contains(t, content, "Terragrunt version: unknown")
	assert.Contains(t, content, "Panic: (no panic message)")
	assert.Contains(t, content, "Command line: \n")
	assert.Contains(t, content, "(no stack trace was available)")
}

func TestReportPanicFallsBackToTempDirWhenGetwdFails(t *testing.T) {
	t.Parallel()

	fs := vfs.NewMemMapFS()
	when := time.Now().UTC()
	pid := os.Getpid()
	r := newStubPanicReporter(fs, "/wd", when, pid)
	r.Getwd = func() (string, error) { return "", errors.New("denied") }

	r.ReportPanic(logger.CreateLogger(), "1.7.9", "divide by zero", []byte("stack"), nil)

	expectedPath := filepath.Join("/tmp", "terragrunt-crash-"+when.UTC().Format("20060102T150405Z")+"-"+strconv.Itoa(pid)+".log")
	_, err := vfs.ReadFile(fs, expectedPath)
	require.NoError(t, err, "expected crash file at %s", expectedPath)
}

func TestReportPanicRetriesTempDirWhenCwdWriteFails(t *testing.T) {
	t.Parallel()

	fs := vfs.NewMemMapFS()
	when := time.Date(2026, 5, 22, 12, 30, 45, 0, time.UTC)
	pid := 4242
	r := newStubPanicReporter(fs, "/readonly", when, pid)
	// Reject writes under the cwd; accept anywhere else (i.e. /tmp).
	r.WriteFile = func(name string, data []byte, perm os.FileMode) error {
		if filepath.Dir(name) == "/readonly" {
			return errors.New("read-only filesystem")
		}

		return vfs.WriteFile(fs, name, data, perm)
	}

	logger, output := newPanicLogger()
	r.ReportPanic(logger, "1.7.9", "boom", []byte("stack"), []string{"terragrunt"})

	tempPath := "/tmp/terragrunt-crash-20260522T123045Z-4242.log"
	_, err := vfs.ReadFile(fs, tempPath)
	require.NoError(t, err, "expected fallback file at %s", tempPath)

	assert.Contains(t, output.String(), tempPath)
	assert.NotContains(t, output.String(), "Failed to save a panic report file")
}

func TestPanicHandler(t *testing.T) {
	t.Parallel()

	t.Run("returns false when rec is nil", func(t *testing.T) {
		t.Parallel()

		r := newStubPanicReporter(vfs.NewMemMapFS(), "/wd", time.Now().UTC(), 1)
		assert.False(t, r.PanicHandler(nil, logger.CreateLogger(), func() string { return "1.7.9" }, []string{"terragrunt"}))
	})

	t.Run("returns true and writes report when rec is set", func(t *testing.T) {
		t.Parallel()

		fs := vfs.NewMemMapFS()
		when := time.Date(2026, 5, 22, 12, 30, 45, 0, time.UTC)
		r := newStubPanicReporter(fs, "/wd", when, 9999)

		l, output := newPanicLogger()

		// recover() must be called from the deferred frame; pass the value through to PanicHandler.
		recovered := false

		func() {
			defer func() {
				recovered = r.PanicHandler(recover(), l, func() string { return "1.7.9" }, []string{"terragrunt", "plan"})
			}()

			panic("boom")
		}()

		assert.True(t, recovered)

		expectedPath := "/wd/terragrunt-crash-20260522T123045Z-9999.log"

		_, err := vfs.ReadFile(fs, expectedPath)
		require.NoError(t, err)
		assert.Contains(t, output.String(), "TERRAGRUNT CRASH")
		assert.Contains(t, output.String(), "Panic: boom")
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

	t.Run("ErrorStack wrapper surfaces captured stack", func(t *testing.T) {
		t.Parallel()

		// Wrap via fmt.Errorf so PanicDetails must walk the chain via errors.As to find the ErrorStack.
		inner := stackedError{
			msg:   "wrapped runtime panic",
			stack: "goroutine 1:\nruntime/panic.go:860\npanic({0x...})\n",
		}
		err := fmt.Errorf("outer: %w", inner)

		msg, stack := log.PanicDetails(err)
		assert.Equal(t, "outer: wrapped runtime panic", msg)
		assert.Equal(t, []byte(inner.stack), stack)
	})

	t.Run("ErrorStack without panic marker returns nil stack", func(t *testing.T) {
		t.Parallel()

		// A benign error that happens to expose an ErrorStack must NOT be returned as a panic stack.
		err := stackedError{msg: "ordinary failure", stack: "io.EOF\n\tnet/io.go:42\n"}

		msg, stack := log.PanicDetails(err)
		assert.Equal(t, "ordinary failure", msg)
		assert.Nil(t, stack, "ErrorStack lacking panic markers must not be returned")
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

	t.Run("plain error containing panic text is not classified as a Terragrunt panic", func(t *testing.T) {
		t.Parallel()

		// Subprocess (tofu/terraform) crash text bubbled up as a regular error must NOT trigger a Terragrunt crash banner.
		err := errors.New("panic: simulated\n\ngoroutine 1 [running]:\nruntime/debug.Stack()\npanic({0x...})\n\t/usr/local/go/src/runtime/panic.go:860")

		assert.False(t, log.IsPanic(err))
	})

	t.Run("matches a wrapped runtime.Error", func(t *testing.T) {
		t.Parallel()

		var rerr runtime.Error

		func() {
			defer func() {
				if r := recover(); r != nil {
					rerr, _ = r.(runtime.Error)
				}
			}()

			var arr []int

			_ = arr[5] //nolint:staticcheck // intentional out-of-bounds to obtain a runtime.Error
		}()

		assert.NotNil(t, rerr, "test setup: expected runtime panic")
		assert.True(t, log.IsPanic(fmt.Errorf("wrapped: %w", rerr)))
	})

	t.Run("matches an error whose ErrorStack contains a runtime panic frame", func(t *testing.T) {
		t.Parallel()

		err := stackedError{
			msg:   "wrapping a panic",
			stack: "goroutine 1:\nruntime/panic.go:860\npanic({0x...})\n",
		}

		assert.True(t, log.IsPanic(err))
	})

	t.Run("matches a panic inside an errors.Join multi-error", func(t *testing.T) {
		t.Parallel()

		panicErr := stackedError{
			msg:   "joined panic",
			stack: "goroutine 1:\nruntime/panic.go:860\npanic({0x...})\n",
		}

		joined := errors.Join(errors.New("first benign failure"), panicErr, errors.New("second benign failure"))

		assert.True(t, log.IsPanic(joined))
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
