//go:build aws || awsgcp

package test_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gruntwork-io/terragrunt/test/helpers"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testFixtureAwsAuthProviderRoleReuse = "fixtures/auth-provider-cmd/role-session-reuse"

// Pins against real STS that an auth-provider role is assumed with the caller's identity, not its own session.
//
// A run that re-assumes signed by the session it just minted gets AccessDenied from real AWS unless the
// role trusts itself, which is the failure this guards. The in-memory test in internal/runner/run/creds
// observes the signing identity directly; only this one proves what AWS actually does with it.
func TestAwsAuthProviderRoleIsAssumedWithCallerIdentity(t *testing.T) {
	// t.Parallel() cannot be used together with t.Setenv()
	assumeRole := os.Getenv("AWS_TEST_S3_ASSUME_ROLE")
	if len(assumeRole) == 0 {
		t.Error("AWS_TEST_S3_ASSUME_ROLE environment variable not set")
		return
	}

	helpers.CleanupTerraformFolder(t, testFixtureAwsAuthProviderRoleReuse)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureAwsAuthProviderRoleReuse)
	rootPath := filepath.Join(tmpEnvPath, testFixtureAwsAuthProviderRoleReuse)
	authCmd := filepath.Join(rootPath, "auth-provider.sh")

	// Set before validating: the script requires this variable and exits non-zero without it.
	t.Setenv("TG_TEST_ROLE_ARN", assumeRole)

	helpers.ValidateAuthProviderScript(t, rootPath, authCmd)

	stdout, stderr, err := helpers.RunTerragruntCommandWithOutput(t, fmt.Sprintf(
		"terragrunt run --all plan --non-interactive --log-level debug --working-dir %s --auth-provider-cmd %s",
		rootPath, authCmd,
	))

	require.NoError(t, err, "stdout:\n%s\nstderr:\n%s", stdout, stderr)
	assert.NotContains(t, stderr, "AccessDenied",
		"the role was re-assumed signed by its own session; AWS rejected the chained assume-role request")
	assert.NotContains(t, stderr, "Failed to assume role",
		"an auth-provider role assumption failed during the run")

	// Without this the test passes when the auth provider never ran and no role was assumed at all.
	assertAuthProviderAssumedRole(t, stderr, assumeRole)
}

// Pins the same invariant on the --json-out-dir path, which runs each unit a second time.
func TestAwsAuthProviderRoleWithJSONOutDir(t *testing.T) {
	// t.Parallel() cannot be used together with t.Setenv()
	assumeRole := os.Getenv("AWS_TEST_S3_ASSUME_ROLE")
	if len(assumeRole) == 0 {
		t.Error("AWS_TEST_S3_ASSUME_ROLE environment variable not set")
		return
	}

	helpers.CleanupTerraformFolder(t, testFixtureAwsAuthProviderRoleReuse)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureAwsAuthProviderRoleReuse)
	rootPath := filepath.Join(tmpEnvPath, testFixtureAwsAuthProviderRoleReuse)
	authCmd := filepath.Join(rootPath, "auth-provider.sh")

	// Set before validating: the script requires this variable and exits non-zero without it.
	t.Setenv("TG_TEST_ROLE_ARN", assumeRole)

	helpers.ValidateAuthProviderScript(t, rootPath, authCmd)

	outDir := filepath.Join(t.TempDir(), "plans")
	jsonOutDir := filepath.Join(t.TempDir(), "json")

	stdout, stderr, err := helpers.RunTerragruntCommandWithOutput(t, fmt.Sprintf(
		"terragrunt run --all plan --non-interactive --log-level debug --working-dir %s --auth-provider-cmd %s --out-dir %s --json-out-dir %s",
		rootPath, authCmd, outDir, jsonOutDir,
	))

	require.NoError(t, err, "stdout:\n%s\nstderr:\n%s", stdout, stderr)
	assert.NotContains(t, stderr, "AccessDenied",
		"the JSON export re-assumed the role signed by its own session")
	assert.NotContains(t, stderr, "Failed to assume role",
		"an auth-provider role assumption failed during the run")

	assertAuthProviderAssumedRole(t, stderr, assumeRole)
}

// Requires exactly one assumption: zero means the run proved nothing, more than one means reuse failed.
func assertAuthProviderAssumedRole(t *testing.T, stderr, assumeRole string) {
	t.Helper()

	assumed := strings.Count(stderr, "Assuming IAM role "+assumeRole)
	assert.Equalf(t, 1, assumed,
		"expected the role to be assumed exactly once for the whole run, got %d; "+
			"zero means no credentials were exercised, more than one means the session was not reused",
		assumed)
}
