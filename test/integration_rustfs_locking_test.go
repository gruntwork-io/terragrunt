//go:build docker

package test_test

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/gruntwork-io/terragrunt/internal/util"
	"github.com/gruntwork-io/terragrunt/test/helpers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/sync/errgroup"
)

const (
	testFixtureS3BackendUseLockfileRustFS = "fixtures/s3-backend-rustfs/use-lockfile"

	rustfsLockStateKey = "use-lockfile/terraform.tfstate"
	rustfsLockKey      = rustfsLockStateKey + ".tflock"
)

// TestRustFSNativeS3Locking covers tofu native S3 state locking (use_lockfile), which
// acquires the lock with a conditional PutObject (If-None-Match: *) on <key>.tflock and
// relies on the store to reject the write when the object exists.
func TestRustFSNativeS3Locking(t *testing.T) {
	t.Parallel()

	if !helpers.IsNativeS3LockingSupported(t) {
		t.Skip("Wrapped binary does not support native S3 locking")
	}

	rustfsAddr := setupRustFS(t)
	c := newRustFSClient(t, rustfsAddr)

	t.Run("apply releases lock", func(t *testing.T) {
		t.Parallel()

		bucket := "terragrunt-test-bucket-" + strings.ToLower(helpers.UniqueID())
		unitPath := copyRustFSLockFixture(t, rustfsAddr, bucket)

		_, _, err := runTerragruntRustFS(
			t,
			"terragrunt run --backend-bootstrap --non-interactive --working-dir "+unitPath+
				" -- apply -auto-approve -lock-timeout=0s",
		)
		require.NoError(t, err)

		assert.Equal(t, "initial", rustfsLockStateValue(t, c, bucket))
		assert.False(t, rustfsObjectExists(t, c, bucket, rustfsLockKey))
	})

	t.Run("foreign lock blocks apply", func(t *testing.T) {
		t.Parallel()

		bucket := "terragrunt-test-bucket-" + strings.ToLower(helpers.UniqueID())
		unitPath := copyRustFSLockFixture(t, rustfsAddr, bucket)

		_, _, err := runTerragruntRustFS(
			t,
			"terragrunt run --backend-bootstrap --non-interactive --working-dir "+unitPath+
				" -- apply -auto-approve -lock-timeout=0s",
		)
		require.NoError(t, err)

		stateBefore := rustfsObjectBody(t, c, bucket, rustfsLockStateKey)

		foreignLock := []byte(`{"ID":"00000000-0000-0000-0000-000000000000","Operation":"OperationTypeApply",` +
			`"Info":"","Who":"someone@elsewhere","Version":"1.10.0","Created":"2026-01-01T00:00:00Z",` +
			`"Path":"` + bucket + `/` + rustfsLockStateKey + `"}`)
		_, err = c.PutObject(t.Context(), &s3.PutObjectInput{
			Bucket: aws.String(bucket),
			Key:    aws.String(rustfsLockKey),
			Body:   bytes.NewReader(foreignLock),
		})
		require.NoError(t, err)

		v := rustfsVenv()
		v.Env["TF_VAR_value"] = "changed"

		applyCmd := "terragrunt run --non-interactive --working-dir " + unitPath +
			" -- apply -auto-approve -lock-timeout=0s"

		err = helpers.RunTerragruntCommandWithVenv(t, t.Context(), v, applyCmd)

		var processErr *util.ProcessExecutionError

		require.ErrorAs(t, err, &processErr)
		assert.Equal(t, stateBefore, rustfsObjectBody(t, c, bucket, rustfsLockStateKey))
		assert.Equal(t, string(foreignLock), rustfsObjectBody(t, c, bucket, rustfsLockKey))

		// Once the foreign lock is released, the same apply goes through.
		_, err = c.DeleteObject(t.Context(), &s3.DeleteObjectInput{
			Bucket: aws.String(bucket),
			Key:    aws.String(rustfsLockKey),
		})
		require.NoError(t, err)

		require.NoError(t, helpers.RunTerragruntCommandWithVenv(t, t.Context(), v, applyCmd))
		assert.Equal(t, "changed", rustfsLockStateValue(t, c, bucket))
		assert.False(t, rustfsObjectExists(t, c, bucket, rustfsLockKey))
	})

	t.Run("concurrent apply fails while lock is held", func(t *testing.T) {
		t.Parallel()

		bucket := "terragrunt-test-bucket-" + strings.ToLower(helpers.UniqueID())
		holderPath := copyRustFSLockFixture(t, rustfsAddr, bucket)
		contenderPath := copyRustFSLockFixture(t, rustfsAddr, bucket)

		// The holder's provisioner waits while the gate file exists, so it keeps the
		// lock until the test removes the gate.
		gatePath := filepath.Join(helpers.TmpDirWOSymlinks(t), "gate")
		require.NoError(t, os.WriteFile(gatePath, nil, 0o600))

		holderVenv := rustfsVenv()
		holderVenv.Env["TF_VAR_gate_path"] = gatePath

		var g errgroup.Group

		holderDone := make(chan struct{})

		g.Go(func() error {
			defer close(holderDone)

			return helpers.RunTerragruntCommandWithVenv(
				t,
				t.Context(),
				holderVenv,
				"terragrunt run --backend-bootstrap --non-interactive --working-dir "+holderPath+
					" -- apply -auto-approve -lock-timeout=0s",
			)
		})

		t.Cleanup(func() {
			assert.NoError(t, os.RemoveAll(gatePath))

			<-holderDone
		})

		holderLock := waitForRustFSLock(t, c, bucket, holderDone)

		_, _, err := runTerragruntRustFS(
			t,
			"terragrunt run --non-interactive --working-dir "+contenderPath+
				" -- apply -auto-approve -lock-timeout=0s",
		)

		var processErr *util.ProcessExecutionError

		require.ErrorAs(t, err, &processErr)
		assert.Equal(t, holderLock, rustfsObjectBody(t, c, bucket, rustfsLockKey))

		require.NoError(t, os.Remove(gatePath))
		require.NoError(t, g.Wait())

		assert.Equal(t, "initial", rustfsLockStateValue(t, c, bucket))
		assert.False(t, rustfsObjectExists(t, c, bucket, rustfsLockKey))

		// With the lock released, the contender's apply goes through.
		_, _, err = runTerragruntRustFS(
			t,
			"terragrunt run --non-interactive --working-dir "+contenderPath+
				" -- apply -auto-approve -lock-timeout=0s",
		)
		require.NoError(t, err)
	})
}

// copyRustFSLockFixture copies the use_lockfile fixture, points it at bucket on the
// RustFS endpoint, and returns the unit path.
func copyRustFSLockFixture(t *testing.T, endpoint, bucket string) string {
	t.Helper()

	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureS3BackendUseLockfileRustFS)
	rootPath := filepath.Join(tmpEnvPath, testFixtureS3BackendUseLockfileRustFS)

	rootConfigPath := filepath.Join(rootPath, "root.hcl")
	helpers.CopyAndFillMapPlaceholders(
		t,
		rootConfigPath,
		rootConfigPath,
		map[string]string{
			"__FILL_IN_BUCKET_NAME__": bucket,
			"__FILL_IN_S3_ENDPOINT__": endpoint,
		},
	)

	return filepath.Join(rootPath, "unit")
}

// waitForRustFSLock polls until the lock object exists and returns its body. It fails
// the test if holderDone closes first or the lock does not show up within two minutes.
func waitForRustFSLock(t *testing.T, c *s3.Client, bucket string, holderDone <-chan struct{}) string {
	t.Helper()

	timeout := time.After(2 * time.Minute)
	ticker := time.NewTicker(200 * time.Millisecond)

	defer ticker.Stop()

	for {
		select {
		case <-holderDone:
			require.FailNow(t, "holder apply finished before the lock was observed")
		case <-timeout:
			require.FailNow(t, "timed out waiting for the lock object")
		case <-ticker.C:
			if rustfsObjectExists(t, c, bucket, rustfsLockKey) {
				return rustfsObjectBody(t, c, bucket, rustfsLockKey)
			}
		}
	}
}

func rustfsObjectExists(t *testing.T, c *s3.Client, bucket, key string) bool {
	t.Helper()

	_, err := c.HeadObject(t.Context(), &s3.HeadObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
	})
	if err == nil {
		return true
	}

	var notFound *types.NotFound

	require.ErrorAs(t, err, &notFound)

	return false
}

func rustfsObjectBody(t *testing.T, c *s3.Client, bucket, key string) string {
	t.Helper()

	out, err := c.GetObject(t.Context(), &s3.GetObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
	})
	require.NoError(t, err)

	defer func() {
		require.NoError(t, out.Body.Close())
	}()

	body, err := io.ReadAll(out.Body)
	require.NoError(t, err)

	return string(body)
}

// rustfsLockStateValue returns the value output recorded in the fixture's state object.
func rustfsLockStateValue(t *testing.T, c *s3.Client, bucket string) string {
	t.Helper()

	var state struct {
		Outputs map[string]struct {
			Value string `json:"value"`
		} `json:"outputs"`
	}

	require.NoError(t, json.Unmarshal([]byte(rustfsObjectBody(t, c, bucket, rustfsLockStateKey)), &state))

	return state.Outputs["value"].Value
}
