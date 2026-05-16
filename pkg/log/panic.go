package log

import (
	stdErrors "errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"strconv"
	"strings"
	"time"

	"github.com/zclconf/go-cty/cty/function"
)

// PanicIssueURL is the canonical bug-report destination shown in the crash banner.
const PanicIssueURL = "https://github.com/gruntwork-io/terragrunt/issues"

// PanicMessageMarkers are substrings that appear only in messages carrying a
// recovered Go panic — the cty function.PanicError prefix and the runtime
// panic dispatcher frames emitted by debug.Stack.
var PanicMessageMarkers = []string{
	"panic in function implementation",
	"runtime/panic.go",
	"panic({",
}

const (
	crashLogPrefix         = "terragrunt-crash"
	crashLogFileTimeLayout = "20060102T150405Z"

	crashFileMode os.FileMode = 0o600

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
`
)

// VersionFunc returns the Terragrunt version string at the moment a crash is reported.
type VersionFunc func() string

// PanicReporter holds the side-effecting hooks the crash-report path depends on.
type PanicReporter struct {
	Now       func() time.Time
	Getwd     func() (string, error)
	GetPID    func() int
	WriteFile func(name string, data []byte, perm os.FileMode) error
	BuildInfo func() (commit string, modified bool)
}

// NewPanicReporter returns a PanicReporter wired with production defaults.
func NewPanicReporter() *PanicReporter {
	return &PanicReporter{
		Now:       time.Now,
		Getwd:     os.Getwd,
		GetPID:    os.Getpid,
		WriteFile: os.WriteFile,
		BuildInfo: ReadBuildInfo,
	}
}

// PanicHandler catches a panic from the deferred call site, writes a crash report, and exits with code 1.
// Must be invoked as `defer r.PanicHandler(...)` so recover() runs in the deferred function directly.
// version is a getter so callers can supply a value populated lazily.
func (r *PanicReporter) PanicHandler(l Logger, version VersionFunc, args []string) {
	rec := recover()
	if rec == nil {
		return
	}

	r.ReportPanic(l, callVersion(version), fmt.Sprintf("%v", rec), debug.Stack(), args)
	os.Exit(1)
}

// ReportPanic writes the crash log and friendly banner for an already-captured panic.
// Use this when the panic surfaced as an error returned from another component.
func (r *PanicReporter) ReportPanic(l Logger, version, panicMsg string, stack []byte, args []string) {
	logPath, logContent, writeErr := r.writeLog(version, panicMsg, stack, args)

	l.Error(fmt.Sprintf(panicOutput, PanicIssueURL))

	if writeErr != nil {
		l.Errorf("Unable to write crash report: %v", writeErr)
		l.Errorf("Please report this issue at %s and include the crash report output below.", PanicIssueURL)
		l.Error(logContent)

		return
	}

	l.Errorf("A panic report has been saved to: %s", logPath)
	l.Errorf("Please report this issue at %s and attach the panic report.", PanicIssueURL)
}

// PanicDetails extracts a clean panic message and stack from err.
// For cty's function.PanicError it splits the recovered Value from the Stack;
// for any other panic-shaped error the full err.Error() is returned as the message and stack is nil.
func PanicDetails(err error) (msg string, stack []byte) {
	if err == nil {
		return "", nil
	}

	var ctyPanic function.PanicError
	if stdErrors.As(err, &ctyPanic) {
		return fmt.Sprintf("%v", ctyPanic.Value), ctyPanic.Stack
	}

	return err.Error(), nil
}

// IsPanic reports whether err originated from a recovered panic.
// Detection is type-driven first (cty's function.PanicError) and falls back to IsPanicMessage on the message and on any ErrorStack found while walking the unwrap chain.
func IsPanic(err error) bool {
	if err == nil {
		return false
	}

	var ctyPanic function.PanicError
	if stdErrors.As(err, &ctyPanic) {
		return true
	}

	if IsPanicMessage(err.Error()) {
		return true
	}

	for cur := err; cur != nil; cur = stdErrors.Unwrap(cur) {
		if e, ok := cur.(interface{ ErrorStack() string }); ok && IsPanicMessage(e.ErrorStack()) {
			return true
		}
	}

	return false
}

// PanicSuppressingWriter wraps an io.Writer and drops payloads that
// IsPanicMessage matches. Suppression is per-Write boundary, so callers
// must emit each panic-bearing message in a single Write to be filtered
// (HCL's diagnostic writer does this today). The crash-report path
// surfaces those payloads separately via the crash log file.
type PanicSuppressingWriter struct {
	Inner io.Writer
}

// NewPanicSuppressingWriter wraps inner so that panic-bearing writes are dropped.
func NewPanicSuppressingWriter(inner io.Writer) *PanicSuppressingWriter {
	return &PanicSuppressingWriter{Inner: inner}
}

// Write drops the payload when it carries panic content; otherwise it forwards to Inner.
func (w *PanicSuppressingWriter) Write(p []byte) (int, error) {
	if IsPanicMessage(string(p)) {
		return len(p), nil
	}

	return w.Inner.Write(p)
}

// IsPanicMessage reports whether s contains any PanicMessageMarkers substring.
// Used by PanicSuppressingWriter to suppress noisy panic content that the crash-report path renders separately.
func IsPanicMessage(s string) bool {
	if s == "" {
		return false
	}

	for _, marker := range PanicMessageMarkers {
		if strings.Contains(s, marker) {
			return true
		}
	}

	return false
}

// ReadBuildInfo extracts the VCS commit and dirty flag baked into the binary by `go build`.
func ReadBuildInfo() (string, bool) {
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

// Private helper functions

func callVersion(fn VersionFunc) string {
	if fn == nil {
		return ""
	}

	return fn()
}

func (r *PanicReporter) writeLog(version, panicMsg string, stack []byte, args []string) (string, string, error) {
	now := r.Now()
	pid := r.GetPID()
	workingDir := r.workingDir()
	logPath := filepath.Join(workingDir, r.formatLogPath(now, pid))

	content := r.formatLog(version, panicMsg, stack, args, now, workingDir, pid)
	if err := r.WriteFile(logPath, []byte(content), crashFileMode); err != nil {
		return "", content, err
	}

	return logPath, content, nil
}

func (r *PanicReporter) workingDir() string {
	wd, err := r.Getwd()
	if err == nil {
		return wd
	}

	return os.TempDir()
}

func (r *PanicReporter) formatLog(version, panicMsg string, stack []byte, args []string, when time.Time, workingDir string, pid int) string {
	commit, modified := "unknown", false
	if r.BuildInfo != nil {
		commit, modified = r.BuildInfo()
	}

	stackStr := strings.TrimSpace(string(stack))
	if stackStr == "" {
		stackStr = "(no stack trace was available)"
	}

	command := strings.Join(args, " ")
	if command == "" {
		command = "(empty command line)"
	}

	if panicMsg == "" {
		panicMsg = "(no panic message)"
	}

	if version == "" {
		version = "unknown"
	}

	return fmt.Sprintf(
		crashLogTemplate,
		when.UTC().Format(time.RFC3339Nano),
		version,
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
		panicMsg,
		stackStr,
	)
}

func (r *PanicReporter) formatLogPath(when time.Time, pid int) string {
	return crashLogPrefix + "-" + when.UTC().Format(crashLogFileTimeLayout) + "-" + strconv.Itoa(pid) + ".log"
}
