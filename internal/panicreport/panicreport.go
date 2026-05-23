// Package panicreport collects a friendly crash report on top-level panics.
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

	"github.com/gruntwork-io/terragrunt/internal/vfs"
	// Imports pkg/log; pkg/log must never import internal/panicreport (would cycle via internal/vfs).
	"github.com/gruntwork-io/terragrunt/pkg/log"
)

// PanicIssueURL is the canonical bug report URL shown in the crash banner.
const PanicIssueURL = "https://github.com/gruntwork-io/terragrunt/issues"

// panicMessageMarkers are substrings found in cty error messages or Go runtime stack traces.
var panicMessageMarkers = []string{
	"panic in function implementation",
	"runtime/panic.go:",
	"panic({0x",
}

const (
	crashLogPrefix         = "terragrunt-crash"
	crashLogFileTimeLayout = "20060102T150405Z"

	unknownValue = "unknown"

	crashFileMode os.FileMode = 0o600

	panicBannerSuccess = `
***************************** TERRAGRUNT CRASH *****************************

Terragrunt crashed. This is always indicative of a bug within Terragrunt.

A panic report has been saved to:

    %s

The report includes the command line; review it for secrets before sharing.

To report this issue, please open a new ticket including the items below:

    1. Your Terragrunt version: %s
    2. The full panic report file linked above.
    3. Any additional information which may help reproduce the issue.

Open issues at: %s

************************ Full error details follow *************************
`

	panicBannerFallback = `
***************************** TERRAGRUNT CRASH *****************************

Terragrunt crashed. This is always indicative of a bug within Terragrunt.

Failed to save a panic report file: %s

The report includes the command line; review it for secrets before sharing.

To report this issue, please open a new ticket including the items below:

    1. Your Terragrunt version: %s
    2. The full panic report shown below.
    3. Any additional information which may help reproduce the issue.

Open issues at: %s

************************ Full error details follow *************************
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

// Reporter writes the crash banner and persists a crash log file on panic.
type Reporter struct {
	FS        vfs.FS
	Now       func() time.Time
	Getwd     func() (string, error)
	GetPID    func() int
	TempDir   func() string
	BuildInfo func() (commit string, modified bool)
}

// New returns a Reporter wired with production defaults.
func New() *Reporter {
	return &Reporter{
		FS:        vfs.NewOSFS(),
		Now:       time.Now,
		Getwd:     os.Getwd,
		GetPID:    os.Getpid,
		TempDir:   os.TempDir,
		BuildInfo: readBuildInfo,
	}
}

// PanicHandler reports rec when non-nil and returns true.
func (r *Reporter) PanicHandler(rec any, l log.Logger, version func() string, args []string) bool {
	if rec == nil {
		return false
	}

	v := ""
	if version != nil {
		v = version()
	}

	if v == "" {
		v = mainModuleVersion()
	}

	r.ReportPanic(l, v, fmt.Sprintf("%v", rec), debug.Stack(), args)

	return true
}

// ReportPanic writes the crash log and friendly banner for a panic surfaced as a returned error.
func (r *Reporter) ReportPanic(l log.Logger, version, panicMsg string, stack []byte, args []string) {
	logPath, logContent, writeErr := r.writeLog(version, panicMsg, stack, args)

	displayVersion := version
	if displayVersion == "" {
		displayVersion = unknownValue
	}

	if writeErr != nil {
		l.Errorf(panicBannerFallback, writeErr, displayVersion, PanicIssueURL)
		l.Error(logContent)

		return
	}

	l.Errorf(panicBannerSuccess, logPath, displayVersion, PanicIssueURL)
	l.Error(logContent)
}

// PanicDetails returns the panic message and stack split out of err; callers must gate on IsPanic.
func PanicDetails(err error) (msg string, stack []byte) {
	if err == nil {
		return "", nil
	}

	var ctyPanic function.PanicError
	if stdErrors.As(err, &ctyPanic) {
		return fmt.Sprintf("%v", ctyPanic.Value), ctyPanic.Stack
	}

	if s := findPanicStack(err); s != nil {
		return err.Error(), s
	}

	return err.Error(), nil
}

// findPanicStack walks single- and multi-error chains and returns the first ErrorStack whose content matches a panic marker.
func findPanicStack(err error) []byte {
	if err == nil {
		return nil
	}

	if e, ok := err.(interface{ ErrorStack() string }); ok {
		if s := e.ErrorStack(); isPanicMessage(s) {
			return []byte(s)
		}
	}

	if u, ok := err.(interface{ Unwrap() []error }); ok {
		for _, e := range u.Unwrap() {
			if s := findPanicStack(e); s != nil {
				return s
			}
		}

		return nil
	}

	return findPanicStack(stdErrors.Unwrap(err))
}

// IsPanic reports whether err originated from a recovered panic; only typed signals are honored to avoid subprocess-output false positives.
func IsPanic(err error) bool {
	if err == nil {
		return false
	}

	var ctyPanic function.PanicError
	if stdErrors.As(err, &ctyPanic) {
		return true
	}

	var runtimeErr runtime.Error
	if stdErrors.As(err, &runtimeErr) {
		return true
	}

	return hasPanicStack(err)
}

// hasPanicStack walks single- and multi-error chains looking for a panic stack.
func hasPanicStack(err error) bool {
	if err == nil {
		return false
	}

	if e, ok := err.(interface{ ErrorStack() string }); ok && isPanicMessage(e.ErrorStack()) {
		return true
	}

	if u, ok := err.(interface{ Unwrap() []error }); ok {
		for _, e := range u.Unwrap() {
			if hasPanicStack(e) {
				return true
			}
		}

		return false
	}

	return hasPanicStack(stdErrors.Unwrap(err))
}

// isPanicMessage reports whether s contains a cty or Go runtime panic marker.
func isPanicMessage(s string) bool {
	if s == "" {
		return false
	}

	for _, marker := range panicMessageMarkers {
		if strings.Contains(s, marker) {
			return true
		}
	}

	return false
}

// readBuildInfo extracts the VCS commit and dirty flag baked into the binary.
func readBuildInfo() (string, bool) {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return unknownValue, false
	}

	commit := unknownValue
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

// writeLog formats and persists the crash report; on cwd write failure it retries under TempDir.
func (r *Reporter) writeLog(version, panicMsg string, stack []byte, args []string) (string, string, error) {
	now := r.now()
	pid := r.getPID()
	workingDir := r.workingDir()
	content := r.formatLog(version, panicMsg, stack, args, now, workingDir, pid)
	fileName := r.formatLogPath(now, pid)
	logPath := filepath.Join(workingDir, fileName)

	err := r.writeFile(logPath, []byte(content), crashFileMode)
	if err == nil {
		return logPath, content, nil
	}

	// Cwd write rejected; try TempDir before giving up.
	tempDir := r.tempDir()
	if tempDir == "" || tempDir == workingDir {
		return "", content, err
	}

	tempPath := filepath.Join(tempDir, fileName)

	tempErr := r.writeFile(tempPath, []byte(content), crashFileMode)
	if tempErr == nil {
		return tempPath, content, nil
	}

	return "", content, stdErrors.Join(err, tempErr)
}

// tempDir returns r.TempDir() when set, else os.TempDir.
func (r *Reporter) tempDir() string {
	if r.TempDir != nil {
		return r.TempDir()
	}

	return os.TempDir()
}

// now returns r.Now() or time.Now if r.Now is nil.
func (r *Reporter) now() time.Time {
	if r.Now != nil {
		return r.Now()
	}

	return time.Now()
}

// getPID returns r.GetPID() or os.Getpid if r.GetPID is nil.
func (r *Reporter) getPID() int {
	if r.GetPID != nil {
		return r.GetPID()
	}

	return os.Getpid()
}

// writeFile writes through r.FS, falling back to a fresh OS FS when unset.
func (r *Reporter) writeFile(name string, data []byte, perm os.FileMode) error {
	fs := r.FS
	if fs == nil {
		fs = vfs.NewOSFS()
	}

	return vfs.WriteFile(fs, name, data, perm)
}

// workingDir returns cwd, or TempDir if cwd is empty or errors.
func (r *Reporter) workingDir() string {
	getwd := os.Getwd
	if r.Getwd != nil {
		getwd = r.Getwd
	}

	if wd, err := getwd(); err == nil && wd != "" {
		return wd
	}

	return r.tempDir()
}

// formatLog renders the crashLogTemplate with runtime context.
func (r *Reporter) formatLog(
	version, panicMsg string,
	stack []byte,
	args []string,
	when time.Time,
	workingDir string,
	pid int,
) string {
	commit, modified := unknownValue, false
	if r.BuildInfo != nil {
		commit, modified = r.BuildInfo()
	}

	stackStr := strings.TrimSpace(string(stack))
	if stackStr == "" {
		stackStr = "(no stack trace was available)"
	}

	command := strings.Join(args, " ")

	if panicMsg == "" {
		panicMsg = "(no panic message)"
	}

	if version == "" {
		version = unknownValue
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

// formatLogPath builds the crash log filename from timestamp and pid.
func (r *Reporter) formatLogPath(when time.Time, pid int) string {
	return crashLogPrefix + "-" + when.UTC().Format(crashLogFileTimeLayout) + "-" + strconv.Itoa(pid) + ".log"
}

// mainModuleVersion falls back to the binary's main module version when no caller version is available.
func mainModuleVersion() string {
	info, ok := debug.ReadBuildInfo()
	if !ok || info.Main.Version == "" {
		return ""
	}

	return info.Main.Version
}
