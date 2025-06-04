package report_test

import (
	"bytes"
	"path/filepath"
	"regexp"
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

	tmp := t.TempDir()

	path := filepath.Join(tmp, "test-run")
	run := newRun(t, path)
	assert.NotNil(t, run)
	assert.Equal(t, path, run.Name)
	assert.False(t, run.Started.IsZero())
	assert.True(t, run.Ended.IsZero())
	assert.Empty(t, run.Result)
	assert.Nil(t, run.Reason)
	assert.Nil(t, run.Cause)
}

func TestAddRun(t *testing.T) {
	t.Parallel()

	tmp := t.TempDir()

	path := filepath.Join(tmp, "test-run")

	tests := []struct {
		run     *report.Run
		name    string
		wantErr bool
	}{
		{
			name:    "successful add",
			run:     newRun(t, path),
			wantErr: false,
		},
		{
			name:    "duplicate run",
			run:     newRun(t, path),
			wantErr: true,
		},
	}

	r := report.NewReport()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := r.AddRun(tt.run)
			if tt.wantErr {
				require.Error(t, err)
				assert.ErrorIs(t, err, report.ErrRunAlreadyExists)
			} else {
				require.NoError(t, err)
				assert.Len(t, r.Runs, 1)
			}
		})
	}
}

func TestGetRun(t *testing.T) {
	t.Parallel()

	tmp := t.TempDir()

	r := report.NewReport()
	run := newRun(t, filepath.Join(tmp, "test-run"))
	r.AddRun(run)

	tests := []struct {
		name        string
		runName     string
		expectedErr error
	}{
		{
			name:    "existing run",
			runName: filepath.Join(tmp, "test-run"),
		},
		{
			name:        "non-existent run",
			runName:     filepath.Join(tmp, "non-existent"),
			expectedErr: report.ErrRunNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			run, err := r.GetRun(tt.runName)
			if tt.expectedErr != nil {
				assert.ErrorIs(t, err, tt.expectedErr)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.runName, run.Name)
			}
		})
	}
}

func TestEndRun(t *testing.T) {
	t.Parallel()

	tmp := t.TempDir()

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
			runName:    filepath.Join(tmp, "test-run"),
			options:    []report.EndOption{},
			wantErr:    false,
			wantResult: report.ResultSucceeded,
		},
		{
			name:    "non-existent run",
			runName: filepath.Join(tmp, "non-existent"),
			options: []report.EndOption{},
			wantErr: true,
		},
		{
			name:       "with result",
			runName:    filepath.Join(tmp, "with-result"),
			options:    []report.EndOption{report.WithResult(report.ResultFailed)},
			wantErr:    false,
			wantResult: report.ResultFailed,
		},
		{
			name:       "with reason",
			runName:    filepath.Join(tmp, "with-reason"),
			options:    []report.EndOption{report.WithReason(report.ReasonRunError)},
			wantErr:    false,
			wantResult: report.ResultSucceeded,
			wantReason: func() *report.Reason { r := report.ReasonRunError; return &r }(),
		},
		{
			name:       "with cause",
			runName:    filepath.Join(tmp, "with-cause"),
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
				run := newRun(t, tt.runName)
				r.AddRun(run)
			}

			err := r.EndRun(tt.runName, tt.options...)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)

				run, err := r.GetRun(tt.runName)
				require.NoError(t, err)

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

	tmp := t.TempDir()

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
				{filepath.Join(tmp, "single-successful-run"), report.ResultSucceeded},
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
				{filepath.Join(tmp, "successful-run"), report.ResultSucceeded},
				{filepath.Join(tmp, "failed-run"), report.ResultFailed},
				{filepath.Join(tmp, "early-exit-run"), report.ResultEarlyExit},
				{filepath.Join(tmp, "excluded-run"), report.ResultExcluded},
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
				run := newRun(t, result.name)
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

	tmp := t.TempDir()

	tests := []struct {
		name     string
		setup    func(*report.Report)
		expected []string
	}{
		{
			name: "single successful run",
			setup: func(r *report.Report) {
				run := newRun(t, filepath.Join(tmp, "successful-run"))
				r.AddRun(run)
				r.EndRun(run.Name)
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
				successRun := newRun(t, filepath.Join(tmp, "success-run"))
				r.AddRun(successRun)
				r.EndRun(successRun.Name)

				// Add failed run with reason
				failedRun := newRun(t, filepath.Join(tmp, "failed-run"))
				r.AddRun(failedRun)
				r.EndRun(failedRun.Name, report.WithResult(report.ResultFailed), report.WithReason(report.ReasonRunError))

				// Add excluded run with cause
				excludedRun := newRun(t, filepath.Join(tmp, "excluded-run"))
				r.AddRun(excludedRun)
				r.EndRun(excludedRun.Name, report.WithResult(report.ResultExcluded), report.WithCauseRetryBlock("test-block"))

				// Add early exit run with both reason and cause
				earlyExitRun := newRun(t, filepath.Join(tmp, "early-exit-run"))
				r.AddRun(earlyExitRun)
				r.EndRun(earlyExitRun.Name,
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

	tmp := t.TempDir()

	tests := []struct {
		name     string
		setup    func(*report.Report)
		expected string
	}{
		{
			name: "single successful run",
			setup: func(r *report.Report) {
				run := newRun(t, filepath.Join(tmp, "successful-run"))
				r.AddRun(run)
				r.EndRun(run.Name)
			},
			expected: `
❯❯ Run Summary
   Units: 1
   Duration: x
   Succeeded: 1
`,
		},
		{
			name: "complex mixed results",
			setup: func(r *report.Report) {
				// Add successful runs
				firstSuccessfulRun := newRun(t, filepath.Join(tmp, "first-successful-run"))
				r.AddRun(firstSuccessfulRun)
				r.EndRun(firstSuccessfulRun.Name)

				secondSuccessfulRun := newRun(t, filepath.Join(tmp, "second-successful-run"))
				r.AddRun(secondSuccessfulRun)
				r.EndRun(secondSuccessfulRun.Name)

				// Add failed runs
				firstFailedRun := newRun(t, filepath.Join(tmp, "first-failed-run"))
				r.AddRun(firstFailedRun)
				r.EndRun(firstFailedRun.Name, report.WithResult(report.ResultFailed))

				secondFailedRun := newRun(t, filepath.Join(tmp, "second-failed-run"))
				r.AddRun(secondFailedRun)
				r.EndRun(secondFailedRun.Name, report.WithResult(report.ResultFailed))

				// Add excluded runs
				firstExcludedRun := newRun(t, filepath.Join(tmp, "first-excluded-run"))
				r.AddRun(firstExcludedRun)
				r.EndRun(firstExcludedRun.Name, report.WithResult(report.ResultExcluded))

				secondExcludedRun := newRun(t, filepath.Join(tmp, "second-excluded-run"))
				r.AddRun(secondExcludedRun)
				r.EndRun(secondExcludedRun.Name, report.WithResult(report.ResultExcluded))

				// Add early exit runs
				firstEarlyExitRun := newRun(t, filepath.Join(tmp, "first-early-exit-run"))
				r.AddRun(firstEarlyExitRun)
				r.EndRun(firstEarlyExitRun.Name, report.WithResult(report.ResultEarlyExit))

				secondEarlyExitRun := newRun(t, filepath.Join(tmp, "second-early-exit-run"))
				r.AddRun(secondEarlyExitRun)
				r.EndRun(secondEarlyExitRun.Name, report.WithResult(report.ResultEarlyExit))
			},
			expected: `
❯❯ Run Summary
   Units: 8
   Duration: x
   Succeeded: 2
   Failed: 2
   Early Exits: 2
   Excluded: 2
`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			r := report.NewReport().WithDisableColor()
			tt.setup(r)

			var buf bytes.Buffer
			err := r.WriteSummary(&buf)
			require.NoError(t, err)

			output := buf.String()

			// Replace the duration with x
			re := regexp.MustCompile(`Duration: .*`)
			output = re.ReplaceAllString(output, "Duration: x")

			expected := strings.TrimSpace(tt.expected)
			assert.Equal(t, expected, strings.TrimSpace(output))
		})
	}
}

// newRun creates a new run, and asserts that it doesn't error.
func newRun(t *testing.T, name string) *report.Run {
	t.Helper()

	run, err := report.NewRun(name)
	require.NoError(t, err)

	return run
}
