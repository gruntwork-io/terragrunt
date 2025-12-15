package test_test

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/gruntwork-io/terragrunt/test/helpers"
	"github.com/gruntwork-io/terragrunt/util"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	testFixtureReportPath = "fixtures/report"
)

func TestTerragruntReport(t *testing.T) {
	t.Parallel()

	// Set up test environment
	helpers.CleanupTerraformFolder(t, testFixtureReportPath)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureReportPath)
	rootPath := util.JoinPath(tmpEnvPath, testFixtureReportPath)

	// Run terragrunt with report experiment enabled
	var (
		stdout bytes.Buffer
		stderr bytes.Buffer
	)

	err := helpers.RunTerragruntCommand(t, "terragrunt run --all apply --non-interactive --working-dir "+rootPath, &stdout, &stderr)
	require.NoError(t, err)

	// Verify the report output contains expected information
	stdoutStr := stdout.String()

	// Replace the timing information with fixed values
	re := regexp.MustCompile(`❯❯ Run Summary\s+\d+\s+units\s+\S+`)
	stdoutStr = re.ReplaceAllString(stdoutStr, "❯❯ Run Summary  13 units  x")

	// Trim stdout to only the run summary.
	// Find the summary section
	lines := strings.Split(stdoutStr, "\n")

	// Find the "Run Summary" line
	summaryStartIdx := -1

	for i, line := range lines {
		if strings.Contains(line, "Run Summary") {
			summaryStartIdx = i
			break
		}
	}

	require.NotEqual(t, -1, summaryStartIdx, "Could not find 'Run Summary' line")

	// Extract the summary section
	summaryLines := lines[summaryStartIdx:]
	stdoutStr = strings.Join(summaryLines, "\n")

	assert.Equal(t, strings.TrimSpace(`
❯❯ Run Summary  13 units  x
   ────────────────────────────
   Succeeded    4
   Failed       3
   Early Exits  4
   Excluded     2
`), strings.TrimSpace(stdoutStr))
}

func TestTerragruntReportDisableSummary(t *testing.T) {
	t.Parallel()

	// Set up test environment
	helpers.CleanupTerraformFolder(t, testFixtureReportPath)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureReportPath)
	rootPath := util.JoinPath(tmpEnvPath, testFixtureReportPath)

	// Run terragrunt with report experiment enabled and summary disabled
	var (
		stdout bytes.Buffer
		stderr bytes.Buffer
	)

	err := helpers.RunTerragruntCommand(t, "terragrunt run --all apply --non-interactive --working-dir "+rootPath+" --summary-disable", &stdout, &stderr)
	require.NoError(t, err)

	// Verify the report output does not contain the summary
	stdoutStr := stdout.String()
	assert.NotContains(t, stdoutStr, "Run Summary")
}

func TestTerragruntReportSaveToFile(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name   string
		format string
	}{
		{
			name:   "CSV format",
			format: "csv",
		},
		{
			name:   "JSON format",
			format: "json",
		},
	}

	expectedHeader := []string{"Name", "Started", "Ended", "Result", "Reason", "Cause"}

	expectedRecords := []map[string]string{
		{"Name": "chain-a", "Result": "failed", "Reason": "run error", "Cause": ""},
		{"Name": "chain-b", "Result": "early exit", "Reason": "ancestor error", "Cause": "chain-a"},
		{"Name": "chain-c", "Result": "early exit", "Reason": "ancestor error", "Cause": "chain-b"},
		{"Name": "error-ignore", "Result": "succeeded", "Reason": "error ignored", "Cause": "ignore_everything"},
		{"Name": "first-early-exit", "Result": "early exit", "Reason": "ancestor error", "Cause": "first-failure"},
		{"Name": "first-exclude", "Result": "excluded", "Reason": "exclude block", "Cause": ""},
		{"Name": "first-failure", "Result": "failed", "Reason": "run error", "Cause": ".*Failed to execute.*"},
		{"Name": "first-success", "Result": "succeeded", "Reason": "", "Cause": ""},
		{"Name": "retry-success", "Result": "succeeded", "Reason": "retry succeeded", "Cause": ""},
		{"Name": "second-early-exit", "Result": "early exit", "Reason": "ancestor error", "Cause": "second-failure"},
		{"Name": "second-exclude", "Result": "excluded", "Reason": "--queue-exclude-dir", "Cause": ""},
		{"Name": "second-failure", "Result": "failed", "Reason": "run error", "Cause": ".*Failed to execute.*"},
		{"Name": "second-success", "Result": "succeeded", "Reason": "", "Cause": ""},
	}

	validResults := map[string]bool{
		"succeeded":  true,
		"failed":     true,
		"early exit": true,
		"excluded":   true,
	}

	for _, tc := range testCases {
		// capture range variable
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// Set up test environment
			helpers.CleanupTerraformFolder(t, testFixtureReportPath)
			tmpEnvPath := helpers.CopyEnvironment(t, testFixtureReportPath)
			rootPath := util.JoinPath(tmpEnvPath, testFixtureReportPath)

			// Run terragrunt with report experiment enabled and save to file
			var (
				stdout bytes.Buffer
				stderr bytes.Buffer
			)

			reportFile := "report." + tc.format
			cmd := fmt.Sprintf(
				"terragrunt run --all apply --non-interactive --working-dir %s --queue-exclude-dir %s --report-file %s",
				rootPath,
				util.JoinPath(rootPath, "second-exclude"),
				reportFile)
			err := helpers.RunTerragruntCommand(t, cmd, &stdout, &stderr)
			require.NoError(t, err)

			// Verify the report file exists
			reportFilePath := util.JoinPath(rootPath, reportFile)
			assert.FileExists(t, reportFilePath)

			// Read and parse the file based on format
			var records []map[string]string

			if tc.format == "csv" {
				file, err := os.Open(reportFilePath)
				require.NoError(t, err)

				defer file.Close()

				reader := csv.NewReader(file)
				csvRecords, err := reader.ReadAll()
				require.NoError(t, err)

				// Verify header
				assert.Equal(t, expectedHeader, csvRecords[0])

				// Convert CSV records to map format
				for _, record := range csvRecords[1:] {
					recordMap := make(map[string]string)
					for i, value := range record {
						recordMap[expectedHeader[i]] = value
					}

					records = append(records, recordMap)
				}
			} else {
				// JSON format
				content, err := os.ReadFile(reportFilePath)
				require.NoError(t, err)

				err = json.Unmarshal(content, &records)
				require.NoError(t, err)
			}

			// Verify we have the expected number of records
			require.Len(t, records, len(expectedRecords))

			// Sort records by name for consistent comparison
			sort.Slice(records, func(i, j int) bool {
				return records[i]["Name"] < records[j]["Name"]
			})

			// Verify each record
			for i, record := range records {
				_, err := time.Parse(time.RFC3339, record["Started"])
				require.NoError(t, err, "Started timestamp in record %d is not in RFC3339 format", i+1)

				_, err = time.Parse(time.RFC3339, record["Ended"])
				require.NoError(t, err, "Ended timestamp in record %d is not in RFC3339 format", i+1)

				// Verify Result is one of the expected values
				assert.True(t, validResults[record["Result"]], "Invalid result value in record %d: %s", i+1, record["Result"])

				// Create a new map with only the fields we want to compare
				compareRecord := map[string]string{
					"Name":   record["Name"],
					"Result": record["Result"],
					"Reason": record["Reason"],
					"Cause":  record["Cause"],
				}

				// Check that the cause is the error message
				if record["Reason"] == "run error" {
					assert.Regexp(t, expectedRecords[i]["Cause"], record["Cause"])

					compareRecord["Cause"] = ""
					expectedRecords[i]["Cause"] = ""
				}

				// Verify the record matches the expected record
				assert.Equal(t, expectedRecords[i], compareRecord)
			}
		})
	}
}

func TestTerragruntReportSaveToFileWithFormat(t *testing.T) {
	t.Parallel()

	// Set up test environment
	helpers.CleanupTerraformFolder(t, testFixtureReportPath)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureReportPath)
	rootPath := util.JoinPath(tmpEnvPath, testFixtureReportPath)

	testCases := []struct {
		name           string
		reportFile     string
		reportFormat   string
		expectedFormat string
		schemaFile     string
	}{
		{
			name:           "default format with no extension",
			reportFile:     "report",
			reportFormat:   "",
			expectedFormat: "csv",
		},
		{
			name:           "csv format from extension",
			reportFile:     "report.csv",
			reportFormat:   "",
			expectedFormat: "csv",
		},
		{
			name:           "json format from extension",
			reportFile:     "report.json",
			reportFormat:   "",
			expectedFormat: "json",
		},
		{
			name:           "explicit csv format overrides extension",
			reportFile:     "report.json",
			reportFormat:   "csv",
			expectedFormat: "csv",
		},
		{
			name:           "explicit json format overrides extension",
			reportFile:     "report.csv",
			reportFormat:   "json",
			expectedFormat: "json",
		},
		{
			name:           "generate schema file",
			reportFile:     "report.json",
			reportFormat:   "json",
			expectedFormat: "json",
			schemaFile:     "schema.json",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// Build command with appropriate flags
			cmd := "terragrunt run --all apply --non-interactive --working-dir " + rootPath
			if tc.reportFile != "" {
				cmd += " --report-file " + tc.reportFile
			}

			if tc.reportFormat != "" {
				cmd += " --report-format " + tc.reportFormat
			}

			if tc.schemaFile != "" {
				cmd += " --report-schema-file " + tc.schemaFile
			}

			// Run terragrunt command
			var (
				stdout bytes.Buffer
				stderr bytes.Buffer
			)

			err := helpers.RunTerragruntCommand(t, cmd, &stdout, &stderr)
			require.NoError(t, err)

			// Verify the report file exists
			reportFile := util.JoinPath(rootPath, tc.reportFile)
			assert.FileExists(t, reportFile)

			// Read the file content
			content, err := os.ReadFile(reportFile)
			require.NoError(t, err)

			// Verify the format based on content
			switch tc.expectedFormat {
			case "csv":
				// For CSV, verify it starts with the expected header
				assert.True(t, strings.HasPrefix(string(content), "Name,Started,Ended,Result,Reason,Cause"))
			case "json":
				// For JSON, verify it's valid JSON and has the expected structure
				var jsonContent []map[string]any

				err := json.Unmarshal(content, &jsonContent)

				require.NoError(t, err)

				require.NotEmpty(t, jsonContent)

				// Verify the first record has the expected fields
				firstRecord := jsonContent[0]
				assert.Contains(t, firstRecord, "Name")
				assert.Contains(t, firstRecord, "Started")
				assert.Contains(t, firstRecord, "Ended")
				assert.Contains(t, firstRecord, "Result")
			}

			// If schema file is specified, verify it exists and is valid JSON
			if tc.schemaFile != "" {
				schemaFilePath := util.JoinPath(rootPath, tc.schemaFile)
				assert.FileExists(t, schemaFilePath)

				// Read and verify schema file content
				schemaContent, err := os.ReadFile(schemaFilePath)
				require.NoError(t, err)

				// Verify it's valid JSON
				var schema map[string]any

				err = json.Unmarshal(schemaContent, &schema)
				require.NoError(t, err)

				// Verify basic schema structure
				assert.Equal(t, "array", schema["type"])
				assert.Equal(t, "Array of Terragrunt runs", schema["description"])
				assert.Equal(t, "Terragrunt Run Report Schema", schema["title"])

				// Verify items schema
				items, ok := schema["items"].(map[string]any)
				require.True(t, ok)

				// Verify required fields
				required, ok := items["required"].([]any)
				require.True(t, ok)
				assert.Contains(t, required, "Name")
				assert.Contains(t, required, "Started")
				assert.Contains(t, required, "Ended")
				assert.Contains(t, required, "Result")

				// Verify properties
				properties, ok := items["properties"].(map[string]any)
				require.True(t, ok)

				// Verify field types
				assert.Equal(t, "string", properties["Name"].(map[string]any)["type"])
				assert.Equal(t, "string", properties["Result"].(map[string]any)["type"])
				assert.Equal(t, "string", properties["Started"].(map[string]any)["type"])
				assert.Equal(t, "string", properties["Ended"].(map[string]any)["type"])
			}
		})
	}
}

func TestTerragruntReportWithUnitTiming(t *testing.T) {
	t.Parallel()

	// Set up test environment
	helpers.CleanupTerraformFolder(t, testFixtureReportPath)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureReportPath)
	rootPath := util.JoinPath(tmpEnvPath, testFixtureReportPath)

	// Run terragrunt with report experiment enabled and unit timing enabled
	var (
		stdout bytes.Buffer
		stderr bytes.Buffer
	)

	err := helpers.RunTerragruntCommand(t, "terragrunt run --all apply --non-interactive --working-dir "+rootPath+" --summary-per-unit", &stdout, &stderr)
	require.NoError(t, err)

	// Verify the report output contains expected information
	stdoutStr := stdout.String()

	// Replace the timing information with fixed values
	re := regexp.MustCompile(`❯❯ Run Summary\s+\d+\s+units\s+\S+`)
	stdoutStr = re.ReplaceAllString(stdoutStr, "❯❯ Run Summary  13 units  x")

	// Replace unit timing durations with x (including minutes, seconds, milliseconds, microseconds, nanoseconds)
	re = regexp.MustCompile(`(?m)\d+(\.\d+)?(m|s|ms|µs|μs|ns)$`)
	stdoutStr = re.ReplaceAllString(stdoutStr, "x")

	// Find and extract the run summary section
	lines := strings.Split(stdoutStr, "\n")

	// Find the "Run Summary" line
	summaryStartIdx := -1

	for i, line := range lines {
		if strings.Contains(line, "Run Summary") {
			summaryStartIdx = i
			break
		}
	}

	require.NotEqual(t, -1, summaryStartIdx, "Could not find 'Run Summary' line")

	// Find the end of the summary (last non-empty line after summary start)
	summaryEndIdx := len(lines) - 1
	for i := summaryEndIdx; i > summaryStartIdx; i-- {
		if strings.TrimSpace(lines[i]) != "" {
			summaryEndIdx = i
			break
		}
	}

	// Extract the summary section
	summaryLines := lines[summaryStartIdx : summaryEndIdx+1]
	stdoutStr = strings.Join(summaryLines, "\n")

	// Sort lines within each category to make the test deterministic
	// We're not testing the sorting functionality here, just the per-unit timing display
	stdoutStr = sortLinesWithinCategories(stdoutStr)

	// The expected format has units grouped by status with timing (sorted alphabetically within categories)
	expectedOutput := `
❯❯ Run Summary  13 units  x
   ────────────────────────────
   Succeeded (4)
      error-ignore ...... x
      first-success ..... x
      retry-success ..... x
      second-success .... x
   Failed (3)
      chain-a ........... x
      first-failure ..... x
      second-failure .... x
   Early Exits (4)
      chain-b ........... x
      chain-c ........... x
      first-early-exit .. x
      second-early-exit . x
   Excluded (2)
      first-exclude ..... x
      second-exclude .... x`

	assert.Equal(t, strings.TrimSpace(expectedOutput), strings.TrimSpace(stdoutStr))
}

func TestReportWithExternalDependenciesExcluded(t *testing.T) {
	t.Parallel()

	cleanupTerraformFolder(t, testFixtureExternalDependency)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureExternalDependency)
	rootPath := filepath.Join(tmpEnvPath, testFixtureExternalDependency)

	dep := t.TempDir()

	f, err := os.Create(filepath.Join(dep, "terragrunt.hcl"))
	require.NoError(t, err)
	f.Close()

	f, err = os.Create(filepath.Join(dep, "main.tf"))
	require.NoError(t, err)
	f.Close()

	reportDir := t.TempDir()
	reportFile := filepath.Join(reportDir, "report.json")

	stdout, stderr, err := helpers.RunTerragruntCommandWithOutput(
		t,
		fmt.Sprintf(
			"terragrunt run --all plan --queue-exclude-external --log-level debug --feature dep=%s --working-dir %s --report-file %s",
			dep,
			rootPath,
			reportFile,
		),
	)

	// The command should succeed without "run not found in report" errors
	require.NoError(t, err)

	// Verify that no "run not found in report" errors appear in stderr
	assert.NotContains(t, stderr, "run not found in report")
	assert.Contains(t, stdout, "Run Summary")

	// External dependencies should be logged as excluded during discovery
	assert.Contains(t, stderr, "Excluded external dependency",
		"External dependencies should be logged as excluded")

	// Verify that the report file exists
	assert.FileExists(t, reportFile)

	// Read the report file
	reportContent, err := os.ReadFile(reportFile)
	require.NoError(t, err)

	// Verify that the report file contains the expected content
	var report []map[string]any

	err = json.Unmarshal(reportContent, &report)
	require.NoError(t, err)

	// Verify that the report file contains the expected content
	assert.Len(t, report, 2)

	depPath := dep
	if resolved, err := filepath.EvalSymlinks(dep); err == nil {
		depPath = resolved
	}

	expected := []struct {
		name   string
		result string
		reason string
	}{
		// The first run is always going to be the external dependency,
		// as it has an instant runtime.
		{name: depPath, result: "excluded", reason: "--queue-exclude-external"},
		{name: "external-dependency", result: "succeeded"},
	}

	for i, r := range report {
		assert.Equal(t, expected[i].name, r["Name"])
		assert.Equal(t, expected[i].result, r["Result"])

		if expected[i].reason != "" {
			assert.Equal(t, expected[i].reason, r["Reason"])
		}
	}
}

// lineType represents the type of line we're processing
type lineType int

const (
	categoryHeaderLine lineType = iota
	unitLine
	otherLine
)

// getLineType determines what type of line we're dealing with
func getLineType(line string, inCategory bool) lineType {
	trimmed := strings.TrimSpace(line)

	// Check if this is a category header line (ends with a count in parentheses)
	if strings.Contains(line, "(") && strings.Contains(line, ")") &&
		(strings.Contains(line, "Succeeded") || strings.Contains(line, "Failed") ||
			strings.Contains(line, "Early Exits") || strings.Contains(line, "Excluded")) {
		return categoryHeaderLine
	}

	// Check if this is a unit line within a category
	if inCategory && strings.HasPrefix(line, "      ") && trimmed != "" {
		return unitLine
	}

	return otherLine
}

// sortLinesWithinCategories sorts the unit lines within each category alphabetically
// to make the test deterministic regardless of actual execution timing
func sortLinesWithinCategories(input string) string {
	lines := strings.Split(input, "\n")

	var (
		result               []string
		currentCategoryLines []string
	)

	inCategory := false

	for _, line := range lines {
		switch getLineType(line, inCategory) {
		case categoryHeaderLine:
			// If we were in a category, sort and add those lines first
			if inCategory && len(currentCategoryLines) > 0 {
				sort.Strings(currentCategoryLines)
				result = append(result, currentCategoryLines...)
				currentCategoryLines = nil
			}
			// Add the category header
			result = append(result, line)
			inCategory = true
		case unitLine:
			// This is a unit line within a category
			currentCategoryLines = append(currentCategoryLines, line)
		case otherLine:
			// If we were in a category, sort and add those lines first
			if inCategory && len(currentCategoryLines) > 0 {
				sort.Strings(currentCategoryLines)
				result = append(result, currentCategoryLines...)
				currentCategoryLines = nil
				inCategory = false
			}
			// Add non-category lines as-is
			result = append(result, line)
		}
	}

	// Handle any remaining category lines
	if inCategory && len(currentCategoryLines) > 0 {
		sort.Strings(currentCategoryLines)
		result = append(result, currentCategoryLines...)
	}

	return strings.Join(result, "\n")
}
