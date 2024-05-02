// common integration test functions
package test

import (
	"fmt"
	"testing"

	"github.com/gruntwork-io/terragrunt/util"
)

func testRunAllPlan(t *testing.T, args string) (string, string, string, error) {
	t.Helper()

	tmpEnvPath := copyEnvironment(t, TEST_FIXTURE_OUT_DIR)
	cleanupTerraformFolder(t, tmpEnvPath)
	testPath := util.JoinPath(tmpEnvPath, TEST_FIXTURE_OUT_DIR)

	// run plan with output directory
	stdout, stderr, err := runTerragruntCommandWithOutput(t, fmt.Sprintf("terraform run-all plan --terragrunt-non-interactive --terragrunt-log-level debug --terragrunt-working-dir %s %s", testPath, args))

	return tmpEnvPath, stdout, stderr, err
}
