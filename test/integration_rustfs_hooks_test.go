//go:build docker

package test_test

import (
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/gruntwork-io/terragrunt/internal/tf"
	"github.com/gruntwork-io/terragrunt/internal/vexec"
	"github.com/gruntwork-io/terragrunt/test/helpers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	testFixtureHooksInitOnceNoSourceWithBackendRustFS   = "fixtures/hooks/init-once/no-source-with-backend-rustfs"
	testFixtureHooksInitOnceWithSourceWithBackendRustFS = "fixtures/hooks/init-once/with-source-with-backend-rustfs"
	testFixtureHooksBeforeAfterAndErrorMergeRustFS      = "fixtures/hooks/before-after-and-error-merge-rustfs"
	testFixtureS3BackendDisableInitRustFS               = "fixtures/s3-backend-disable-init-rustfs"
)

// TestRustFSInitHookWithBackend pins that init and init-from-module hooks each
// run exactly once when a unit with an S3 backend is applied, both when the
// unit has no source and when it copies a module from one.
func TestRustFSInitHookWithBackend(t *testing.T) {
	t.Parallel()

	rustfsAddr := setupRustFS(t)

	testCases := []struct {
		name    string
		fixture string
	}{
		{
			name:    "no source",
			fixture: testFixtureHooksInitOnceNoSourceWithBackendRustFS,
		},
		{
			name:    "with source",
			fixture: testFixtureHooksInitOnceWithSourceWithBackendRustFS,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			s3BucketName := "terragrunt-test-bucket-" + strings.ToLower(helpers.UniqueID())

			tmpEnvPath := helpers.CopyEnvironment(t, "fixtures/hooks/init-once")
			rootPath := filepath.Join(tmpEnvPath, tc.fixture)
			configPath := filepath.Join(rootPath, "terragrunt.hcl")

			helpers.CopyAndFillMapPlaceholders(
				t,
				configPath,
				configPath,
				map[string]string{
					"__FILL_IN_BUCKET_NAME__": s3BucketName,
					"__FILL_IN_S3_ENDPOINT__": rustfsAddr,
				},
			)

			recorder := helpers.NewExecRecorder(vexec.NewOSExec())

			err := helpers.RunTerragruntCommandWithVenv(
				t,
				t.Context(),
				rustfsVenv().WithExec(recorder),
				"terragrunt apply -auto-approve --non-interactive --backend-bootstrap --working-dir "+rootPath,
			)
			require.NoError(t, err)

			commands := recorder.Commands()

			// With the always-cache behavior, a unit with no source is still
			// copied into the cache as source ".", so init-from-module hooks run.
			assert.Equal(
				t,
				1,
				countEchoCommands(commands, "AFTER_INIT_FROM_MODULE_ONLY_ONCE"),
				"init-from-module hook must run exactly once, got %v",
				commands,
			)
			assert.Equal(
				t,
				1,
				countEchoCommands(commands, "AFTER_INIT_ONLY_ONCE"),
				"init hook must run exactly once, got %v",
				commands,
			)
		})
	}
}

// TestRustFSBeforeAfterAndErrorMergeHook pins how before, after, and error
// hooks merge from an included parent into a child with an S3 backend: hooks
// with distinct names from both configs run, and a child hook replaces the
// parent hook of the same name.
func TestRustFSBeforeAfterAndErrorMergeHook(t *testing.T) {
	t.Parallel()

	rustfsAddr := setupRustFS(t)

	s3BucketName := "terragrunt-test-bucket-" + strings.ToLower(helpers.UniqueID())

	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureHooksBeforeAfterAndErrorMergeRustFS)
	rootPath := filepath.Join(tmpEnvPath, testFixtureHooksBeforeAfterAndErrorMergeRustFS)
	childPath := filepath.Join(rootPath, "qa", "my-app")
	rootConfigPath := filepath.Join(rootPath, "root.hcl")

	helpers.CopyAndFillMapPlaceholders(
		t,
		rootConfigPath,
		rootConfigPath,
		map[string]string{
			"__FILL_IN_BUCKET_NAME__": s3BucketName,
			"__FILL_IN_S3_ENDPOINT__": rustfsAddr,
		},
	)

	_, _, err := runTerragruntRustFS(
		t,
		"terragrunt apply -auto-approve --non-interactive --backend-bootstrap --working-dir "+childPath,
	)
	// The parent's produce_error_to_test_error_hook runs `exit`, which is a
	// shell builtin and not an executable, so the error hooks fire.
	require.ErrorIs(t, err, exec.ErrNotFound)

	cacheDir := helpers.FindCacheWorkingDir(t, childPath)
	require.NotEmpty(t, cacheDir, "Cache directory should exist")

	for _, name := range []string{
		"before.out",
		"before-child.out",
		"after.out",
		"after-parent.out",
		"error-hook-parent.out",
		"error-hook-child.out",
		"error-hook-merge-child.out",
	} {
		assert.FileExists(t, filepath.Join(cacheDir, name))
	}

	for _, name := range []string{
		"before-parent.out",
		"error-hook-merge-parent.out",
	} {
		assert.NoFileExists(t, filepath.Join(cacheDir, name), "child hook of the same name must replace the parent's")
	}
}

// TestRustFSDisableInitS3Backend pins that remote_state.disable_init stops
// Terragrunt from bootstrapping the S3 bucket, even with --backend-bootstrap,
// while tofu still initializes the backend from -backend-config args rather
// than running with -backend=false.
func TestRustFSDisableInitS3Backend(t *testing.T) {
	t.Parallel()

	rustfsAddr := setupRustFS(t)

	testCases := []struct {
		name         string
		flags        string
		bucketExists bool
	}{
		{
			name:         "pre-existing bucket",
			bucketExists: true,
		},
		{
			name: "missing bucket",
		},
		{
			name:  "missing bucket with backend bootstrap",
			flags: "--backend-bootstrap",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			s3BucketName := "terragrunt-test-bucket-" + strings.ToLower(helpers.UniqueID())

			tmpEnvPath := helpers.CopyEnvironment(t, testFixtureS3BackendDisableInitRustFS)
			rootPath := filepath.Join(tmpEnvPath, testFixtureS3BackendDisableInitRustFS)
			configPath := filepath.Join(rootPath, "terragrunt.hcl")

			helpers.CopyAndFillMapPlaceholders(
				t,
				configPath,
				configPath,
				map[string]string{
					"__FILL_IN_BUCKET_NAME__": s3BucketName,
					"__FILL_IN_S3_ENDPOINT__": rustfsAddr,
				},
			)

			client := newRustFSClient(t, rustfsAddr)

			if tc.bucketExists {
				createRustFSBucket(t, client, s3BucketName)
			}

			recorder := helpers.NewExecRecorder(vexec.NewOSExec())

			err := helpers.RunTerragruntCommandWithVenv(
				t,
				t.Context(),
				rustfsVenv().WithExec(recorder),
				"terragrunt run plan "+tc.flags+" --non-interactive --working-dir "+rootPath,
			)

			commands := recorder.Commands()

			initIdx := slices.IndexFunc(commands, func(c helpers.RecordedCommand) bool {
				return len(c.Args) > 0 && c.Args[0] == tf.CommandNameInit
			})
			require.NotEqual(t, -1, initIdx, "expected tofu init, got %v", commands)

			initArgs := commands[initIdx].Args
			assert.Contains(t, initArgs, "-backend-config=bucket="+s3BucketName)
			assert.NotContains(t, initArgs, "-backend=false")

			if tc.bucketExists {
				require.NoError(t, err)

				return
			}

			assert.NotContains(
				t,
				listRustFSBuckets(t, client),
				s3BucketName,
				"Terragrunt must not create the bucket when disable_init is true",
			)
			require.Error(t, err, "plan must fail when the backend bucket does not exist")
		})
	}
}

// countEchoCommands returns how many recorded commands echo exactly msg.
func countEchoCommands(commands []helpers.RecordedCommand, msg string) int {
	count := 0

	for _, c := range commands {
		if filepath.Base(c.Name) == "echo" && slices.Equal(c.Args, []string{msg}) {
			count++
		}
	}

	return count
}

// listRustFSBuckets returns the names of every bucket in the RustFS instance
// behind c.
func listRustFSBuckets(t *testing.T, c *s3.Client) []string {
	t.Helper()

	out, err := c.ListBuckets(t.Context(), &s3.ListBucketsInput{})
	require.NoError(t, err)

	names := make([]string, 0, len(out.Buckets))
	for _, b := range out.Buckets {
		names = append(names, *b.Name)
	}

	return names
}
