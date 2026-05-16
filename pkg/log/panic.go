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

// panicMessageMarkers are substrings emitted only in cty/runtime panic messages.
var panicMessageMarkers = []string{
	"panic in function implementation",
	"runtime/panic.go:",
	"panic({0x",
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

[1]: ` + PanicIssueURL + `

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

// PanicReporter holds the side-effecting hooks the crash-report path depends on.
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
		BuildInfo: ReadBuildInfo,
	}
}

// PanicHandler must be invoked as `defer r.PanicHandler(...)`; on a recovered panic it writes the crash report and exits 1.
func (r *PanicReporter) PanicHandler(l Logger, version func() string, args []string) {
	rec := recover()
	if rec == nil {
		return
	}

	v := ""
	if version != nil {
		v = version()
	}

	r.ReportPanic(l, v, fmt.Sprintf("%v", rec), debug.Stack(), args)
	os.Exit(1)
}

// ReportPanic writes the crash log and friendly banner for a panic surfaced as a returned error.
func (r *PanicReporter) ReportPanic(l Logger, version, panicMsg string, stack []byte, args []string) {
	logPath, logContent, writeErr := r.writeLog(version, panicMsg, stack, args)

	l.Error(panicOutput)

	if writeErr != nil {
		l.Errorf("Unable to write crash report: %v", writeErr)
		l.Errorf("Please report this issue at %s and include the crash report output below.", PanicIssueURL)
		l.Error(logContent)

		return
	}

	l.Errorf("A panic report has been saved to: %s", logPath)
	l.Errorf("Please report this issue at %s and attach the panic report.", PanicIssueURL)
}

// PanicDetails returns (Value, Stack) split out of a cty function.PanicError when present; otherwise (err.Error(), nil).
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

// IsPanic reports whether err originated from a recovered panic (cty function.PanicError or runtime stack marker on the unwrap chain).
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

// PanicSuppressingWriter forwards to Inner but drops any Write whose payload IsPanicMessage matches.
type PanicSuppressingWriter struct {
	Inner io.Writer
}

// NewPanicSuppressingWriter wraps inner so that panic-bearing writes are dropped.
func NewPanicSuppressingWriter(inner io.Writer) *PanicSuppressingWriter {
	return &PanicSuppressingWriter{Inner: inner}
}

// Write returns (len(p), nil) on a dropped payload to honor the io.Writer contract; any other return would surface as a short-write error to callers like io.MultiWriter.
func (w *PanicSuppressingWriter) Write(p []byte) (int, error) {
	if IsPanicMessage(string(p)) {
		return len(p), nil
	}

	return w.Inner.Write(p)
}

// IsPanicMessage reports whether s contains a cty or Go-runtime panic marker.
func IsPanicMessage(s string) bool {
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

// now returns r.Now() or time.Now if r.Now is nil; safeguards a partial PanicReporter literal in the crash path.
func (r *PanicReporter) now() time.Time {
	if r.Now == nil {
		return time.Now()
	}

	return r.Now()
}

// getPID returns r.GetPID() or os.Getpid if r.GetPID is nil.
func (r *PanicReporter) getPID() int {
	if r.GetPID == nil {
		return os.Getpid()
	}

	return r.GetPID()
}

// writeFile delegates to r.WriteFile or os.WriteFile when unset.
func (r *PanicReporter) writeFile(name string, data []byte, perm os.FileMode) error {
	if r.WriteFile == nil {
		return os.WriteFile(name, data, perm)
	}

	return r.WriteFile(name, data, perm)
}

func (r *PanicReporter) workingDir() string {
	if r.Getwd != nil {
		if wd, err := r.Getwd(); err == nil {
			return wd
		}
	} else if wd, err := os.Getwd(); err == nil {
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
