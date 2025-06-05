package test_test

import (
	"bytes"
	"encoding/csv"
	"os"
	"regexp"
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

	// Set up test environment
	helpers.CleanupTerraformFolder(t, testFixtureReportPath)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureReportPath)
	rootPath := util.JoinPath(tmpEnvPath, testFixtureReportPath)

	// Run terragrunt with report experiment enabled and save to file
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	err := helpers.RunTerragruntCommand(t, "terragrunt run --all apply --experiment report --non-interactive --working-dir "+rootPath+" --report-file report.csv", &stdout, &stderr)
	require.NoError(t, err)

	// Verify the report file exists
	reportFile := util.JoinPath(rootPath, "report.csv")
	assert.FileExists(t, reportFile)

	// Read and parse the CSV file
	file, err := os.Open(reportFile)
	require.NoError(t, err)
	defer file.Close()

	reader := csv.NewReader(file)
	records, err := reader.ReadAll()
	require.NoError(t, err)

	// Verify we have at least the header and some data rows
	require.GreaterOrEqual(t, len(records), 2)

	// Verify header row
	expectedHeader := []string{"Name", "Started", "Ended", "Result", "Reason", "Cause"}
	assert.Equal(t, expectedHeader, records[0])

	// Verify data rows
	for i, record := range records[1:] {
		// Verify number of fields
		require.Equal(t, len(expectedHeader), len(record), "Record %d has wrong number of fields", i+1)

		// Verify timestamp formats if present
		if record[1] != "" {
			_, err := time.Parse(time.RFC3339, record[1])
			assert.NoError(t, err, "Started timestamp in record %d is not in RFC3339 format", i+1)
		}
		if record[2] != "" {
			_, err := time.Parse(time.RFC3339, record[2])
			assert.NoError(t, err, "Ended timestamp in record %d is not in RFC3339 format", i+1)
		}

		// Verify Result is one of the expected values
		validResults := map[string]bool{
			"succeeded":  true,
			"failed":     true,
			"early exit": true,
			"excluded":   true,
		}
		assert.True(t, validResults[record[3]], "Invalid result value in record %d: %s", i+1, record[3])
	}
}
