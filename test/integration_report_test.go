package test_test

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/report"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/gruntwork-io/terragrunt/test/helpers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	testFixtureReportPath = "fixtures/report"
)

func TestTerragruntReportDisableSummary(t *testing.T) {
	t.Parallel()

	// Set up test environment
	helpers.CleanupTerraformFolder(t, testFixtureReportPath)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureReportPath)
	rootPath := filepath.Join(tmpEnvPath, testFixtureReportPath)

	stdout, _, err := helpers.RunTerragruntCommandWithOutput(
		t,
		"terragrunt run --all apply --non-interactive --working-dir "+
			rootPath+" --summary-disable",
	)
	require.Error(t, err)

	// Verify the report output does not contain the summary
	assert.NotContains(t, stdout, "Run Summary")
}

// TestTerragruntReportRunErrorCause verifies that a unit that fails before
// OpenTofu/Terraform runs still carries the failure text as its report cause.
//
// Only chain-b is put on the queue: its dependency chain-a is neither in the
// queue (so there is no failed ancestor to blame) nor applied (so resolving its
// outputs fails during config evaluation, before OpenTofu/Terraform runs).
func TestTerragruntReportRunErrorCause(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureReportPath)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureReportPath)
	rootPath := filepath.Join(tmpEnvPath, testFixtureReportPath)

	_, _, err := helpers.RunTerragruntCommandWithOutput(
		t,
		"terragrunt run --all --non-interactive --working-dir "+rootPath+
			" --queue-strict-include --queue-include-dir chain-b"+
			" --report-file "+helpers.ReportFile+" -- apply",
	)
	require.Error(t, err)

	reportFilePath := filepath.Join(rootPath, helpers.ReportFile)
	assert.FileExists(t, reportFilePath)

	runs, err := report.ParseJSONRunsFromFile(vfs.NewOSFS(), reportFilePath)
	require.NoError(t, err)

	run := runs.FindByName("chain-b")
	require.NotNil(t, run)
	assert.Equal(t, "failed", run.Result)
	require.NotNil(t, run.Reason)
	assert.Equal(t, "run error", *run.Reason)
	require.NotNil(t, run.Cause, "config evaluation failure must populate the run cause")
	assert.Contains(t, *run.Cause, "detected no outputs")
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

// createReportTestUnit creates a unit directory with terragrunt.hcl and main.tf files.
func createReportTestUnit(t *testing.T, dir, comment string) {
	t.Helper()

	err := os.MkdirAll(dir, 0755)
	require.NoError(t, err)

	err = os.WriteFile(filepath.Join(dir, "terragrunt.hcl"), []byte(comment), 0644)
	require.NoError(t, err)

	err = os.WriteFile(filepath.Join(dir, "main.tf"), []byte(`# Minimal terraform config`), 0644)
	require.NoError(t, err)
}
