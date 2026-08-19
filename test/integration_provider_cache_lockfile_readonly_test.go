//go:build tf

package test_test

import (
	"fmt"
	"path/filepath"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/tf"
	"github.com/gruntwork-io/terragrunt/test/helpers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	testFixtureProviderCacheLockfileReadonly = "fixtures/provider-cache/lockfile-readonly"

	// lockfileReadonlyLogMessage is a distinctive fragment of the message Terragrunt logs
	// when it leaves the lock file untouched because `-lockfile=readonly` is set.
	lockfileReadonlyLogMessage = "so Terragrunt will not generate or update"
)

// TestTFTerragruntProviderCacheLockfileReadonly is a regression test for GitHub issue
// #6349. With the provider cache enabled, Terragrunt used to generate
// `.terraform.lock.hcl` before running `init`, which satisfied OpenTofu/Terraform's
// dependency check and silently defeated `-lockfile=readonly`. The cache must now
// leave the lock file alone when that flag is set, whether it arrives on the command
// line or through `TF_CLI_ARGS_init`, so init fails exactly as it does without the cache.
//
//nolint:paralleltest,tparallel // the env-var subtest relies on t.Setenv.
func TestTFTerragruntProviderCacheLockfileReadonly(t *testing.T) {
	lockfileName := ".terraform.lock.hcl"

	t.Run("cache writes lock file without readonly", func(t *testing.T) {
		appPath := copyProviderCacheLockfileReadonlyFixture(t)
		providerCacheDir := helpers.TmpDirWOSymlinks(t)

		helpers.RunTerragrunt(t, fmt.Sprintf(
			"terragrunt run --provider-cache --provider-cache-dir %s --non-interactive --working-dir %s -- init",
			providerCacheDir,
			appPath,
		))

		assert.FileExists(t, filepath.Join(appPath, lockfileName),
			"provider cache should generate the lock file when -lockfile=readonly is not set")
	})

	t.Run("readonly via flag is enforced", func(t *testing.T) {
		appPath := copyProviderCacheLockfileReadonlyFixture(t)
		providerCacheDir := helpers.TmpDirWOSymlinks(t)

		_, stderr, err := helpers.RunTerragruntCommandWithOutput(t, fmt.Sprintf(
			"terragrunt run --provider-cache --provider-cache-dir %s "+
				"--non-interactive --working-dir %s -- init -lockfile=readonly",
			providerCacheDir, appPath,
		))

		require.Error(t, err, "init must fail because the lock file is missing and read-only")
		assert.NoFileExists(t, filepath.Join(appPath, lockfileName),
			"provider cache must not generate the lock file when -lockfile=readonly is set")
		assert.NotContains(t, stderr, lockfileReadonlyLogMessage,
			"skipping the lock file is the requested behaviour, so it must not be logged above debug level")
	})

	t.Run("readonly is logged at debug level", func(t *testing.T) {
		appPath := copyProviderCacheLockfileReadonlyFixture(t)
		providerCacheDir := helpers.TmpDirWOSymlinks(t)

		_, stderr, err := helpers.RunTerragruntCommandWithOutput(t, fmt.Sprintf(
			"terragrunt run --provider-cache --provider-cache-dir %s "+
				"--non-interactive --log-level debug --working-dir %s -- init -lockfile=readonly",
			providerCacheDir, appPath,
		))

		require.Error(t, err, "init must fail because the lock file is missing and read-only")
		assert.Contains(t, stderr, lockfileReadonlyLogMessage,
			"debug logging must still explain why the lock file was left untouched")
	})

	t.Run("readonly via TF_CLI_ARGS_init is enforced", func(t *testing.T) {
		t.Setenv(
			tf.EnvNameTFCLIArgsInit,
			fmt.Sprintf("%s=%s", tf.FlagNameLockfile, tf.LockfileModeReadonly),
		)

		appPath := copyProviderCacheLockfileReadonlyFixture(t)
		providerCacheDir := helpers.TmpDirWOSymlinks(t)

		_, _, err := helpers.RunTerragruntCommandWithOutput(t, fmt.Sprintf(
			"terragrunt run --provider-cache --provider-cache-dir %s --non-interactive --working-dir %s -- init",
			providerCacheDir,
			appPath,
		))

		require.Error(t, err, "init must fail because the lock file is missing and read-only")
		assert.NoFileExists(
			t,
			filepath.Join(appPath, lockfileName),
			"provider cache must not generate the lock file when TF_CLI_ARGS_init requests -lockfile=readonly",
		)
	})
}

func copyProviderCacheLockfileReadonlyFixture(t *testing.T) string {
	t.Helper()

	helpers.CleanupTerraformFolder(t, testFixtureProviderCacheLockfileReadonly)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureProviderCacheLockfileReadonly)

	return filepath.Join(tmpEnvPath, testFixtureProviderCacheLockfileReadonly)
}
