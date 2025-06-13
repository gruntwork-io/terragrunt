// Package report provides a mechanism for collecting data on runs and generating a reports and summaries on that data.
package report

import (
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/invopop/jsonschema"
	"github.com/mgutz/ansi"
)

// Report captures data for a report/summary.
type Report struct {
	workingDir     string
	format         Format
	Runs           []*Run
	mu             sync.RWMutex
	shouldColor    bool
	showUnitTiming bool
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

// Summary formats data from a report for output as a summary.
type Summary struct {
	firstRunStart  *time.Time
	lastRunEnd     *time.Time
	padder         string
	workingDir     string
	runs           []*Run
	UnitsSucceeded int
	UnitsFailed    int
	EarlyExits     int
	Excluded       int
	shouldColor    bool
	showUnitTiming bool
}

// Colorizer is a colorizer for the run summary output.
type Colorizer struct {
	headingColorizer     func(string) string
	successColorizer     func(string) string
	failureColorizer     func(string) string
	exitColorizer        func(string) string
	excludeColorizer     func(string) string
	successUnitColorizer func(string) string
	failureUnitColorizer func(string) string
	exitUnitColorizer    func(string) string
	excludeUnitColorizer func(string) string
	nanosecondColorizer  func(string) string
	microsecondColorizer func(string) string
	millisecondColorizer func(string) string
	secondColorizer      func(string) string
	minuteColorizer      func(string) string
	defaultColorizer     func(string) string
}

// NewColorizer creates a new Colorizer.
func NewColorizer(shouldColor bool) *Colorizer {
	// Check if unit colorizing is enabled via environment variable
	shouldColorizeUnits := shouldColor && os.Getenv(envTmpUndocumentedReportUnitColorize) != ""

	if !shouldColor {
		return &Colorizer{
			headingColorizer:     func(s string) string { return s },
			successColorizer:     func(s string) string { return s },
			failureColorizer:     func(s string) string { return s },
			exitColorizer:        func(s string) string { return s },
			excludeColorizer:     func(s string) string { return s },
			successUnitColorizer: func(s string) string { return s },
			failureUnitColorizer: func(s string) string { return s },
			exitUnitColorizer:    func(s string) string { return s },
			excludeUnitColorizer: func(s string) string { return s },
			nanosecondColorizer:  func(s string) string { return s },
			microsecondColorizer: func(s string) string { return s },
			millisecondColorizer: func(s string) string { return s },
			secondColorizer:      func(s string) string { return s },
			minuteColorizer:      func(s string) string { return s },
			defaultColorizer:     func(s string) string { return s },
		}
	}

	// Define unit colorizers based on environment variable
	var successUnitColorizer, failureUnitColorizer, exitUnitColorizer, excludeUnitColorizer func(string) string
	if shouldColorizeUnits {
		successUnitColorizer = ansi.ColorFunc("green+h")
		failureUnitColorizer = ansi.ColorFunc("red+h")
		exitUnitColorizer = ansi.ColorFunc("yellow+h")
		excludeUnitColorizer = ansi.ColorFunc("blue+h")
	} else {
		successUnitColorizer = func(s string) string { return s }
		failureUnitColorizer = func(s string) string { return s }
		exitUnitColorizer = func(s string) string { return s }
		excludeUnitColorizer = func(s string) string { return s }
	}

	return &Colorizer{
		headingColorizer:     ansi.ColorFunc("yellow+bh"),
		successColorizer:     ansi.ColorFunc("green+bh"),
		failureColorizer:     ansi.ColorFunc("red+bh"),
		exitColorizer:        ansi.ColorFunc("yellow+bh"),
		excludeColorizer:     ansi.ColorFunc("blue+bh"),
		successUnitColorizer: successUnitColorizer,
		failureUnitColorizer: failureUnitColorizer,
		exitUnitColorizer:    exitUnitColorizer,
		excludeUnitColorizer: excludeUnitColorizer,
		nanosecondColorizer:  ansi.ColorFunc("cyan+bh"),
		microsecondColorizer: ansi.ColorFunc("cyan+bh"),
		millisecondColorizer: ansi.ColorFunc("cyan+bh"),
		secondColorizer:      ansi.ColorFunc("green+bh"),
		minuteColorizer:      ansi.ColorFunc("yellow+bh"),
		defaultColorizer:     ansi.ColorFunc("white+bh"),
	}
}

// colorDuration returns the duration as a string, colored based on the duration.
func (c *Colorizer) colorDuration(duration time.Duration) string {
	if duration < time.Microsecond {
		return c.nanosecondColorizer(fmt.Sprintf("%dns", duration.Nanoseconds()))
	}

	if duration < time.Millisecond {
		return c.microsecondColorizer(fmt.Sprintf("%dµs", duration.Microseconds()))
	}

	if duration < time.Second {
		return c.millisecondColorizer(fmt.Sprintf("%dms", duration.Milliseconds()))
	}

	if duration < time.Minute {
		return c.secondColorizer(fmt.Sprintf("%ds", int(duration.Seconds())))
	}

	return c.minuteColorizer(fmt.Sprintf("%dm", int(duration.Minutes())))
}

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

// WithShowUnitTiming sets the showUnitTiming flag for the report.
//
// When enabled, the summary of the report will include timings for each unit.
func (r *Report) WithShowUnitTiming() *Report {
	r.showUnitTiming = true

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
	ReasonExcludeDir     Reason = "--queue-exclude-dir"
	ReasonExcludeBlock   Reason = "exclude block"
	ReasonAncestorError  Reason = "ancestor error"
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

// These are undocumented temporary environment variables that are used
// to play with the summary, so that we can experiment with it.
const (
	envTmpUndocumentedReportPadder       = "TMP_UNDOCUMENTED_REPORT_PADDER"
	envTmpUndocumentedReportUnitColorize = "TMP_UNDOCUMENTED_REPORT_UNIT_COLORIZE"
)

// Summarize returns a summary of the report.
func (r *Report) Summarize() *Summary {
	summary := &Summary{
		workingDir:     r.workingDir,
		shouldColor:    r.shouldColor,
		showUnitTiming: r.showUnitTiming,
		padder:         " ",
		runs:           r.Runs,
	}

	if os.Getenv(envTmpUndocumentedReportPadder) != "" {
		summary.padder = os.Getenv(envTmpUndocumentedReportPadder)
	}

	if len(r.Runs) == 0 {
		return summary
	}

	for _, run := range r.Runs {
		summary.Update(run)
	}

	return summary
}

func (s *Summary) TotalUnits() int {
	return len(s.runs)
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

// TotalDurationString returns the total duration of all runs in the report as a string.
// It returns the duration in the format that is easy to understand by humans.
func (s *Summary) TotalDurationString(colorizer *Colorizer) string {
	duration := s.TotalDuration()

	return colorizer.colorDuration(duration)
}

// WriteToFile writes the report to a file.
func (r *Report) WriteToFile(path string) error {
	tmpFile, err := os.CreateTemp("", "terragrunt-report-*")
	if err != nil {
		return err
	}

	r.mu.Lock()
	r.SortRuns()
	r.mu.Unlock()

	switch r.format {
	case FormatCSV:
		err = r.WriteCSV(tmpFile)
	case FormatJSON:
		err = r.WriteJSON(tmpFile)
	default:
		return fmt.Errorf("unsupported format: %s", r.format)
	}

	if err != nil {
		return fmt.Errorf("failed to write report: %w", err)
	}

	if err := tmpFile.Close(); err != nil {
		return fmt.Errorf("failed to close report file: %w", err)
	}

	if r.workingDir != "" && !filepath.IsAbs(path) {
		path = filepath.Join(r.workingDir, path)
	}

	return os.Rename(tmpFile.Name(), path)
}

// WriteCSV writes the report to a writer in CSV format.
func (r *Report) WriteCSV(w io.Writer) error {
	r.mu.RLock()
	defer r.mu.RUnlock()

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

		name := run.Path

		if r.workingDir != "" {
			name = strings.TrimPrefix(name, r.workingDir+string(os.PathSeparator))
		}

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

			if reason == string(ReasonAncestorError) && r.workingDir != "" {
				cause = strings.TrimPrefix(cause, r.workingDir+string(os.PathSeparator))
			}
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

// JSONRun represents a run in JSON format.
type JSONRun struct {
	// Started is the time when the run started.
	Started time.Time `json:"Started" jsonschema:"required"`
	// Ended is the time when the run ended.
	Ended time.Time `json:"Ended" jsonschema:"required"`
	// Reason is the reason for the run result, if any.
	Reason *string `json:"Reason,omitempty" jsonschema:"enum=retry succeeded,enum=error ignored,enum=run error,enum=--queue-exclude-dir,enum=exclude block,enum=ancestor error"`
	// Cause is the cause of the run result, if any.
	Cause *string `json:"Cause,omitempty"`
	// Name is the name of the run.
	Name string `json:"Name" jsonschema:"required"`
	// Result is the result of the run.
	Result string `json:"Result" jsonschema:"required,enum=succeeded,enum=failed,enum=early exit,enum=excluded"`
}

// WriteJSON writes the report to a writer in JSON format.
func (r *Report) WriteJSON(w io.Writer) error {
	r.mu.RLock()
	defer r.mu.RUnlock()

	runs := make([]JSONRun, 0, len(r.Runs))

	for _, run := range r.Runs {
		run.mu.RLock()
		defer run.mu.RUnlock()

		name := run.Path
		if r.workingDir != "" {
			name = strings.TrimPrefix(name, r.workingDir+string(os.PathSeparator))
		}

		jsonRun := JSONRun{
			Name:    name,
			Started: run.Started,
			Ended:   run.Ended,
			Result:  string(run.Result),
		}

		if run.Reason != nil {
			reason := string(*run.Reason)
			jsonRun.Reason = &reason
		}

		if run.Cause != nil {
			cause := string(*run.Cause)
			if run.Reason != nil && *run.Reason == ReasonAncestorError && r.workingDir != "" {
				cause = strings.TrimPrefix(cause, r.workingDir+string(os.PathSeparator))
			}

			jsonRun.Cause = &cause
		}

		runs = append(runs, jsonRun)
	}

	jsonBytes, err := json.MarshalIndent(runs, "", "  ")
	if err != nil {
		return err
	}

	jsonBytes = append(jsonBytes, '\n')

	_, err = w.Write(jsonBytes)

	return err
}

// WriteSchemaToFile writes a JSON schema for the report to a file.
func (r *Report) WriteSchemaToFile(path string) error {
	tmpFile, err := os.CreateTemp("", "terragrunt-schema-*")
	if err != nil {
		return err
	}

	if err := r.WriteSchema(tmpFile); err != nil {
		return fmt.Errorf("failed to write schema: %w", err)
	}

	if err := tmpFile.Close(); err != nil {
		return fmt.Errorf("failed to close schema file: %w", err)
	}

	if r.workingDir != "" && !filepath.IsAbs(path) {
		path = filepath.Join(r.workingDir, path)
	}

	return os.Rename(tmpFile.Name(), path)
}

// WriteSchema writes a JSON schema for the report to a writer.
func (r *Report) WriteSchema(w io.Writer) error {
	reflector := jsonschema.Reflector{
		DoNotReference: true,
	}

	schema := reflector.Reflect(&JSONRun{})

	schema.Description = "Schema for Terragrunt run report"
	schema.Title = "Terragrunt Run Report Schema"
	schema.ID = "https://terragrunt.gruntwork.io/schemas/run/report/v1/schema.json"

	arraySchema := &jsonschema.Schema{
		Type:        "array",
		Title:       "Terragrunt Run Report Schema",
		Description: "Array of Terragrunt runs",
		Items:       schema,
	}

	jsonBytes, err := json.MarshalIndent(arraySchema, "", "  ")
	if err != nil {
		return err
	}

	jsonBytes = append(jsonBytes, '\n')

	_, err = w.Write(jsonBytes)

	return err
}

// WriteSummary writes the summary to a writer.
func (r *Report) WriteSummary(w io.Writer) error {
	_, err := fmt.Fprintf(w, "\n")
	if err != nil {
		return err
	}

	err = r.Summarize().Write(w)
	if err != nil {
		return err
	}

	_, err = fmt.Fprintf(w, "\n")
	if err != nil {
		return err
	}

	return nil
}

// Write writes the summary to a writer.
func (s *Summary) Write(w io.Writer) error {
	colorizer := NewColorizer(s.shouldColor)

	if s.showUnitTiming {
		return s.writeIntegratedSummary(w, colorizer)
	}

	header := fmt.Sprintf("%s  %d units  %s", runSummaryHeader, s.TotalUnits(), s.TotalDurationString(colorizer))
	if err := s.writeSummaryHeader(w, colorizer.headingColorizer(header)); err != nil {
		return err
	}

	separatorLine := fmt.Sprintf("%s%s", prefix, strings.Repeat("─", separatorLineLength))
	if err := s.writeSummaryHeader(w, separatorLine); err != nil {
		return err
	}

	if s.UnitsSucceeded > 0 {
		if err := s.writeSummaryEntry(w, colorizer.successColorizer(successLabel), colorizer.successUnitColorizer(strconv.Itoa(s.UnitsSucceeded))); err != nil {
			return err
		}
	}

	if s.UnitsFailed > 0 {
		if err := s.writeSummaryEntry(w, colorizer.failureColorizer(failureLabel), colorizer.failureUnitColorizer(strconv.Itoa(s.UnitsFailed))); err != nil {
			return err
		}
	}

	if s.EarlyExits > 0 {
		if err := s.writeSummaryEntry(w, colorizer.exitColorizer(earlyExitLabel), colorizer.exitUnitColorizer(strconv.Itoa(s.EarlyExits))); err != nil {
			return err
		}
	}

	if s.Excluded > 0 {
		if err := s.writeSummaryEntry(w, colorizer.excludeColorizer(excludeLabel), colorizer.excludeUnitColorizer(strconv.Itoa(s.Excluded))); err != nil {
			return err
		}
	}

	return nil
}

const (
	prefix                  = "   "
	unitPrefixMultiplier    = 2
	runSummaryHeader        = "❯❯ Run Summary"
	durationLabel           = "Duration"
	unitsLabel              = "Units"
	successLabel            = "Succeeded"
	failureLabel            = "Failed"
	earlyExitLabel          = "Early Exits"
	excludeLabel            = "Excluded"
	separator               = "  "
	separatorLineLength     = 28
	durationAlignmentOffset = 4
	headerUnitCountSpacing  = 2
)

func (s *Summary) writeSummaryHeader(w io.Writer, value string) error {
	_, err := fmt.Fprintf(w, "%s\n", value)
	if err != nil {
		return err
	}

	return nil
}

func (s *Summary) writeSummaryEntry(w io.Writer, label string, value string) error {
	_, err := fmt.Fprintf(w, "%s%s%s%s%s\n", prefix, label, separator, s.padding(label), value)
	if err != nil {
		return err
	}

	return nil
}

// writeIntegratedSummary writes the summary with integrated unit timing grouped by categories
func (s *Summary) writeIntegratedSummary(w io.Writer, colorizer *Colorizer) error {
	header := fmt.Sprintf("%s  %d units  %s", runSummaryHeader, s.TotalUnits(), s.TotalDurationString(colorizer))
	if err := s.writeSummaryHeader(w, colorizer.headingColorizer(header)); err != nil {
		return err
	}

	separatorLine := fmt.Sprintf("%s%s", prefix, strings.Repeat("─", separatorLineLength))
	if err := s.writeSummaryHeader(w, separatorLine); err != nil {
		return err
	}

	resultGroups := map[Result][]*Run{
		ResultSucceeded: {},
		ResultFailed:    {},
		ResultEarlyExit: {},
		ResultExcluded:  {},
	}

	for _, run := range s.runs {
		resultGroups[run.Result] = append(resultGroups[run.Result], run)
	}

	categories := []struct {
		colorizer     func(string) string
		unitColorizer func(string) string
		result        Result
		label         string
		count         int
	}{
		{
			colorizer:     colorizer.successColorizer,
			unitColorizer: colorizer.successUnitColorizer,
			result:        ResultSucceeded,
			label:         successLabel,
			count:         s.UnitsSucceeded,
		},
		{
			colorizer:     colorizer.failureColorizer,
			unitColorizer: colorizer.failureUnitColorizer,
			result:        ResultFailed,
			label:         failureLabel,
			count:         s.UnitsFailed,
		},
		{
			colorizer:     colorizer.exitColorizer,
			unitColorizer: colorizer.exitUnitColorizer,
			result:        ResultEarlyExit,
			label:         earlyExitLabel,
			count:         s.EarlyExits,
		},
		{
			colorizer:     colorizer.excludeColorizer,
			unitColorizer: colorizer.excludeUnitColorizer,
			result:        ResultExcluded,
			label:         excludeLabel,
			count:         s.Excluded,
		},
	}

	for _, category := range categories {
		if category.count > 0 {
			categoryHeader := fmt.Sprintf("%s (%d)", category.label, category.count)

			categoryHeaderColored := category.colorizer(categoryHeader)
			if _, err := fmt.Fprintf(w, "%s%s\n", prefix, categoryHeaderColored); err != nil {
				return err
			}

			runs := resultGroups[category.result]
			slices.SortFunc(runs, func(a, b *Run) int {
				aDuration := a.Ended.Sub(a.Started)
				bDuration := b.Ended.Sub(b.Started)

				return int(bDuration - aDuration)
			})

			for _, run := range runs {
				if err := s.writeCleanUnitTiming(w, run, colorizer, category.unitColorizer); err != nil {
					return err
				}
			}
		}
	}

	return nil
}

// writeCleanUnitTiming writes unit timing with cleaner formatting (no colons, better alignment)
func (s *Summary) writeCleanUnitTiming(w io.Writer, run *Run, colorizer *Colorizer, unitColorizer func(string) string) error {
	duration := run.Ended.Sub(run.Started)

	name := run.Path
	if s.workingDir != "" {
		name = strings.TrimPrefix(name, s.workingDir+string(os.PathSeparator))
	}

	padding := s.cleanUnitDurationPadding(name)

	_, err := fmt.Fprintf(
		w, "%s%s%s%s\n",
		strings.Repeat(prefix, unitPrefixMultiplier),
		unitColorizer(name),
		padding,
		colorizer.colorDuration(duration),
	)
	if err != nil {
		return err
	}

	return nil
}

func (s *Summary) padding(label string) string {
	headerUnitCountVisualPosition := s.visualLength(runSummaryHeader) + headerUnitCountSpacing

	currentLabelLength := s.visualLength(label)
	currentPosition := len(prefix) + currentLabelLength + len(separator)

	paddingNeeded := headerUnitCountVisualPosition - currentPosition

	paddingNeeded -= 4

	if paddingNeeded < 0 {
		paddingNeeded = 0
	}

	return strings.Repeat(s.padder, paddingNeeded)
}

// visualLength calculates the visual length of a string by removing ANSI escape codes
func (s *Summary) visualLength(text string) int {
	ansiRegex := regexp.MustCompile(`\x1b\[[0-9;]*m`)
	cleanText := ansiRegex.ReplaceAllString(text, "")

	return len(cleanText)
}

// cleanUnitDurationPadding calculates padding for unit names to align durations with header
func (s *Summary) cleanUnitDurationPadding(name string) string {
	headerPrefix := fmt.Sprintf("%s  %d units  ", runSummaryHeader, s.TotalUnits())
	headerDurationColumn := len(headerPrefix)

	unitPrefix := strings.Repeat(prefix, unitPrefixMultiplier)
	currentPosition := len(unitPrefix) + len(name)

	paddingNeeded := headerDurationColumn - currentPosition - durationAlignmentOffset

	if paddingNeeded < 1 {
		paddingNeeded = 1
	}

	return strings.Repeat(s.padder, paddingNeeded)
}
