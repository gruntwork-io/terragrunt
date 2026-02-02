package test_test

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gruntwork-io/terragrunt/test/helpers"
)

const (
	testFixtureRunCmdIncludeOutput = "fixtures/regressions/run-cmd-include-output"
	runCmdOutputMarker             = "RUN_CMD_OUTPUT_MARKER_12345"
)

// TestRunCmdOutputFromIncludedFileInStack verifies that run_cmd output from included
// files (like root.hcl) is visible when running terragrunt commands on a stack.
// This is a regression test for issue #5400.
func TestRunCmdOutputFromIncludedFileInStack(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureRunCmdIncludeOutput)

	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureRunCmdIncludeOutput)
	rootPath := filepath.Join(tmpEnvPath, testFixtureRunCmdIncludeOutput)

	cmd := "terragrunt run --all plan --non-interactive --working-dir " + rootPath

	stdout, stderr, err := helpers.RunTerragruntCommandWithOutput(t, cmd)
	require.NoError(t, err)

	combinedOutput := strings.Join([]string{stdout, stderr}, "\n")

	// The run_cmd output marker should be visible in the combined output
	// Before the fix, this output was suppressed because the command ran during
	// discovery phase with io.Discard writers, and the cached result was reused
	// during the execution phase without replaying the output.
	assert.Contains(t, combinedOutput, runCmdOutputMarker,
		"run_cmd output from included file should be visible in stack run output")
}
