// Package report provides a mechanism for collecting data on runs and generating a reports and summaries on that data.
package report

import (
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"os"
	"sync"
	"time"
)

// Report captures data for a report/summary.
type Report struct {
	Runs []*Run
}

// Run captures data for a run.
type Run struct {
	Started time.Time
	Ended   time.Time
	Reason  *Reason
	Cause   *Cause
	Name    string
	Result  Result
	mu      sync.RWMutex
}

// Result captures the result of a run.
type Result string

// Reason captures the reason for a run.
type Reason string

// Cause captures the cause of a run.
type Cause string

// Summary formats data from a report for output as a summary.
type Summary struct {
	firstRunStart  *time.Time
	lastRunEnd     *time.Time
	TotalUnits     int
	UnitsSucceeded int
	UnitsFailed    int
	EarlyExits     int
	Excluded       int
}

// NewReport creates a new report.
func NewReport() *Report {
	return &Report{
		Runs: make([]*Run, 0),
	}
}

// NewRun creates a new run.
func NewRun(name string) *Run {
	return &Run{
		Name:    name,
		Started: time.Now(),
	}
}

// ErrRunAlreadyExists is returned when a run already exists in the report.
var ErrRunAlreadyExists = errors.New("run already exists")

// AddRun adds a run to the report.
// If the run already exists, it returns the ErrRunAlreadyExists error.
func (r *Report) AddRun(run *Run) error {
	for _, existingRun := range r.Runs {
		if existingRun.Name == run.Name {
			return fmt.Errorf("%w: %s", ErrRunAlreadyExists, run.Name)
		}
	}

	r.Runs = append(r.Runs, run)

	return nil
}

// GetRun returns a run from the report.
func (r *Report) GetRun(name string) *Run {
	for _, run := range r.Runs {
		if run.Name == name {
			return run
		}
	}

	return nil
}

// ErrRunNotFound is returned when a run is not found in the report.
var ErrRunNotFound = errors.New("run not found")

// EndRun ends a run and adds it to the report.
// If the run does not exist, it returns the ErrRunNotFound error.
// By default, the run is assumed to have succeeded. To change this, pass WithResult to the function.
func (r *Report) EndRun(name string, endOptions ...EndOption) error {
	var run *Run

	for _, existingRun := range r.Runs {
		if existingRun.Name == name {
			run = existingRun
			break
		}
	}

	if run == nil {
		return fmt.Errorf("%w: %s", ErrRunNotFound, name)
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

// EndOption are optional configurations for ending a run.
type EndOption func(*Run)

const (
	ResultSucceeded Result = "succeeded"
	ResultFailed    Result = "failed"
	ResultEarlyExit Result = "early exit"
	ResultExcluded  Result = "excluded"
)

// WithResult sets the result of a run.
func WithResult(result Result) EndOption {
	return func(run *Run) {
		run.Result = result
	}
}

const (
	ReasonRetrySucceeded Reason = "retry succeeded"
	ReasonErrorIgnored   Reason = "error ignored"
	ReasonRunError       Reason = "run error"
	ReasonExcludeDir     Reason = "--exclude-dir"
	ReasonExcludeBlock   Reason = "exclude block"
	ReasonEarlyExit      Reason = "early exit"
)

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

// withCause sets the cause of a run to the name of a particular cause.
func withCause(name string) EndOption {
	return func(run *Run) {
		cause := Cause(name)
		run.Cause = &cause
	}
}

// Summarize returns a summary of the report.
func (r *Report) Summarize() *Summary {
	summary := &Summary{
		TotalUnits: len(r.Runs),
	}

	if len(r.Runs) == 0 {
		return summary
	}

	for _, run := range r.Runs {
		summary.Update(run)
	}

	return summary
}

func (s *Summary) Update(run *Run) {
	run.mu.RLock()
	defer run.mu.RUnlock()

	switch run.Result {
	case ResultSucceeded:
		s.UnitsSucceeded++
	case ResultFailed:
		s.UnitsFailed++
	case ResultEarlyExit:
		s.EarlyExits++
	case ResultExcluded:
		s.Excluded++
	}

	if s.firstRunStart == nil || run.Started.Before(*s.firstRunStart) {
		s.firstRunStart = &run.Started
	}

	if s.lastRunEnd == nil || run.Ended.After(*s.lastRunEnd) {
		s.lastRunEnd = &run.Ended
	}
}

// TotalDuration returns the total duration of all runs in the report.
func (s *Summary) TotalDuration() time.Duration {
	if s.firstRunStart == nil || s.lastRunEnd == nil {
		return 0
	}

	return s.lastRunEnd.Sub(*s.firstRunStart)
}

// WriteToFile writes the report to a file.
func (r *Report) WriteToFile(path string) error {
	file, err := os.Create(path)
	if err != nil {
		return err
	}

	defer file.Close()

	return r.WriteCSV(file)
}

// WriteCSV writes the report to a writer in CSV format.
func (r *Report) WriteCSV(w io.Writer) error {
	csvWriter := csv.NewWriter(w)
	defer csvWriter.Flush()

	err := csvWriter.Write([]string{
		"Name",
		"Started",
		"Ended",
		"Result",
		"Reason",
		"Cause",
	})
	if err != nil {
		return err
	}

	for _, run := range r.Runs {
		run.mu.RLock()
		defer run.mu.RUnlock()

		name := run.Name
		started := run.Started.Format(time.RFC3339)
		ended := run.Ended.Format(time.RFC3339)
		result := string(run.Result)
		reason := ""

		if run.Reason != nil {
			reason = string(*run.Reason)
		}

		cause := ""
		if run.Cause != nil {
			cause = string(*run.Cause)
		}

		err := csvWriter.Write([]string{
			name,
			started,
			ended,
			result,
			reason,
			cause,
		})
		if err != nil {
			return err
		}
	}

	return nil
}

// WriteSummary writes the summary to a writer.
func (r *Report) WriteSummary(w io.Writer) error {
	// Create a two line gap before the summary
	_, err := fmt.Fprintf(w, "\n\n")
	if err != nil {
		return err
	}

	// Write the summary
	return r.Summarize().Write(w)
}

// Write writes the summary to a writer.
func (s *Summary) Write(w io.Writer) error {
	_, err := fmt.Fprintf(w, "❯❯ Run Summary\n")
	if err != nil {
		return err
	}

	_, err = fmt.Fprintf(w, "Units: %d\n", s.TotalUnits)
	if err != nil {
		return err
	}

	_, err = fmt.Fprintf(w, "Duration: %s\n", s.TotalDuration())
	if err != nil {
		return err
	}

	if s.UnitsSucceeded > 0 {
		_, err := fmt.Fprintf(w, "Succeeded: %d\n", s.UnitsSucceeded)
		if err != nil {
			return err
		}
	}

	if s.UnitsFailed > 0 {
		_, err := fmt.Fprintf(w, "Failed: %d\n", s.UnitsFailed)
		if err != nil {
			return err
		}
	}

	if s.EarlyExits > 0 {
		_, err := fmt.Fprintf(w, "Early Exits: %d\n", s.EarlyExits)
		if err != nil {
			return err
		}
	}

	if s.Excluded > 0 {
		_, err := fmt.Fprintf(w, "Excluded: %d\n", s.Excluded)
		if err != nil {
			return err
		}
	}

	return nil
}
