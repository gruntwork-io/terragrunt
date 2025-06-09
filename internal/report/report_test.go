package report_test

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/gruntwork-io/terragrunt/internal/report"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/xeipuuv/gojsonschema"
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
	assert.Equal(t, path, run.Path)
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

	r := report.NewReport()

	err := r.AddRun(newRun(t, path))
	require.NoError(t, err)
	assert.Len(t, r.Runs, 1)

	err = r.AddRun(newRun(t, path))
	require.Error(t, err)
	assert.ErrorIs(t, err, report.ErrRunAlreadyExists)
}

func TestGetRun(t *testing.T) {
	t.Parallel()

	tmp := t.TempDir()

	r := report.NewReport()
	run := newRun(t, filepath.Join(tmp, "test-run"))
	r.AddRun(run)

	tests := []struct {
		expectedErr error
		name        string
		runName     string
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
				assert.Equal(t, tt.runName, run.Path)
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
				require.Error(t, err)
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

	tests := []struct {
		name     string
		setup    func(dir string, r *report.Report)
		expected [][]string
	}{
		{
			name: "single successful run",
			setup: func(dir string, r *report.Report) {
				run := newRun(t, filepath.Join(dir, "successful-run"))
				r.AddRun(run)
				r.EndRun(run.Path)
			},
			expected: [][]string{
				{"Name", "Started", "Ended", "Result", "Reason", "Cause"},
				{"successful-run", "", "", "succeeded", "", ""},
			},
		},
		{
			name: "complex mixed results",
			setup: func(dir string, r *report.Report) {
				// Add successful run
				successRun := newRun(t, filepath.Join(dir, "success-run"))
				r.AddRun(successRun)
				r.EndRun(successRun.Path)

				// Add failed run with reason
				failedRun := newRun(t, filepath.Join(dir, "failed-run"))
				r.AddRun(failedRun)
				r.EndRun(failedRun.Path, report.WithResult(report.ResultFailed), report.WithReason(report.ReasonRunError))

				// Add excluded run with cause
				excludedRun := newRun(t, filepath.Join(dir, "excluded-run"))
				r.AddRun(excludedRun)
				r.EndRun(excludedRun.Path, report.WithResult(report.ResultExcluded), report.WithCauseRetryBlock("test-block"))

				// Add early exit run with both reason and cause
				earlyExitRun := newRun(t, filepath.Join(dir, "early-exit-run"))
				r.AddRun(earlyExitRun)
				r.EndRun(earlyExitRun.Path,
					report.WithResult(report.ResultEarlyExit),
					report.WithReason(report.ReasonRunError),
					report.WithCauseRetryBlock("another-block"),
				)
			},
			expected: [][]string{
				{"Name", "Started", "Ended", "Result", "Reason", "Cause"},
				{"success-run", "", "", "succeeded", "", ""},
				{"failed-run", "", "", "failed", "run error", ""},
				{"excluded-run", "", "", "excluded", "", "test-block"},
				{"early-exit-run", "", "", "early exit", "run error", "another-block"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			tmp := t.TempDir()

			// Create a temporary file for the CSV
			csvFile := filepath.Join(tmp, "report.csv")
			file, err := os.Create(csvFile)
			require.NoError(t, err)
			defer file.Close()

			// Setup and write the report
			r := report.NewReport().WithWorkingDir(tmp)
			tt.setup(tmp, r)

			err = r.WriteCSV(file)
			require.NoError(t, err)

			// Close the file before reading
			file.Close()

			// Read the CSV file
			file, err = os.Open(csvFile)
			require.NoError(t, err)
			defer file.Close()

			reader := csv.NewReader(file)
			records, err := reader.ReadAll()
			require.NoError(t, err)

			// Verify the number of records
			require.Len(t, records, len(tt.expected))

			// Verify each record
			for i, record := range records {
				expected := tt.expected[i]
				require.Len(t, record, len(expected), "Record %d has wrong number of fields", i)

				// For the header row, verify exact match
				if i == 0 {
					assert.Equal(t, expected, record)
					continue
				}

				// For data rows, verify fields individually
				assert.Equal(t, expected[0], record[0], "Name mismatch in record %d", i)
				// Skip timestamp verification for Started and Ended fields
				assert.Equal(t, expected[3], record[3], "Result mismatch in record %d", i)
				assert.Equal(t, expected[4], record[4], "Reason mismatch in record %d", i)
				assert.Equal(t, expected[5], record[5], "Cause mismatch in record %d", i)

				// Verify that timestamps are in RFC3339 format
				if record[1] != "" {
					_, err := time.Parse(time.RFC3339, record[1])
					require.NoError(t, err, "Started timestamp in record %d is not in RFC3339 format", i)
				}
				if record[2] != "" {
					_, err := time.Parse(time.RFC3339, record[2])
					require.NoError(t, err, "Ended timestamp in record %d is not in RFC3339 format", i)
				}
			}
		})
	}
}

func TestWriteJSON(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		setup    func(dir string, r *report.Report)
		expected string
	}{
		{
			name: "single successful run",
			setup: func(dir string, r *report.Report) {
				run := newRun(t, filepath.Join(dir, "successful-run"))
				r.AddRun(run)
				r.EndRun(run.Path)
			},
			expected: `[
  {
    "Name": "successful-run",
    "Started": "2024-03-21T10:00:00Z",
    "Ended": "2024-03-21T10:01:00Z",
    "Result": "succeeded"
  }
]`,
		},
		{
			name: "complex mixed results",
			setup: func(dir string, r *report.Report) {
				// Add successful run
				successRun := newRun(t, filepath.Join(dir, "success-run"))
				r.AddRun(successRun)
				r.EndRun(successRun.Path)

				// Add failed run with reason
				failedRun := newRun(t, filepath.Join(dir, "failed-run"))
				r.AddRun(failedRun)
				r.EndRun(
					failedRun.Path,
					report.WithResult(report.ResultFailed),
					report.WithReason(report.ReasonRunError),
				)

				// Add excluded run with cause
				retriedRun := newRun(t, filepath.Join(dir, "retried-run"))
				r.AddRun(retriedRun)
				r.EndRun(
					retriedRun.Path,
					report.WithResult(report.ResultSucceeded),
					report.WithReason(report.ReasonRetrySucceeded),
				)

				// Add excluded run with cause
				excludedRun := newRun(t, filepath.Join(dir, "excluded-run"))
				r.AddRun(excludedRun)
				r.EndRun(
					excludedRun.Path,
					report.WithResult(report.ResultExcluded),
					report.WithReason(report.ReasonExcludeBlock),
					report.WithCauseExcludeBlock("test-block"),
				)
			},
			expected: `[
  {
    "Name": "success-run",
    "Started": "2024-03-21T10:00:00Z",
    "Ended": "2024-03-21T10:01:00Z",
    "Result": "succeeded"
  },
  {
    "Name": "failed-run",
    "Started": "2024-03-21T10:01:00Z",
    "Ended": "2024-03-21T10:02:00Z",
    "Result": "failed",
    "Reason": "run error"
  },
  {
    "Name": "retried-run",
    "Started": "2024-03-21T10:03:00Z",
    "Ended": "2024-03-21T10:04:00Z",
    "Result": "succeeded",
    "Reason": "retry succeeded"
  },
  {
    "Name": "excluded-run",
    "Started": "2024-03-21T10:02:00Z",
    "Ended": "2024-03-21T10:03:00Z",
    "Result": "excluded",
    "Reason": "exclude block",
    "Cause": "test-block"
  }
]`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			tmp := t.TempDir()

			// Create a temporary file for the JSON
			jsonFile := filepath.Join(tmp, "report.json")
			file, err := os.Create(jsonFile)
			require.NoError(t, err)
			defer file.Close()

			// Setup and write the report
			r := report.NewReport().WithWorkingDir(tmp)
			tt.setup(tmp, r)

			err = r.WriteJSON(file)
			require.NoError(t, err)

			// Close the file before reading
			file.Close()

			// Read the JSON file
			file, err = os.Open(jsonFile)
			require.NoError(t, err)
			defer file.Close()

			// Read the actual output
			actualBytes, err := os.ReadFile(jsonFile)
			require.NoError(t, err)

			// Parse both expected and actual JSON to compare them
			var expectedJSON, actualJSON []map[string]interface{}
			err = json.Unmarshal([]byte(tt.expected), &expectedJSON)
			require.NoError(t, err)
			err = json.Unmarshal(actualBytes, &actualJSON)
			require.NoError(t, err)

			// Verify the number of records
			require.Len(t, actualJSON, len(expectedJSON))

			// Verify each record
			for i, actualRecord := range actualJSON {
				expectedRecord := expectedJSON[i]

				// Verify name
				assert.Equal(t, expectedRecord["Name"], actualRecord["Name"], "Name mismatch in record %d", i)

				// Verify result
				assert.Equal(t, expectedRecord["Result"], actualRecord["Result"], "Result mismatch in record %d", i)

				// Verify reason if present
				if expectedReason, ok := expectedRecord["Reason"]; ok {
					assert.Equal(t, expectedReason, actualRecord["Reason"], "Reason mismatch in record %d", i)
				} else {
					assert.NotContains(t, actualRecord, "Reason", "Unexpected reason in record %d", i)
				}

				// Verify cause if present
				if expectedCause, ok := expectedRecord["Cause"]; ok {
					assert.Equal(t, expectedCause, actualRecord["Cause"], "Cause mismatch in record %d", i)
				} else {
					assert.NotContains(t, actualRecord, "Cause", "Unexpected cause in record %d", i)
				}

				// Verify timestamps are in RFC3339 format
				if started, ok := actualRecord["Started"].(string); ok {
					_, err := time.Parse(time.RFC3339, started)
					require.NoError(t, err, "Started timestamp in record %d is not in RFC3339 format", i)
				}
				if ended, ok := actualRecord["Ended"].(string); ok {
					_, err := time.Parse(time.RFC3339, ended)
					require.NoError(t, err, "Ended timestamp in record %d is not in RFC3339 format", i)
				}
			}
		})
	}
}

func TestWriteSchema(t *testing.T) {
	t.Parallel()

	// Create a buffer to write the schema to
	var buf bytes.Buffer

	// Create a new report
	r := report.NewReport()

	// Write the schema
	err := r.WriteSchema(&buf)
	require.NoError(t, err)

	// Assert the contents of the schema
	assert.Equal(t, `{
  "items": {
    "$schema": "https://json-schema.org/draft/2020-12/schema",
    "$id": "https://terragrunt.gruntwork.io/schemas/run/report/v1",
    "properties": {
      "Started": {
        "type": "string",
        "format": "date-time"
      },
      "Ended": {
        "type": "string",
        "format": "date-time"
      },
      "Reason": {
        "type": "string"
      },
      "Cause": {
        "type": "string"
      },
      "Name": {
        "type": "string"
      },
      "Result": {
        "type": "string"
      }
    },
    "additionalProperties": false,
    "type": "object",
    "required": [
      "Started",
      "Ended",
      "Name",
      "Result"
    ],
    "title": "Terragrunt Run Report Schema",
    "description": "Schema for Terragrunt run report"
  },
  "type": "array",
  "title": "Terragrunt Run Report Schema",
  "description": "Array of Terragrunt runs"
}
`, buf.String())

	// Parse the schema
	var schema map[string]interface{}
	err = json.Unmarshal(buf.Bytes(), &schema)
	require.NoError(t, err)

	// Verify the schema structure
	assert.Equal(t, "array", schema["type"])
	assert.Equal(t, "Array of Terragrunt runs", schema["description"])
	assert.Equal(t, "Terragrunt Run Report Schema", schema["title"])

	// Verify the items schema
	items, ok := schema["items"].(map[string]interface{})
	require.True(t, ok)

	// Verify the properties
	properties, ok := items["properties"].(map[string]interface{})
	require.True(t, ok)

	// Verify required fields
	required, ok := items["required"].([]interface{})
	require.True(t, ok)
	assert.Contains(t, required, "Name")
	assert.Contains(t, required, "Started")
	assert.Contains(t, required, "Ended")
	assert.Contains(t, required, "Result")

	// Verify field types
	assert.Equal(t, "string", properties["Name"].(map[string]interface{})["type"])
	assert.Equal(t, "string", properties["Result"].(map[string]interface{})["type"])
	assert.Equal(t, "string", properties["Started"].(map[string]interface{})["type"])
	assert.Equal(t, "string", properties["Ended"].(map[string]interface{})["type"])

	// Verify optional fields
	reason, ok := properties["Reason"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "string", reason["type"])

	cause, ok := properties["Cause"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "string", cause["type"])
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
				r.EndRun(run.Path)
			},
			expected: `
❯❯ Run Summary
   Duration:   x
   Units:      1
   Succeeded:  1
`,
		},
		{
			name: "complex mixed results",
			setup: func(r *report.Report) {
				// Add successful runs
				firstSuccessfulRun := newRun(t, filepath.Join(tmp, "first-successful-run"))
				r.AddRun(firstSuccessfulRun)
				r.EndRun(firstSuccessfulRun.Path)

				secondSuccessfulRun := newRun(t, filepath.Join(tmp, "second-successful-run"))
				r.AddRun(secondSuccessfulRun)
				r.EndRun(secondSuccessfulRun.Path)

				// Add failed runs
				firstFailedRun := newRun(t, filepath.Join(tmp, "first-failed-run"))
				r.AddRun(firstFailedRun)
				r.EndRun(firstFailedRun.Path, report.WithResult(report.ResultFailed))

				secondFailedRun := newRun(t, filepath.Join(tmp, "second-failed-run"))
				r.AddRun(secondFailedRun)
				r.EndRun(secondFailedRun.Path, report.WithResult(report.ResultFailed))

				// Add excluded runs
				firstExcludedRun := newRun(t, filepath.Join(tmp, "first-excluded-run"))
				r.AddRun(firstExcludedRun)
				r.EndRun(firstExcludedRun.Path, report.WithResult(report.ResultExcluded))

				secondExcludedRun := newRun(t, filepath.Join(tmp, "second-excluded-run"))
				r.AddRun(secondExcludedRun)
				r.EndRun(secondExcludedRun.Path, report.WithResult(report.ResultExcluded))

				// Add early exit runs
				firstEarlyExitRun := newRun(t, filepath.Join(tmp, "first-early-exit-run"))
				r.AddRun(firstEarlyExitRun)
				r.EndRun(firstEarlyExitRun.Path, report.WithResult(report.ResultEarlyExit))

				secondEarlyExitRun := newRun(t, filepath.Join(tmp, "second-early-exit-run"))
				r.AddRun(secondEarlyExitRun)
				r.EndRun(secondEarlyExitRun.Path, report.WithResult(report.ResultEarlyExit))
			},
			expected: `
❯❯ Run Summary
   Duration:     x
   Units:        8
   Succeeded:    2
   Failed:       2
   Early Exits:  2
   Excluded:     2
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
			re := regexp.MustCompile(`Duration:(\s+).*`)
			output = re.ReplaceAllString(output, "Duration:${1}x")

			expected := strings.TrimSpace(tt.expected)
			assert.Equal(t, expected, strings.TrimSpace(output))
		})
	}
}

func TestSchemaIsValid(t *testing.T) {
	t.Parallel()

	tmp := t.TempDir()

	// Create a new report with working directory
	r := report.NewReport().WithWorkingDir(tmp)

	// Add a simple run that succeeds
	simpleRun := newRun(t, filepath.Join(tmp, "simple-run"))
	r.AddRun(simpleRun)
	r.EndRun(simpleRun.Path,
		report.WithResult(report.ResultSucceeded),
	)

	// Add a complex run that tests all possible fields and states
	complexRun := newRun(t, filepath.Join(tmp, "complex-run"))
	r.AddRun(complexRun)
	r.EndRun(complexRun.Path,
		report.WithResult(report.ResultFailed),
		report.WithReason(report.ReasonRunError),
		report.WithCauseAncestorExit("some-error"),
	)

	// Create an excluded run with exclude block
	excludedRun := newRun(t, filepath.Join(tmp, "excluded-run"))
	r.AddRun(excludedRun)
	r.EndRun(excludedRun.Path,
		report.WithResult(report.ResultExcluded),
		report.WithReason(report.ReasonExcludeBlock),
		report.WithCauseExcludeBlock("test-block"),
	)

	// Create a retry run that succeeded
	retryRun := newRun(t, filepath.Join(tmp, "retry-run"))
	r.AddRun(retryRun)
	r.EndRun(retryRun.Path,
		report.WithResult(report.ResultSucceeded),
		report.WithReason(report.ReasonRetrySucceeded),
		report.WithCauseRetryBlock("retry-block"),
	)

	// Create an early exit run
	earlyExitRun := newRun(t, filepath.Join(tmp, "early-exit-run"))
	r.AddRun(earlyExitRun)
	r.EndRun(earlyExitRun.Path,
		report.WithResult(report.ResultEarlyExit),
		report.WithReason(report.ReasonAncestorError),
		report.WithCauseAncestorExit("parent-unit"),
	)

	// Create a run with ignored error
	ignoredRun := newRun(t, filepath.Join(tmp, "ignored-run"))
	r.AddRun(ignoredRun)
	r.EndRun(ignoredRun.Path,
		report.WithResult(report.ResultSucceeded),
		report.WithReason(report.ReasonErrorIgnored),
		report.WithCauseIgnoreBlock("ignore-block"),
	)

	// Write the report to a JSON file
	reportFile := filepath.Join(tmp, "report.json")
	file, err := os.Create(reportFile)
	require.NoError(t, err)
	defer file.Close()

	err = r.WriteJSON(file)
	require.NoError(t, err)
	file.Close()

	// Write the schema to a file
	schemaFile := filepath.Join(tmp, "schema.json")
	file, err = os.Create(schemaFile)
	require.NoError(t, err)
	defer file.Close()

	err = r.WriteSchema(file)
	require.NoError(t, err)
	file.Close()

	// Read the schema and report files
	schemaBytes, err := os.ReadFile(schemaFile)
	require.NoError(t, err)

	reportBytes, err := os.ReadFile(reportFile)
	require.NoError(t, err)

	// Create a new schema loader
	schemaLoader := gojsonschema.NewBytesLoader(schemaBytes)
	documentLoader := gojsonschema.NewBytesLoader(reportBytes)

	// Validate the report against the schema
	result, err := gojsonschema.Validate(schemaLoader, documentLoader)
	require.NoError(t, err)

	// Check if the validation was successful
	assert.True(t, result.Valid(), "JSON report does not match schema: %v", result.Errors())

	// Additional validation of the report content
	var reportData []report.JSONRun
	err = json.Unmarshal(reportBytes, &reportData)
	require.NoError(t, err)

	// Verify we have all the expected runs
	require.Len(t, reportData, 6)

	// Helper function to find a run by name
	findRun := func(name string) *report.JSONRun {
		for _, run := range reportData {
			if run.Name == name {
				return &run
			}
		}
		return nil
	}

	// Verify simple run
	simple := findRun("simple-run")
	require.NotNil(t, simple)
	assert.Equal(t, "succeeded", simple.Result)
	assert.Nil(t, simple.Reason)
	assert.Nil(t, simple.Cause)
	assert.False(t, simple.Started.IsZero())
	assert.False(t, simple.Ended.IsZero())

	// Verify complex run
	complex := findRun("complex-run")
	require.NotNil(t, complex)
	assert.Equal(t, "failed", complex.Result)
	assert.Equal(t, "run error", *complex.Reason)
	assert.Equal(t, "some-error", *complex.Cause)
	assert.False(t, complex.Started.IsZero())
	assert.False(t, complex.Ended.IsZero())

	// Verify excluded run
	excluded := findRun("excluded-run")
	require.NotNil(t, excluded)
	assert.Equal(t, "excluded", excluded.Result)
	assert.Equal(t, "exclude block", *excluded.Reason)
	assert.Equal(t, "test-block", *excluded.Cause)

	// Verify retry run
	retry := findRun("retry-run")
	require.NotNil(t, retry)
	assert.Equal(t, "succeeded", retry.Result)
	assert.Equal(t, "retry succeeded", *retry.Reason)
	assert.Equal(t, "retry-block", *retry.Cause)

	// Verify early exit run
	earlyExit := findRun("early-exit-run")
	require.NotNil(t, earlyExit)
	assert.Equal(t, "early exit", earlyExit.Result)
	assert.Equal(t, "ancestor error", *earlyExit.Reason)
	assert.Equal(t, "parent-unit", *earlyExit.Cause)

	// Verify ignored run
	ignored := findRun("ignored-run")
	require.NotNil(t, ignored)
	assert.Equal(t, "succeeded", ignored.Result)
	assert.Equal(t, "error ignored", *ignored.Reason)
	assert.Equal(t, "ignore-block", *ignored.Cause)
}

// newRun creates a new run, and asserts that it doesn't error.
func newRun(t *testing.T, name string) *report.Run {
	t.Helper()

	run, err := report.NewRun(name)
	require.NoError(t, err)

	return run
}
