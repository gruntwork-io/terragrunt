package report

import (
	"bytes"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNewReport(t *testing.T) {
	t.Parallel()

	report := NewReport()
	assert.NotNil(t, report)
	assert.NotNil(t, report.runs)
	assert.Empty(t, report.runs)
}

func TestNewRun(t *testing.T) {
	t.Parallel()

	name := "test-run"
	run := NewRun(name)
	assert.NotNil(t, run)
	assert.Equal(t, name, run.Name)
	assert.False(t, run.Started.IsZero())
	assert.True(t, run.Ended.IsZero())
	assert.Empty(t, run.Result)
	assert.Nil(t, run.Reason)
	assert.Nil(t, run.Cause)
}

func TestAddRun(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		run     *Run
		wantErr bool
	}{
		{
			name:    "successful add",
			run:     NewRun("test-run"),
			wantErr: false,
		},
		{
			name:    "duplicate run",
			run:     NewRun("test-run"),
			wantErr: true,
		},
	}

	report := NewReport()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := report.AddRun(tt.run)
			if tt.wantErr {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), "run already exists")
			} else {
				assert.NoError(t, err)
				assert.Len(t, report.runs, 1)
			}
		})
	}
}

func TestGetRun(t *testing.T) {
	t.Parallel()

	report := NewReport()
	run := NewRun("test-run")
	report.AddRun(run)

	tests := []struct {
		name    string
		runName string
		wantNil bool
	}{
		{
			name:    "existing run",
			runName: "test-run",
			wantNil: false,
		},
		{
			name:    "non-existent run",
			runName: "non-existent",
			wantNil: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			run := report.GetRun(tt.runName)
			if tt.wantNil {
				assert.Nil(t, run)
			} else {
				assert.NotNil(t, run)
				assert.Equal(t, tt.runName, run.Name)
			}
		})
	}
}

func TestEndRun(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		runName    string
		options    []EndOption
		wantErr    bool
		wantResult Result
		wantReason *Reason
		wantCause  *Cause
	}{
		{
			name:       "successful end",
			runName:    "test-run",
			options:    []EndOption{},
			wantErr:    false,
			wantResult: ResultSucceeded,
		},
		{
			name:    "non-existent run",
			runName: "non-existent",
			options: []EndOption{},
			wantErr: true,
		},
		{
			name:       "with result",
			runName:    "test-run-2",
			options:    []EndOption{WithResult(ResultFailed)},
			wantErr:    false,
			wantResult: ResultFailed,
		},
		{
			name:       "with reason",
			runName:    "test-run-3",
			options:    []EndOption{WithReason(ReasonRunError)},
			wantErr:    false,
			wantResult: ResultSucceeded,
			wantReason: func() *Reason { r := ReasonRunError; return &r }(),
		},
		{
			name:       "with cause",
			runName:    "test-run-4",
			options:    []EndOption{WithCauseRetryBlock("test-block")},
			wantErr:    false,
			wantResult: ResultSucceeded,
			wantCause:  func() *Cause { c := Cause("test-block"); return &c }(),
		},
	}

	report := NewReport()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if !tt.wantErr {
				run := NewRun(tt.runName)
				report.AddRun(run)
			}

			err := report.EndRun(tt.runName, tt.options...)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				run := report.GetRun(tt.runName)
				assert.Equal(t, tt.wantResult, run.Result)
				if tt.wantReason != nil {
					assert.NotNil(t, run.Reason)
					assert.Equal(t, *tt.wantReason, *run.Reason)
				}
				if tt.wantCause != nil {
					assert.NotNil(t, run.Cause)
					assert.Equal(t, *tt.wantCause, *run.Cause)
				}
				assert.False(t, run.Ended.IsZero())
			}
		})
	}
}

func TestSummarize(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		runs []struct {
			name   string
			result Result
		}
		wantTotalUnits int
		wantSucceeded  int
		wantFailed     int
		wantEarlyExits int
		wantExcluded   int
	}{
		{
			name: "empty report",
			runs: []struct {
				name   string
				result Result
			}{},
			wantTotalUnits: 0,
		},
		{
			name: "single successful run",
			runs: []struct {
				name   string
				result Result
			}{
				{"run1", ResultSucceeded},
			},
			wantTotalUnits: 1,
			wantSucceeded:  1,
		},
		{
			name: "mixed results",
			runs: []struct {
				name   string
				result Result
			}{
				{"run1", ResultSucceeded},
				{"run2", ResultFailed},
				{"run3", ResultEarlyExit},
				{"run4", ResultExcluded},
			},
			wantTotalUnits: 4,
			wantSucceeded:  1,
			wantFailed:     1,
			wantEarlyExits: 1,
			wantExcluded:   1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			report := NewReport()
			for _, r := range tt.runs {
				run := NewRun(r.name)
				report.AddRun(run)
				report.EndRun(r.name, WithResult(r.result))
			}

			summary := report.Summarize()
			assert.Equal(t, tt.wantTotalUnits, summary.TotalUnits)
			assert.Equal(t, tt.wantSucceeded, summary.UnitsSucceeded)
			assert.Equal(t, tt.wantFailed, summary.UnitsFailed)
			assert.Equal(t, tt.wantEarlyExits, summary.EarlyExits)
			assert.Equal(t, tt.wantExcluded, summary.Excluded)
		})
	}
}

func TestWriteCSV(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		setup    func(*Report)
		expected []string
	}{
		{
			name: "single successful run",
			setup: func(r *Report) {
				run := NewRun("successful-run")
				r.AddRun(run)
				r.EndRun("successful-run")
			},
			expected: []string{
				"Name,Started,Ended,Result,Reason,Cause",
				"successful-run,",
				"succeeded",
				"",
				"",
			},
		},
		{
			name: "complex mixed results",
			setup: func(r *Report) {
				// Add successful run
				successRun := NewRun("success-run")
				r.AddRun(successRun)
				r.EndRun("success-run")

				// Add failed run with reason
				failedRun := NewRun("failed-run")
				r.AddRun(failedRun)
				r.EndRun("failed-run", WithResult(ResultFailed), WithReason(ReasonRunError))

				// Add excluded run with cause
				excludedRun := NewRun("excluded-run")
				r.AddRun(excludedRun)
				r.EndRun("excluded-run", WithResult(ResultExcluded), WithCauseRetryBlock("test-block"))

				// Add early exit run with both reason and cause
				earlyExitRun := NewRun("early-exit-run")
				r.AddRun(earlyExitRun)
				r.EndRun("early-exit-run",
					WithResult(ResultEarlyExit),
					WithReason(ReasonRunError),
					WithCauseRetryBlock("another-block"),
				)
			},
			expected: []string{
				"Name,Started,Ended,Result,Reason,Cause",
				"success-run,",
				"succeeded",
				"",
				"",
				"failed-run,",
				"failed",
				"run error",
				"",
				"excluded-run,",
				"excluded",
				"",
				"test-block",
				"early-exit-run,",
				"early exit",
				"run error",
				"another-block",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			report := NewReport()
			tt.setup(report)

			var buf bytes.Buffer
			err := report.WriteCSV(&buf)
			assert.NoError(t, err)

			output := buf.String()
			for _, exp := range tt.expected {
				assert.Contains(t, output, exp)
			}
		})
	}
}

func TestWriteSummary(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		setup    func(*Report)
		expected string
	}{
		{
			name: "single successful run",
			setup: func(r *Report) {
				run := NewRun("successful-run")
				r.AddRun(run)
				r.EndRun("successful-run")
			},
			expected: `
❯❯ Run Summary
Total Units: 1
Total Duration: %s
Units Succeeded: 1
`,
		},
		{
			name: "complex mixed results",
			setup: func(r *Report) {
				// Add successful runs
				firstSuccessfulRun := NewRun("first-successful-run")
				r.AddRun(firstSuccessfulRun)
				r.EndRun("first-successful-run")

				secondSuccessfulRun := NewRun("second-successful-run")
				r.AddRun(secondSuccessfulRun)
				r.EndRun("second-successful-run")

				// Add failed runs
				firstFailedRun := NewRun("first-failed-run")
				r.AddRun(firstFailedRun)
				r.EndRun("first-failed-run", WithResult(ResultFailed))

				secondFailedRun := NewRun("second-failed-run")
				r.AddRun(secondFailedRun)
				r.EndRun("second-failed-run", WithResult(ResultFailed))

				// Add excluded runs
				firstExcludedRun := NewRun("first-excluded-run")
				r.AddRun(firstExcludedRun)
				r.EndRun("first-excluded-run", WithResult(ResultExcluded))

				secondExcludedRun := NewRun("second-excluded-run")
				r.AddRun(secondExcludedRun)
				r.EndRun("second-excluded-run", WithResult(ResultExcluded))

				// Add early exit runs
				firstEarlyExitRun := NewRun("first-early-exit-run")
				r.AddRun(firstEarlyExitRun)
				r.EndRun("first-early-exit-run", WithResult(ResultEarlyExit))

				secondEarlyExitRun := NewRun("second-early-exit-run")
				r.AddRun(secondEarlyExitRun)
				r.EndRun("second-early-exit-run", WithResult(ResultEarlyExit))
			},
			expected: `
❯❯ Run Summary
Total Units: 8
Total Duration: %s
Units Succeeded: 2
Units Failed: 2
Early Exits: 2
Excluded: 2
`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			report := NewReport()
			tt.setup(report)

			var buf bytes.Buffer
			err := report.WriteSummary(&buf)
			assert.NoError(t, err)

			output := buf.String()
			// Get the first and last run to calculate duration
			runs := report.runs
			var firstRun, lastRun *Run
			for _, run := range runs {
				if firstRun == nil || run.Started.Before(firstRun.Started) {
					firstRun = run
				}
				if lastRun == nil || run.Ended.After(lastRun.Ended) {
					lastRun = run
				}
			}

			expected := fmt.Sprintf(strings.TrimSpace(tt.expected), lastRun.Ended.Sub(firstRun.Started).String())
			assert.Equal(t, expected, strings.TrimSpace(output))
		})
	}
}
