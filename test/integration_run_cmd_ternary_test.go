package test_test

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gruntwork-io/terragrunt/test/helpers"
)

const (
	testFixtureRunCmdTernaryTrue  = "fixtures/run-cmd-ternary/true-condition"
	testFixtureRunCmdTernaryFalse = "fixtures/run-cmd-ternary/false-condition"
)

// TestRunCmdTernaryOnlyRunsSelectedBranch verifies that when a ternary expression is
// used in locals, only the branch matching the condition executes run_cmd.
// Regression test for https://github.com/gruntwork-io/terragrunt/issues/1448
func TestRunCmdTernaryOnlyRunsSelectedBranch(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		fixturePath string
		wantCmd     string
		forbidCmd   string
	}{
		{
			name:        "true condition runs only true branch",
			fixturePath: testFixtureRunCmdTernaryTrue,
			wantCmd:     "Running command: echo branch_true",
			forbidCmd:   "Running command: echo branch_false",
		},
		{
			name:        "false condition runs only false branch",
			fixturePath: testFixtureRunCmdTernaryFalse,
			wantCmd:     "Running command: echo branch_false",
			forbidCmd:   "Running command: echo branch_true",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			helpers.CleanupTerraformFolder(t, tt.fixturePath)

			tmpEnvPath := helpers.CopyEnvironment(t, tt.fixturePath)
			rootPath := filepath.Join(tmpEnvPath, tt.fixturePath)

			cmd := "terragrunt plan --non-interactive --log-level debug --working-dir " + rootPath

			stdout, stderr, err := helpers.RunTerragruntCommandWithOutput(t, cmd)
			require.NoError(t, err)

			combined := stdout + stderr

			assert.Contains(t, combined, tt.wantCmd)
			assert.NotContains(t, combined, tt.forbidCmd)
		})
	}
}
