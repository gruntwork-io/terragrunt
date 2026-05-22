package log

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

// PanicIssueURL is the canonical bug report URL shown in the crash banner.
const PanicIssueURL = "https://github.com/gruntwork-io/terragrunt/issues"

// panicMessageMarkers are substrings emitted only in cty/runtime panic messages.
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

// PanicReporter holds hooks used by the crash report path.
type PanicReporter struct {
	Now       func() time.Time
	Getwd     func() (string, error)
	GetPID    func() int
	TempDir   func() string
	WriteFile func(name string, data []byte, perm os.FileMode) error
	BuildInfo func() (commit string, modified bool)
}

// NewPanicReporter returns a PanicReporter wired with production defaults.
func NewPanicReporter() *PanicReporter {
	return &PanicReporter{
		Now:       time.Now,
		Getwd:     os.Getwd,
		GetPID:    os.Getpid,
		TempDir:   os.TempDir,
		WriteFile: os.WriteFile,
		BuildInfo: readBuildInfo,
	}
}

// PanicHandler must be invoked as defer r.PanicHandler(...) to catch panics.
func (r *PanicReporter) PanicHandler(l Logger, version func() string, args []string) {
	rec := recover()
	if rec == nil {
		return
	}

	v := ""
	if version != nil {
		v = version()
	}

	if v == "" {
		v = mainModuleVersion()
	}

	r.ReportPanic(l, v, fmt.Sprintf("%v", rec), debug.Stack(), args)
	os.Exit(1)
}

// ReportPanic writes the crash log and friendly banner for a panic surfaced as a returned error.
func (r *PanicReporter) ReportPanic(l Logger, version, panicMsg string, stack []byte, args []string) {
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

// PanicDetails returns (Value, Stack) split out of a cty function.PanicError.
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
func IsPanic(err error) bool {
	if err == nil {
		return false
	}

	var ctyPanic function.PanicError
	if stdErrors.As(err, &ctyPanic) {
		return true
	}

	if isPanicMessage(err.Error()) {
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

func (r *PanicReporter) writeLog(version, panicMsg string, stack []byte, args []string) (string, string, error) {
	now := r.now()
	pid := r.getPID()
	workingDir := r.workingDir()
	logPath := filepath.Join(workingDir, r.formatLogPath(now, pid))

	content := r.formatLog(version, panicMsg, stack, args, now, workingDir, pid)
	if err := r.writeFile(logPath, []byte(content), crashFileMode); err != nil {
		return "", content, err
	}

	return logPath, content, nil
}

// now returns r.Now() or time.Now if r.Now is nil.
func (r *PanicReporter) now() time.Time {
	if r.Now != nil {
		return r.Now()
	}

	return time.Now()
}

// getPID returns r.GetPID() or os.Getpid if r.GetPID is nil.
func (r *PanicReporter) getPID() int {
	if r.GetPID != nil {
		return r.GetPID()
	}

	return os.Getpid()
}

// writeFile delegates to r.WriteFile or os.WriteFile when unset.
func (r *PanicReporter) writeFile(name string, data []byte, perm os.FileMode) error {
	if r.WriteFile != nil {
		return r.WriteFile(name, data, perm)
	}

	return os.WriteFile(name, data, perm)
}

// workingDir returns the directory to write the crash log to.
func (r *PanicReporter) workingDir() string {
	getwd := os.Getwd
	if r.Getwd != nil {
		getwd = r.Getwd
	}

	if wd, err := getwd(); err == nil && wd != "" {
		return wd
	}

	if r.TempDir != nil {
		if td := r.TempDir(); td != "" {
			return td
		}
	}

	return os.TempDir()
}

func (r *PanicReporter) formatLog(
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

func (r *PanicReporter) formatLogPath(when time.Time, pid int) string {
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
