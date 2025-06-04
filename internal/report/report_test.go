package report_test

import (
	"bytes"
	"fmt"
	"strings"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/report"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewReport(t *testing.T) {
	t.Parallel()

	report := report.NewReport()
	assert.NotNil(t, report)
	assert.NotNil(t, report.Runs)
	assert.Empty(t, report.Runs)
}

func TestNewRun(t *testing.T) {
	t.Parallel()

	name := "test-run"
	run := report.NewRun(name)
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
		run     *report.Run
		name    string
		wantErr bool
	}{
		{
			name:    "successful add",
			run:     report.NewRun("test-run"),
			wantErr: false,
		},
		{
			name:    "duplicate run",
			run:     report.NewRun("test-run"),
			wantErr: true,
		},
	}

	report := report.NewReport()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := report.AddRun(tt.run)
			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), "run already exists")
			} else {
				require.NoError(t, err)
				assert.Len(t, report.Runs, 1)
			}
		})
	}
}

func TestGetRun(t *testing.T) {
	t.Parallel()

	r := report.NewReport()
	run := report.NewRun("test-run")
	r.AddRun(run)

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

			run := r.GetRun(tt.runName)
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
		wantReason *report.Reason
		wantCause  *report.Cause
		name       string
		runName    string
		wantResult report.Result
		options    []report.EndOption
		wantErr    bool
	}{
		{
			name:       "successful end",
			runName:    "test-run",
			options:    []report.EndOption{},
			wantErr:    false,
			wantResult: report.ResultSucceeded,
		},
		{
			name:    "non-existent run",
			runName: "non-existent",
			options: []report.EndOption{},
			wantErr: true,
		},
		{
			name:       "with result",
			runName:    "test-run-2",
			options:    []report.EndOption{report.WithResult(report.ResultFailed)},
			wantErr:    false,
			wantResult: report.ResultFailed,
		},
		{
			name:       "with reason",
			runName:    "test-run-3",
			options:    []report.EndOption{report.WithReason(report.ReasonRunError)},
			wantErr:    false,
			wantResult: report.ResultSucceeded,
			wantReason: func() *report.Reason { r := report.ReasonRunError; return &r }(),
		},
		{
			name:       "with cause",
			runName:    "test-run-4",
			options:    []report.EndOption{report.WithCauseRetryBlock("test-block")},
			wantErr:    false,
			wantResult: report.ResultSucceeded,
			wantCause:  func() *report.Cause { c := report.Cause("test-block"); return &c }(),
		},
	}

	r := report.NewReport()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if !tt.wantErr {
				run := report.NewRun(tt.runName)
				r.AddRun(run)
			}

			err := r.EndRun(tt.runName, tt.options...)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				run := r.GetRun(tt.runName)
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
		name    string
		results []struct {
			name   string
			result report.Result
		}
		wantTotalUnits int
		wantSucceeded  int
		wantFailed     int
		wantEarlyExits int
		wantExcluded   int
	}{
		{
			name: "empty report",
			results: []struct {
				name   string
				result report.Result
			}{},
			wantTotalUnits: 0,
		},
		{
			name: "single successful run",
			results: []struct {
				name   string
				result report.Result
			}{
				{"run1", report.ResultSucceeded},
			},
			wantTotalUnits: 1,
			wantSucceeded:  1,
		},
		{
			name: "mixed results",
			results: []struct {
				name   string
				result report.Result
			}{
				{"run1", report.ResultSucceeded},
				{"run2", report.ResultFailed},
				{"run3", report.ResultEarlyExit},
				{"run4", report.ResultExcluded},
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

			r := report.NewReport()
			for _, result := range tt.results {
				run := report.NewRun(result.name)
				r.AddRun(run)
				r.EndRun(result.name, report.WithResult(result.result))
			}

			summary := r.Summarize()
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
		setup    func(*report.Report)
		expected []string
	}{
		{
			name: "single successful run",
			setup: func(r *report.Report) {
				run := report.NewRun("successful-run")
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
			setup: func(r *report.Report) {
				// Add successful run
				successRun := report.NewRun("success-run")
				r.AddRun(successRun)
				r.EndRun("success-run")

				// Add failed run with reason
				failedRun := report.NewRun("failed-run")
				r.AddRun(failedRun)
				r.EndRun("failed-run", report.WithResult(report.ResultFailed), report.WithReason(report.ReasonRunError))

				// Add excluded run with cause
				excludedRun := report.NewRun("excluded-run")
				r.AddRun(excludedRun)
				r.EndRun("excluded-run", report.WithResult(report.ResultExcluded), report.WithCauseRetryBlock("test-block"))

				// Add early exit run with both reason and cause
				earlyExitRun := report.NewRun("early-exit-run")
				r.AddRun(earlyExitRun)
				r.EndRun("early-exit-run",
					report.WithResult(report.ResultEarlyExit),
					report.WithReason(report.ReasonRunError),
					report.WithCauseRetryBlock("another-block"),
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
			t.Parallel()

			r := report.NewReport()
			tt.setup(r)

			var buf bytes.Buffer
			err := r.WriteCSV(&buf)
			require.NoError(t, err)

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
		setup    func(*report.Report)
		expected string
	}{
		{
			name: "single successful run",
			setup: func(r *report.Report) {
				run := report.NewRun("successful-run")
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
			setup: func(r *report.Report) {
				// Add successful runs
				firstSuccessfulRun := report.NewRun("first-successful-run")
				r.AddRun(firstSuccessfulRun)
				r.EndRun("first-successful-run")

				secondSuccessfulRun := report.NewRun("second-successful-run")
				r.AddRun(secondSuccessfulRun)
				r.EndRun("second-successful-run")

				// Add failed runs
				firstFailedRun := report.NewRun("first-failed-run")
				r.AddRun(firstFailedRun)
				r.EndRun("first-failed-run", report.WithResult(report.ResultFailed))

				secondFailedRun := report.NewRun("second-failed-run")
				r.AddRun(secondFailedRun)
				r.EndRun("second-failed-run", report.WithResult(report.ResultFailed))

				// Add excluded runs
				firstExcludedRun := report.NewRun("first-excluded-run")
				r.AddRun(firstExcludedRun)
				r.EndRun("first-excluded-run", report.WithResult(report.ResultExcluded))

				secondExcludedRun := report.NewRun("second-excluded-run")
				r.AddRun(secondExcludedRun)
				r.EndRun("second-excluded-run", report.WithResult(report.ResultExcluded))

				// Add early exit runs
				firstEarlyExitRun := report.NewRun("first-early-exit-run")
				r.AddRun(firstEarlyExitRun)
				r.EndRun("first-early-exit-run", report.WithResult(report.ResultEarlyExit))

				secondEarlyExitRun := report.NewRun("second-early-exit-run")
				r.AddRun(secondEarlyExitRun)
				r.EndRun("second-early-exit-run", report.WithResult(report.ResultEarlyExit))
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

			r := report.NewReport()
			tt.setup(r)

			var buf bytes.Buffer
			err := r.WriteSummary(&buf)
			require.NoError(t, err)

			output := buf.String()
			// Get the first and last run to calculate duration
			runs := r.Runs
			var firstRun, lastRun *report.Run
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
