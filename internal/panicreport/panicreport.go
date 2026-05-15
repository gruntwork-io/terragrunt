// Package panicreport produces OpenTofu/Terraform-style crash reports for
// recovered panics: a friendly banner pointing the user to the issue tracker
// and a detailed crash log file with runtime context and the panic stack.
//
// Typical use from main:
//
//	reporter := panicreport.New("1.0.5")
//	defer reporter.PanicHandler(logger, os.Args)
//
// For panics that surface as errors returned from another component (e.g.
// a cty function.PanicError bubbled out of HCL evaluation):
//
//	if panicreport.IsPanic(err) {
//		reporter.ReportPanic(logger, err.Error(), nil, os.Args)
//		os.Exit(1)
//	}
package panicreport

import (
	stdErrors "errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"strconv"
	"strings"
	"time"

	"github.com/zclconf/go-cty/cty/function"
)

// IssueURL is the canonical bug-report destination shown in the crash banner.
const IssueURL = "https://github.com/gruntwork-io/terragrunt/issues"

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

// runtimePanicMarkers are substrings that runtime/debug.Stack only emits
// while a goroutine is unwinding from a real panic (`panic(...)`, sigpanic
// from nil deref / divide-by-zero, etc.). Used by IsPanic for the
// stack-trace heuristic.
var runtimePanicMarkers = []string{
	"runtime/panic.go",
	"panic({",
}

// Logger is the narrow surface this package needs. *log.Logger from
// github.com/gruntwork-io/terragrunt/pkg/log satisfies it.
type Logger interface {
	Error(args ...any)
	Errorf(format string, args ...any)
}

// Reporter writes crash reports. The exported function-typed fields are
// stubbed in tests for deterministic output (time, working directory, PID,
// file write, build info). Fields are ordered to minimize struct padding.
type Reporter struct {
	Now       func() time.Time
	Getwd     func() (string, error)
	GetPID    func() int
	WriteFile func(name string, data []byte, perm os.FileMode) error
	BuildInfo func() (commit string, modified bool)
	Version   string
}

// New returns a Reporter wired with production defaults.
func New(version string) *Reporter {
	return &Reporter{
		Version:   version,
		Now:       time.Now,
		Getwd:     os.Getwd,
		GetPID:    os.Getpid,
		WriteFile: os.WriteFile,
		BuildInfo: ReadBuildInfo,
	}
}

// PanicHandler catches a panic from the deferred call site, writes a crash
// report, prints the banner via l, and exits with code 1. Must be invoked
// as `defer r.PanicHandler(...)` — wrapping it inside another deferred
// closure makes the internal recover() return nil and silently swallow the
// panic. Mirrors OpenTofu's logging.PanicHandler.
func (r *Reporter) PanicHandler(l Logger, args []string) {
	rec := recover()
	if rec == nil {
		return
	}

	r.ReportPanic(l, fmt.Sprintf("%v", rec), debug.Stack(), args)
	os.Exit(1)
}

// ReportPanic writes a crash log and a friendly banner for an
// already-captured panic. Use this when the panic surfaced as an error
// returned from another component (for example cty's function.PanicError).
// Pass nil stack to use the message text alone.
func (r *Reporter) ReportPanic(l Logger, panicMsg string, stack []byte, args []string) {
	logPath, logContent, writeErr := r.writeLog(panicMsg, stack, args)

	l.Error(fmt.Sprintf(panicOutput, IssueURL))

	if writeErr != nil {
		l.Errorf("Unable to write crash report: %v", writeErr)
		l.Errorf("Please report this issue at %s and include the crash report output below.", IssueURL)
		l.Error(logContent)

		return
	}

	l.Errorf("A panic report has been saved to: %s", logPath)
	l.Errorf("Please report this issue at %s and attach the panic report.", IssueURL)
}

// IsPanic reports whether err originated from a recovered panic. Detection
// is type-driven first (cty's function.PanicError, returned when an HCL
// function implementation panics) and falls back to inspecting the unwrap
// chain for a runtime panic frame (`panic({` and `runtime/panic.go`) — the
// signature debug.Stack always emits during panic unwinding, regardless of
// whether the trigger was an explicit panic() or a runtime fault.
func IsPanic(err error) bool {
	if err == nil {
		return false
	}

	var ctyPanic function.PanicError
	if stdErrors.As(err, &ctyPanic) {
		return true
	}

	if hasPanicFrame(err.Error()) {
		return true
	}

	if e, ok := err.(interface{ ErrorStack() string }); ok && hasPanicFrame(e.ErrorStack()) {
		return true
	}

	return false
}

// ReadBuildInfo extracts the VCS commit and dirty flag baked into the binary
// by `go build`. Helps disambiguate custom builds (forks, container images,
// "works on my machine" scenarios) when triaging crash reports. Exported so
// callers may compose their own Reporter instances.
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

func (r *Reporter) writeLog(panicMsg string, stack []byte, args []string) (string, string, error) {
	now := r.Now()
	pid := r.GetPID()
	workingDir := r.workingDir()
	logPath := filepath.Join(workingDir, r.formatLogPath(now, pid))

	content := r.formatLog(panicMsg, stack, args, now, workingDir, pid)
	if err := r.WriteFile(logPath, []byte(content), crashFileMode); err != nil {
		return "", content, err
	}

	return logPath, content, nil
}

func (r *Reporter) workingDir() string {
	wd, err := r.Getwd()
	if err == nil {
		return wd
	}

	return os.TempDir()
}

func (r *Reporter) formatLog(panicMsg string, stack []byte, args []string, when time.Time, workingDir string, pid int) string {
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

	version := r.Version
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

func (r *Reporter) formatLogPath(when time.Time, pid int) string {
	return crashLogPrefix + "-" + when.UTC().Format(crashLogFileTimeLayout) + "-" + strconv.Itoa(pid) + ".log"
}

func hasPanicFrame(s string) bool {
	if s == "" {
		return false
	}

	for _, marker := range runtimePanicMarkers {
		if strings.Contains(s, marker) {
			return true
		}
	}

	return false
}
