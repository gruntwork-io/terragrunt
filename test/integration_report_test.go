package test_test

import (
	"bytes"
	"regexp"
	"strings"
	"testing"

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

	// The command should fail since we have failing units
	require.Error(t, err)

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
   Duration:    x
   Units:       8
   Succeeded:   2
   Failed:      2
   Early Exits: 2
   Excluded:    2
`), strings.TrimSpace(stdoutStr))
}
