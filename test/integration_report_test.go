package test_test

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"os"
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

func TestTerragruntReportExperiment(t *testing.T) {
	t.Parallel()

	// Set up test environment
	helpers.CleanupTerraformFolder(t, testFixtureReportPath)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureReportPath)
	rootPath := util.JoinPath(tmpEnvPath, testFixtureReportPath)

	// Run terragrunt with report experiment enabled
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	err := helpers.RunTerragruntCommand(t, "terragrunt run --all apply --experiment report --non-interactive --working-dir "+rootPath, &stdout, &stderr)
	require.NoError(t, err)

	// Verify the report output contains expected information
	stdoutStr := stdout.String()

	// Replace the duration line with a fixed duration
	re := regexp.MustCompile(`Duration:(\s+)(.*)`)
	stdoutStr = re.ReplaceAllString(stdoutStr, "Duration:${1}x")

	// Trim stdout to only the run summary.
	// We do this by only returning the last 8 lines (seven lines of the summary, footer gap).
	// We add one extra to avoid an off-by-one in slice math.
	lines := strings.Split(stdoutStr, "\n")
	stdoutStr = strings.Join(lines[len(lines)-9:], "\n")

	assert.Equal(t, strings.TrimSpace(`
❯❯ Run Summary
   Duration:     x
   Units:        13
   Succeeded:    4
   Failed:       3
   Early Exits:  4
   Excluded:     2
`), strings.TrimSpace(stdoutStr))
}

func TestTerragruntReportExperimentDisableSummary(t *testing.T) {
	t.Parallel()

	// Set up test environment
	helpers.CleanupTerraformFolder(t, testFixtureReportPath)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureReportPath)
	rootPath := util.JoinPath(tmpEnvPath, testFixtureReportPath)

	// Run terragrunt with report experiment enabled and summary disabled
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	err := helpers.RunTerragruntCommand(t, "terragrunt run --all apply --experiment report --non-interactive --working-dir "+rootPath+" --summary-disable", &stdout, &stderr)
	require.NoError(t, err)

	// Verify the report output does not contain the summary
	stdoutStr := stdout.String()
	assert.NotContains(t, stdoutStr, "Run Summary")
}

func TestTerragruntReportExperimentSaveToFile(t *testing.T) {
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
		tc := tc // capture range variable
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// Set up test environment
			helpers.CleanupTerraformFolder(t, testFixtureReportPath)
			tmpEnvPath := helpers.CopyEnvironment(t, testFixtureReportPath)
			rootPath := util.JoinPath(tmpEnvPath, testFixtureReportPath)

			// Run terragrunt with report experiment enabled and save to file
			var stdout bytes.Buffer
			var stderr bytes.Buffer
			reportFile := "report." + tc.format
			cmd := fmt.Sprintf(
				"terragrunt run --all apply --experiment report --non-interactive --working-dir %s --queue-exclude-dir %s --report-file %s",
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

func TestTerragruntReportExperimentSaveToFileWithFormat(t *testing.T) {
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
			cmd := "terragrunt run --all apply --experiment report --non-interactive --working-dir " + rootPath
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
			var stdout bytes.Buffer
			var stderr bytes.Buffer
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
				var jsonContent []map[string]interface{}
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
				var schema map[string]interface{}
				err = json.Unmarshal(schemaContent, &schema)
				require.NoError(t, err)

				// Verify basic schema structure
				assert.Equal(t, "array", schema["type"])
				assert.Equal(t, "Array of Terragrunt runs", schema["description"])
				assert.Equal(t, "Terragrunt Run Report Schema", schema["title"])

				// Verify items schema
				items, ok := schema["items"].(map[string]interface{})
				require.True(t, ok)

				// Verify required fields
				required, ok := items["required"].([]interface{})
				require.True(t, ok)
				assert.Contains(t, required, "Name")
				assert.Contains(t, required, "Started")
				assert.Contains(t, required, "Ended")
				assert.Contains(t, required, "Result")

				// Verify properties
				properties, ok := items["properties"].(map[string]interface{})
				require.True(t, ok)

				// Verify field types
				assert.Equal(t, "string", properties["Name"].(map[string]interface{})["type"])
				assert.Equal(t, "string", properties["Result"].(map[string]interface{})["type"])
				assert.Equal(t, "string", properties["Started"].(map[string]interface{})["type"])
				assert.Equal(t, "string", properties["Ended"].(map[string]interface{})["type"])
			}
		})
	}
}

func TestTerragruntReportExperimentWithUnitTiming(t *testing.T) {
	t.Parallel()

	// Set up test environment
	helpers.CleanupTerraformFolder(t, testFixtureReportPath)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureReportPath)
	rootPath := util.JoinPath(tmpEnvPath, testFixtureReportPath)

	// Run terragrunt with report experiment enabled and unit timing enabled
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	err := helpers.RunTerragruntCommand(t, "terragrunt run --all apply --experiment report --non-interactive --working-dir "+rootPath+" --summary-unit-timing", &stdout, &stderr)
	require.NoError(t, err)

	// Verify the report output contains expected information
	stdoutStr := stdout.String()

	// Replace the duration lines with fixed durations
	re := regexp.MustCompile(`Duration:(\s+)(.*)`)
	stdoutStr = re.ReplaceAllString(stdoutStr, "Duration:${1}x")

	// Replace unit timing durations with x
	re = regexp.MustCompile(`([ ]{6})([^\s]+:)(\s+)(.*)`)
	stdoutStr = re.ReplaceAllString(stdoutStr, "${1}${2}${3}x")

	// Trim stdout to only the run summary
	lines := strings.Split(stdoutStr, "\n")

	postLogLines := lines[len(lines)-22:]

	unitLogLines := postLogLines[2:15]

	// Sort the duration lines alphabetically so that they show up consistently.
	sort.Slice(unitLogLines, func(i, j int) bool {
		// Extract the unit name from the line
		unitNameI := strings.TrimSpace(re.ReplaceAllString(unitLogLines[i], "${2}"))
		unitNameJ := strings.TrimSpace(re.ReplaceAllString(unitLogLines[j], "${2}"))

		// Compare the unit names
		return unitNameI < unitNameJ
	})

	updatedLines := append(postLogLines[:2], unitLogLines...)
	updatedLines = append(updatedLines, postLogLines[15:]...)

	stdoutStr = strings.Join(updatedLines, "\n")

	assert.Equal(t, strings.TrimSpace(`
❯❯ Run Summary
   Duration:     x
      chain-a:            x
      chain-b:            x
      chain-c:            x
      error-ignore:       x
      first-early-exit:   x
      first-exclude:      x
      first-failure:      x
      first-success:      x
      retry-success:      x
      second-early-exit:  x
      second-exclude:     x
      second-failure:     x
      second-success:     x
   Units:        13
   Succeeded:    4
   Failed:       3
   Early Exits:  4
   Excluded:     2
`), strings.TrimSpace(stdoutStr))
}
