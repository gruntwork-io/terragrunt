// Package report provides a mechanism for collecting data on runs and generating a reports and summaries on that data.
package report

import (
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"sync"
	"time"
)

// Report captures data for a report/summary.
type Report struct {
	workingDir           string
	format               Format
	Runs                 []*Run
	mu                   sync.RWMutex
	shouldColor          bool
	showUnitLevelSummary bool
}

// Run captures data for a run.
type Run struct {
	Started time.Time
	Ended   time.Time
	Reason  *Reason
	Cause   *Cause
	Path    string
	Result  Result
	mu      sync.RWMutex
}

// Result captures the result of a run.
type Result string

// Reason captures the reason for a run.
type Reason string

// Cause captures the cause of a run.
type Cause string

// Format captures the format of a report.
type Format string

const (
	FormatCSV  Format = "csv"
	FormatJSON Format = "json"
)

const (
	ResultSucceeded Result = "succeeded"
	ResultFailed    Result = "failed"
	ResultEarlyExit Result = "early exit"
	ResultExcluded  Result = "excluded"
)

const (
	ReasonRetrySucceeded  Reason = "retry succeeded"
	ReasonErrorIgnored    Reason = "error ignored"
	ReasonRunError        Reason = "run error"
	ReasonExcludeDir      Reason = "--queue-exclude-dir"
	ReasonExcludeBlock    Reason = "exclude block"
	ReasonExcludeExternal Reason = "--queue-exclude-external"
	ReasonAncestorError   Reason = "ancestor error"
)

// NewReport creates a new report.
func NewReport() *Report {
	report := &Report{
		Runs:        make([]*Run, 0),
		shouldColor: true,
	}

	return report
}

// NewReportOption is an option for creating a new report.
type NewReportOption func(*Report)

// WithDisableColor sets the shouldColor flag for the report.
func (r *Report) WithDisableColor() *Report {
	r.shouldColor = false

	return r
}

// WithWorkingDir sets the working directory for the report.
func (r *Report) WithWorkingDir(workingDir string) *Report {
	r.workingDir = workingDir

	return r
}

// WithFormat sets the format for the report.
func (r *Report) WithFormat(format Format) *Report {
	r.format = format

	return r
}

// WithShowUnitLevelSummary sets the showUnitLevelSummary flag for the report.
//
// When enabled, the summary of the report will include timings for each unit.
func (r *Report) WithShowUnitLevelSummary() *Report {
	r.showUnitLevelSummary = true

	return r
}

// ErrPathMustBeAbsolute is returned when a report run path is not absolute.
var ErrPathMustBeAbsolute = errors.New("report run path must be absolute")

// NewRun creates a new run.
// The path passed in must be an absolute path to ensure that the run can be uniquely identified.
func NewRun(path string) (*Run, error) {
	if !filepath.IsAbs(path) {
		return nil, ErrPathMustBeAbsolute
	}

	return &Run{
		Path:    path,
		Started: time.Now(),
	}, nil
}

// ErrRunAlreadyExists is returned when a run already exists in the report.
var ErrRunAlreadyExists = errors.New("run already exists")

// AddRun adds a run to the report.
// If the run already exists, it returns the ErrRunAlreadyExists error.
func (r *Report) AddRun(run *Run) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	for _, existingRun := range r.Runs {
		if existingRun.Path == run.Path {
			return fmt.Errorf("%w: %s", ErrRunAlreadyExists, run.Path)
		}
	}

	r.Runs = append(r.Runs, run)

	return nil
}

// ErrRunNotFound is returned when a run is not found in the report.
var ErrRunNotFound = errors.New("run not found in report")

// GetRun returns a run from the report.
// The path passed in must be an absolute path to ensure that the run can be uniquely identified.
func (r *Report) GetRun(path string) (*Run, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if !filepath.IsAbs(path) {
		return nil, ErrPathMustBeAbsolute
	}

	for _, run := range r.Runs {
		if run.Path == path {
			return run, nil
		}
	}

	return nil, fmt.Errorf("%w: %s", ErrRunNotFound, path)
}

// EnsureRun tries to get a run from the report.
// If the run does not exist, it creates a new run and adds it to the report, then returns the run.
// This is useful when a run is being ended that might not have been started due to exclusion, etc.
func (r *Report) EnsureRun(path string) (*Run, error) {
	run, err := r.GetRun(path)
	if err == nil {
		return run, nil
	}

	if !errors.Is(err, ErrRunNotFound) {
		return run, err
	}

	run, err = NewRun(path)
	if err != nil {
		return run, err
	}

	if err = r.AddRun(run); err != nil {
		return run, err
	}

	return run, nil
}

// EndRun ends a run and adds it to the report.
// If the run does not exist, it returns the ErrRunNotFound error.
// By default, the run is assumed to have succeeded. To change this, pass WithResult to the function.
// If the run has already ended from an early exit, it does nothing.
func (r *Report) EndRun(path string, endOptions ...EndOption) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if !filepath.IsAbs(path) {
		return ErrPathMustBeAbsolute
	}

	var run *Run

	for _, existingRun := range r.Runs {
		if existingRun.Path == path {
			run = existingRun
			break
		}
	}

	if run == nil {
		return fmt.Errorf("%w: %s", ErrRunNotFound, path)
	}

	// If the run has already ended from an early exit or excluded, we don't need to do anything.
	if !run.Ended.IsZero() && (run.Result == ResultEarlyExit || run.Result == ResultExcluded) {
		return nil
	}

	run.mu.Lock()
	defer run.mu.Unlock()

	run.Ended = time.Now()
	run.Result = ResultSucceeded

	for _, endOption := range endOptions {
		endOption(run)
	}

	return nil
}

func (r *Report) SortRuns() {
	slices.SortFunc(r.Runs, func(a, b *Run) int {
		return a.Started.Compare(b.Started)
	})
}

// EndOption are optional configurations for ending a run.
type EndOption func(*Run)

// WithResult sets the result of a run.
func WithResult(result Result) EndOption {
	return func(run *Run) {
		run.Result = result
	}
}

// WithReason sets the reason of a run.
func WithReason(reason Reason) EndOption {
	return func(run *Run) {
		run.Reason = &reason
	}
}

// WithCauseRetryBlock sets the cause of a run to the name of a particular retry block.
//
// This function is a wrapper around withCause, just to make sure that authors always use consistent
// reasons for causes.
func WithCauseRetryBlock(name string) EndOption {
	return withCause(name)
}

// WithCauseIgnoreBlock sets the cause of a run to the name of a particular ignore block.
//
// This function is a wrapper around withCause, just to make sure that authors always use consistent
// reasons for causes.
func WithCauseIgnoreBlock(name string) EndOption {
	return withCause(name)
}

// WithCauseExcludeBlock sets the cause of a run to the name of a particular exclude block.
//
// This function is a wrapper around withCause, just to make sure that authors always use consistent
// reasons for causes.
func WithCauseExcludeBlock(name string) EndOption {
	return withCause(name)
}

// WithCauseAncestorExit sets the cause of a run to the name of a particular ancestor that exited.
//
// This function is a wrapper around withCause, just to make sure that authors always use consistent
// reasons for causes.
func WithCauseAncestorExit(name string) EndOption {
	return withCause(name)
}

// WithCauseRunError sets the cause of a run to the name of a particular run error.
//
// This function is a wrapper around withCause, just to make sure that authors always use consistent
// reasons for causes.
func WithCauseRunError(name string) EndOption {
	return withCause(name)
}

// withCause sets the cause of a run to the name of a particular cause.
func withCause(name string) EndOption {
	return func(run *Run) {
		cause := Cause(name)
		run.Cause = &cause
	}
}
