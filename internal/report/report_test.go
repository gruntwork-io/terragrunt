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

func TestEnsureRun(t *testing.T) {
	t.Parallel()

	tmp := t.TempDir()

	tests := []struct {
		expectedErrIs error
		setupFunc     func(*report.Report) *report.Run
		name          string
		runName       string
		existingRun   bool
		expectError   bool
	}{
		{
			name:        "creates new run when run does not exist",
			runName:     filepath.Join(tmp, "new-run"),
			existingRun: false,
			expectError: false,
		},
		{
			name:        "returns existing run when it exists",
			runName:     filepath.Join(tmp, "existing-run"),
			existingRun: true,
			expectError: false,
			setupFunc: func(r *report.Report) *report.Run {
				run := newRun(t, filepath.Join(tmp, "existing-run"))
				err := r.AddRun(run)
				require.NoError(t, err)
				return run
			},
		},
		{
			name:          "returns error for invalid path",
			runName:       "relative-path",
			existingRun:   false,
			expectError:   true,
			expectedErrIs: report.ErrPathMustBeAbsolute,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			r := report.NewReport()

			var existingRun *report.Run

			if tt.setupFunc != nil {
				existingRun = tt.setupFunc(r)
			}

			run, err := r.EnsureRun(tt.runName)

			if tt.expectError {
				require.Error(t, err)
				assert.Nil(t, run)

				if tt.expectedErrIs != nil {
					require.ErrorIs(t, err, tt.expectedErrIs)
				}
			} else {
				require.NoError(t, err)
				require.NotNil(t, run)
				assert.Equal(t, tt.runName, run.Path)
				assert.False(t, run.Started.IsZero())

				if tt.existingRun {
					// Should return the same instance as the existing run
					assert.Equal(t, existingRun.Started, run.Started)
				}

				// Verify the run was added to the report
				retrievedRun, err := r.GetRun(tt.runName)
				require.NoError(t, err)
				assert.Equal(t, run, retrievedRun)

				// Verify that calling EnsureRun again returns the same run
				secondRun, err := r.EnsureRun(tt.runName)
				require.NoError(t, err)
				assert.Equal(t, run, secondRun)
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

func TestEndRunAlreadyEnded(t *testing.T) {
	t.Parallel()

	tmp := t.TempDir()

	tests := []struct {
		name           string
		initialResult  report.Result
		expectedResult report.Result
		secondResult   report.Result
		initialOptions []report.EndOption
		secondOptions  []report.EndOption
	}{
		{
			name:           "already ended with early exit is not overwritten",
			initialResult:  report.ResultEarlyExit,
			secondResult:   report.ResultSucceeded,
			expectedResult: report.ResultEarlyExit,
		},
		{
			name:           "already ended with excluded is not overwritten",
			initialResult:  report.ResultExcluded,
			secondResult:   report.ResultSucceeded,
			expectedResult: report.ResultExcluded,
		},
		{
			name:           "already ended with retry succeeded is overwritten",
			initialResult:  report.ResultSucceeded,
			initialOptions: []report.EndOption{report.WithReason(report.ReasonRetrySucceeded)},
			secondResult:   report.ResultSucceeded,
			expectedResult: report.ResultSucceeded,
		},
		{
			name:           "already ended with retry failed is overwritten",
			initialResult:  report.ResultSucceeded,
			initialOptions: []report.EndOption{report.WithReason(report.ReasonRetrySucceeded)},
			secondResult:   report.ResultFailed,
			expectedResult: report.ResultFailed,
		},
		{
			name:           "already ended with error ignored is overwritten",
			initialResult:  report.ResultSucceeded,
			initialOptions: []report.EndOption{report.WithReason(report.ReasonErrorIgnored)},
			secondResult:   report.ResultSucceeded,
			expectedResult: report.ResultSucceeded,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// Create a new report and run for each test case
			r := report.NewReport()
			runName := filepath.Join(tmp, tt.name)
			run := newRun(t, runName)
			r.AddRun(run)

			// Set up initial options with the initial result
			initialOptions := append(tt.initialOptions, report.WithResult(tt.initialResult))

			// End the run with the initial state
			err := r.EndRun(runName, initialOptions...)
			require.NoError(t, err)

			// Set up second options with the second result
			secondOptions := append(tt.secondOptions, report.WithResult(tt.secondResult))

			// Then try to end it again with a different state
			err = r.EndRun(runName, secondOptions...)
			require.NoError(t, err)

			// Verify that the result is the expected one
			endedRun, err := r.GetRun(runName)
			require.NoError(t, err)
			assert.Equal(t, tt.expectedResult, endedRun.Result)
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
			assert.Equal(t, tt.wantTotalUnits, summary.TotalUnits())
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
			var expectedJSON, actualJSON []map[string]any

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

const ExpectedSchema = `{
  "items": {
    "$schema": "https://json-schema.org/draft/2020-12/schema",
    "$id": "https://terragrunt.gruntwork.io/schemas/run/report/v1/schema.json",
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
        "type": "string",
        "enum": [
          "retry succeeded",
          "error ignored",
          "run error",
          "--queue-exclude-dir",
          "exclude block",
          "ancestor error"
        ]
      },
      "Cause": {
        "type": "string"
      },
      "Name": {
        "type": "string"
      },
      "Result": {
        "type": "string",
        "enum": [
          "succeeded",
          "failed",
          "early exit",
          "excluded"
        ]
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
`

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
	assert.JSONEq(t, ExpectedSchema, buf.String())

	// Parse the schema
	var schema map[string]any

	err = json.Unmarshal(buf.Bytes(), &schema)
	require.NoError(t, err)

	// Verify the schema structure
	assert.Equal(t, "array", schema["type"])
	assert.Equal(t, "Array of Terragrunt runs", schema["description"])
	assert.Equal(t, "Terragrunt Run Report Schema", schema["title"])

	// Verify the items schema
	items, ok := schema["items"].(map[string]any)
	require.True(t, ok)

	// Verify the properties
	properties, ok := items["properties"].(map[string]any)
	require.True(t, ok)

	// Verify required fields
	required, ok := items["required"].([]any)
	require.True(t, ok)
	assert.Contains(t, required, "Name")
	assert.Contains(t, required, "Started")
	assert.Contains(t, required, "Ended")
	assert.Contains(t, required, "Result")

	// Verify field types
	assert.Equal(t, "string", properties["Name"].(map[string]any)["type"])
	assert.Equal(t, "string", properties["Result"].(map[string]any)["type"])
	assert.Equal(t, "string", properties["Started"].(map[string]any)["type"])
	assert.Equal(t, "string", properties["Ended"].(map[string]any)["type"])

	// Verify optional fields
	reason, ok := properties["Reason"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "string", reason["type"])

	cause, ok := properties["Cause"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "string", cause["type"])
}

func TestExpectedSchemaIsInDocs(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		file string
	}{
		{
			name: "starlight",
			file: filepath.Join("..", "..", "docs-starlight", "public", "schemas", "run", "report", "v1", "schema.json"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			schema, err := os.ReadFile(tt.file)
			require.NoError(t, err)

			assert.JSONEq(t, ExpectedSchema, string(schema))
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
				r.EndRun(run.Path)
			},
			expected: `
❯❯ Run Summary  1 units  x
   ────────────────────────────
   Succeeded    1
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
❯❯ Run Summary  8 units  x
   ────────────────────────────
   Succeeded    2
   Failed       2
   Early Exits  2
   Excluded     2
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

			// Replace the duration in the header with x
			// Pattern matches: "❯❯ Run Summary  8 units  42µs" -> "❯❯ Run Summary  8 units  x"
			re := regexp.MustCompile(`(❯❯ Run Summary\s+\d+\s+units\s+)[^\n]+`)
			output = re.ReplaceAllString(output, "${1}x")

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

func TestWriteUnitLevelSummary(t *testing.T) {
	t.Parallel()

	tmp := t.TempDir()

	tests := []struct {
		name     string
		setup    func(*report.Report)
		expected string
	}{
		{
			name: "empty runs",
			setup: func(r *report.Report) {
				// No runs added
			},
			expected: ``,
		},
		{
			name: "single run",
			setup: func(r *report.Report) {
				run := newRun(t, filepath.Join(tmp, "single-run"))
				r.AddRun(run)
				r.EndRun(run.Path)
			},
			expected: `
❯❯ Run Summary  1 units  x
   ────────────────────────────
   Succeeded (1)
      single-run ....... x
`,
		},
		{
			name: "multiple runs sorted by duration",
			setup: func(r *report.Report) {
				// Add runs with different durations
				longRun := newRun(t, filepath.Join(tmp, "long-run"))
				r.AddRun(longRun)

				mediumRun := newRun(t, filepath.Join(tmp, "medium-run"))
				r.AddRun(mediumRun)

				shortRun := newRun(t, filepath.Join(tmp, "short-run"))
				r.AddRun(shortRun)

				r.EndRun(shortRun.Path)
				r.EndRun(mediumRun.Path)
				r.EndRun(longRun.Path)
			},
			expected: `
❯❯ Run Summary  3 units  x
   ────────────────────────────
   Succeeded (3)
      long-run ......... x
      medium-run ....... x
      short-run ........ x
`,
		},
		{
			name: "mixed results grouped by category",
			setup: func(r *report.Report) {
				// Add runs with different results
				successRun1 := newRun(t, filepath.Join(tmp, "success-1"))
				successRun2 := newRun(t, filepath.Join(tmp, "success-2"))
				failRun := newRun(t, filepath.Join(tmp, "fail-run"))
				excludedRun := newRun(t, filepath.Join(tmp, "excluded-run"))

				r.AddRun(successRun1)
				r.AddRun(successRun2)
				r.AddRun(failRun)
				r.AddRun(excludedRun)

				r.EndRun(successRun1.Path)
				r.EndRun(successRun2.Path)
				r.EndRun(failRun.Path, report.WithResult(report.ResultFailed))
				r.EndRun(excludedRun.Path, report.WithResult(report.ResultExcluded))
			},
			expected: `
❯❯ Run Summary  4 units  x
   ────────────────────────────
   Succeeded (2)
      success-1 ........ x
      success-2 ........ x
   Failed (1)
      fail-run ......... x
   Excluded (1)
      excluded-run ..... x
`,
		},
		{
			name: "very short unit names",
			setup: func(r *report.Report) {
				// Add runs with very short names
				a := newRun(t, filepath.Join(tmp, "a"))
				b := newRun(t, filepath.Join(tmp, "b"))
				c := newRun(t, filepath.Join(tmp, "c"))

				r.AddRun(a)
				r.AddRun(b)
				r.AddRun(c)

				r.EndRun(a.Path)
				r.EndRun(b.Path)
				r.EndRun(c.Path)
			},
			expected: `
❯❯ Run Summary  3 units  x
   ────────────────────────────
   Succeeded (3)
      a ................ x
      b ................ x
      c ................ x
`,
		},
		{
			name: "very long unit names",
			setup: func(r *report.Report) {
				// Add runs with very long names
				longName1 := newRun(t, filepath.Join(tmp, "this-is-a-very-long-name-1"))
				longName2 := newRun(t, filepath.Join(tmp, "this-is-a-very-long-name-2"))
				longName3 := newRun(t, filepath.Join(tmp, "this-is-a-very-long-name-3"))

				r.AddRun(longName1)
				r.AddRun(longName2)
				r.AddRun(longName3)

				r.EndRun(longName1.Path)
				r.EndRun(longName2.Path)
				r.EndRun(longName3.Path)
			},
			expected: `
❯❯ Run Summary  3 units           x
   ───────────────────────────────────
   Succeeded (3)
      this-is-a-very-long-name-1  x
      this-is-a-very-long-name-2  x
      this-is-a-very-long-name-3  x
`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			r := report.NewReport().
				WithDisableColor().
				WithShowUnitLevelSummary().
				WithWorkingDir(tmp)

			tt.setup(r)

			var buf bytes.Buffer

			err := r.WriteSummary(&buf)
			require.NoError(t, err)

			// Replace the duration with x since we can't control the actual duration in tests
			output := buf.String()

			// Replace the header duration with x
			re := regexp.MustCompile(`❯❯ Run Summary  (\d+) units(\s+)(\d+.+)`)
			output = re.ReplaceAllString(output, "❯❯ Run Summary  ${1} units${2}x")

			// Replace all the unit level summaries
			re = regexp.MustCompile(`([ ]{6})([^ ]+)( )([^ ]*)( )(\d+.+)`)
			output = re.ReplaceAllString(output, "${1}${2}${3}${4}${5}x")

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
