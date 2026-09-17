//go:build aws || awsgcp

package test_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/test/helpers"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testFixtureAwsAuthProviderRoleReuse = "fixtures/auth-provider-cmd/role-session-reuse"

// TestAWSAuthProviderRoleIsAssumedWithCallerIdentity checks real STS gets the caller's own credentials.
func TestAWSAuthProviderRoleIsAssumedWithCallerIdentity(t *testing.T) {
	t.Parallel()

	assumeRole := os.Getenv("AWS_TEST_S3_ASSUME_ROLE")
	require.NotEmpty(t, assumeRole, "AWS_TEST_S3_ASSUME_ROLE environment variable not set")

	helpers.CleanupTerraformFolder(t, testFixtureAwsAuthProviderRoleReuse)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureAwsAuthProviderRoleReuse)
	rootPath := filepath.Join(tmpEnvPath, testFixtureAwsAuthProviderRoleReuse)
	authCmd := filepath.Join(rootPath, "auth-provider.sh")

	// The script exits non-zero without this.
	v := venv.OSVenv()
	v.Env["TG_TEST_ROLE_ARN"] = assumeRole

	helpers.ValidateAuthProviderScript(t, v, rootPath, authCmd)

	stdout, stderr, err := helpers.RunTerragruntCommandWithOutputWithVenv(t, v, fmt.Sprintf(
		"terragrunt run --all plan --non-interactive --log-level debug --working-dir %s --auth-provider-cmd %s",
		rootPath, authCmd,
	))

	require.NoError(t, err, "stdout:\n%s\nstderr:\n%s", stdout, stderr)
	assert.NotContains(t, stderr, "AccessDenied",
		"the role was re-assumed signed by its own session; AWS rejected the chained assume-role request")
	assert.NotContains(t, stderr, "Failed to assume role",
		"an auth-provider role assumption failed during the run")

	assertAuthProviderAssumedRole(t, stderr, assumeRole)
}

// TestAWSAuthProviderRoleWithJSONOutDir covers the --json-out-dir path, which runs each unit twice.
func TestAWSAuthProviderRoleWithJSONOutDir(t *testing.T) {
	t.Parallel()

	assumeRole := os.Getenv("AWS_TEST_S3_ASSUME_ROLE")
	require.NotEmpty(t, assumeRole, "AWS_TEST_S3_ASSUME_ROLE environment variable not set")

	helpers.CleanupTerraformFolder(t, testFixtureAwsAuthProviderRoleReuse)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureAwsAuthProviderRoleReuse)
	rootPath := filepath.Join(tmpEnvPath, testFixtureAwsAuthProviderRoleReuse)
	authCmd := filepath.Join(rootPath, "auth-provider.sh")

	// The script exits non-zero without this.
	v := venv.OSVenv()
	v.Env["TG_TEST_ROLE_ARN"] = assumeRole

	helpers.ValidateAuthProviderScript(t, v, rootPath, authCmd)

	outDir := filepath.Join(t.TempDir(), "plans")
	jsonOutDir := filepath.Join(t.TempDir(), "json")

	stdout, stderr, err := helpers.RunTerragruntCommandWithOutputWithVenv(t, v, fmt.Sprintf(
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

// assertAuthProviderAssumedRole fails if the role was never used, or was assumed more than once.
func assertAuthProviderAssumedRole(t *testing.T, stderr, assumeRole string) {
	t.Helper()

	// Tests share one process, so a sibling may have cached the session already.
	assumed := strings.Count(stderr, "Assuming IAM role "+assumeRole)
	reused := strings.Count(stderr, "Using cached credentials for IAM role "+assumeRole)

	assert.Positivef(t, assumed+reused,
		"no credentials were exercised for %s, so this run proved nothing", assumeRole)
	assert.LessOrEqualf(t, assumed, 1,
		"the role was assumed %d times; the assumed session must be reused, not re-assumed", assumed)
}
