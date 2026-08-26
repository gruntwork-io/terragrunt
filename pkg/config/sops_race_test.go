package config //nolint:testpackage // needs access to sopsDecryptFileImpl

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gruntwork-io/terragrunt/internal/strict/controls"
	"github.com/gruntwork-io/terragrunt/internal/vsops"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/gruntwork-io/terragrunt/test/helpers/venvtest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestSOPSDecryptConcurrencyWithRacing covers the config half of
// https://github.com/gruntwork-io/terragrunt/issues/5515: units decrypting at
// the same time each reach the decrypter with their own credentials, and the
// shared decrypt cache tolerates the concurrency. internal/vsops covers the
// process-environment window those credentials travel through.
//
// The CI "Race" job runs tests matching .*WithRacing with -race.
func TestSOPSDecryptConcurrencyWithRacing(t *testing.T) {
	t.Parallel()

	const (
		authKey       = "SOPS_RACE_TEST_TOKEN"
		numGoroutines = 10
	)

	dir := t.TempDir()

	files := make([]string, 0, numGoroutines)

	for i := 1; i <= numGoroutines; i++ {
		unitDir := filepath.Join(dir, fmt.Sprintf("unit-%02d", i))
		require.NoError(t, os.MkdirAll(unitDir, 0o755))

		secretFile := filepath.Join(unitDir, "secret.enc.json")
		require.NoError(t, os.WriteFile(secretFile,
			fmt.Appendf(nil, `{"value":"secret-from-unit-%02d"}`, i), 0o644))

		files = append(files, secretFile)
	}

	// Echoing the token back into the cleartext is what ties each result to the
	// venv it was decrypted with, so a leak between units shows up as a
	// mismatch rather than as a passing test.
	mockDecrypter := vsops.NewMemDecrypter(func(env map[string]string, path string, _ string) ([]byte, error) {
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}

		return fmt.Appendf(data, "\n%s", env[authKey]), nil
	})

	var (
		wg      sync.WaitGroup
		barrier = make(chan struct{})
	)

	ctx := WithConfigValues(t.Context())

	for i, f := range files {
		wg.Add(1)

		go func(idx int, filePath string) {
			defer wg.Done()

			<-barrier

			token := fmt.Sprintf("token-%d", idx)

			l := logger.CreateLogger()
			v := venvtest.NewWithOSFS().WithEnv(map[string]string{authKey: token})

			_, pctx := NewParsingContext(ctx, l, v, WithStrictControls(controls.New()))
			pctx.WorkingDir = filepath.Dir(filePath)

			result, err := sopsDecryptFileImpl(ctx, pctx, l, filePath, "json", mockDecrypter)
			assert.NoError(t, err)
			assert.Contains(t, result, `"value":"secret-from-unit-`)
			assert.Contains(t, result, token)
		}(i, f)
	}

	close(barrier)
	wg.Wait()
}

// TestSOPSDecryptDistinctPathsOverlapWithRacing pins that a decrypt holds up
// only the units reading the same file. Every decrypter here blocks until all
// of them have been entered, so a lock covering the whole cache would never let
// the last one arrive.
func TestSOPSDecryptDistinctPathsOverlapWithRacing(t *testing.T) {
	t.Parallel()

	const numUnits = 4

	dir := t.TempDir()

	files := make([]string, 0, numUnits)

	for i := range numUnits {
		secretFile := filepath.Join(dir, fmt.Sprintf("unit-%02d.enc.json", i))
		require.NoError(t, os.WriteFile(secretFile, []byte(`{"value":"secret"}`), 0o644))

		files = append(files, secretFile)
	}

	var (
		entered = make(chan struct{}, numUnits)
		release = make(chan struct{})
	)

	blockingDecrypter := vsops.NewMemDecrypter(
		func(_ map[string]string, path string, _ string) ([]byte, error) {
			entered <- struct{}{}

			<-release

			return os.ReadFile(path)
		})

	ctx := WithConfigValues(t.Context())

	var wg sync.WaitGroup

	for _, f := range files {
		wg.Go(func() {
			l := logger.CreateLogger()
			v := venvtest.NewWithOSFS().WithEnv(map[string]string{})

			_, pctx := NewParsingContext(ctx, l, v, WithStrictControls(controls.New()))
			pctx.WorkingDir = dir

			result, err := sopsDecryptFileImpl(ctx, pctx, l, f, "json", blockingDecrypter)
			require.NoError(t, err)
			assert.Contains(t, result, `"value":"secret"`)
		})
	}

	for range numUnits {
		select {
		case <-entered:
		case <-time.After(10 * time.Second):
			close(release)
			wg.Wait()
			t.Fatal("decrypts of distinct files did not overlap")
		}
	}

	close(release)
	wg.Wait()
}

// TestSOPSDecryptDeduplicatesSamePathWithRacing pins that units sharing an
// encrypted file pay for one decrypt between them. A SOPS decrypt can be a KMS
// round-trip, so a second one is not merely wasted CPU.
func TestSOPSDecryptDeduplicatesSamePathWithRacing(t *testing.T) {
	t.Parallel()

	const numGoroutines = 8

	dir := t.TempDir()
	secretFile := filepath.Join(dir, "shared.enc.json")
	require.NoError(t, os.WriteFile(secretFile, []byte(`{"value":"shared"}`), 0o644))

	var decrypts atomic.Int64

	countingDecrypter := vsops.NewMemDecrypter(
		func(_ map[string]string, path string, _ string) ([]byte, error) {
			decrypts.Add(1)

			return os.ReadFile(path)
		})

	var (
		wg      sync.WaitGroup
		barrier = make(chan struct{})
	)

	ctx := WithConfigValues(t.Context())

	for range numGoroutines {
		wg.Go(func() {
			<-barrier

			l := logger.CreateLogger()
			v := venvtest.NewWithOSFS().WithEnv(map[string]string{})

			_, pctx := NewParsingContext(ctx, l, v, WithStrictControls(controls.New()))
			pctx.WorkingDir = dir

			result, err := sopsDecryptFileImpl(ctx, pctx, l, secretFile, "json", countingDecrypter)
			require.NoError(t, err)
			assert.Contains(t, result, `"value":"shared"`)
		})
	}

	close(barrier)
	wg.Wait()

	assert.Equal(t, int64(1), decrypts.Load(), "the file should be decrypted once for all readers")
}
