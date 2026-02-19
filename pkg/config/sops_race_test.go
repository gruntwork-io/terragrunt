package config //nolint:testpackage // needs access to sopsDecryptFileImpl

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/gruntwork-io/terragrunt/pkg/options"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestSOPSDecryptConcurrencyWithRacing is a regression test for
// https://github.com/gruntwork-io/terragrunt/issues/5515
//
// Run with -race to detect data races in env var handling during concurrent
// SOPS decryption. The CI "Race" job runs tests matching .*WithRacing with -race.
//
// Multiple goroutines call sopsDecryptFileImpl concurrently with different
// opts.Env credentials. Without proper locking, the race detector catches
// concurrent os.Setenv/os.Getenv/os.Unsetenv calls.
func TestSOPSDecryptConcurrencyWithRacing(t *testing.T) {
	t.Parallel()

	const (
		authKey       = "SOPS_RACE_TEST_TOKEN"
		numGoroutines = 10
	)

	dir := t.TempDir()

	var files []string

	for i := 1; i <= numGoroutines; i++ {
		unitDir := filepath.Join(dir, fmt.Sprintf("unit-%02d", i))
		require.NoError(t, os.MkdirAll(unitDir, 0755))

		secretFile := filepath.Join(unitDir, "secret.enc.json")
		require.NoError(t, os.WriteFile(secretFile,
			[]byte(fmt.Sprintf(`{"value":"secret-from-unit-%02d"}`, i)), 0644))

		files = append(files, secretFile)
	}

	// Mock decrypt that reads the env var (creating a read that races with
	// concurrent Setenv/Unsetenv if locking is broken).
	mockDecryptFn := func(path string, _ string) ([]byte, error) {
		_ = os.Getenv(authKey) // read that would race without lock

		return os.ReadFile(path)
	}

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

			opts, err := options.NewTerragruntOptionsForTest(filePath)
			if !assert.NoError(t, err) {
				return
			}

			opts.WorkingDir = filepath.Dir(filePath)
			opts.Env = map[string]string{authKey: fmt.Sprintf("token-%d", idx)}

			l := logger.CreateLogger()
			_, pctx := NewParsingContext(ctx, l, opts.StrictControls)
			pctx.WorkingDir = opts.WorkingDir
			pctx.Env = opts.Env

			result, err := sopsDecryptFileImpl(ctx, pctx, l, filePath, "json", mockDecryptFn)
			assert.NoError(t, err)
			assert.Contains(t, result, `"value":"secret-from-unit-`)
		}(i, f)
	}

	close(barrier)
	wg.Wait()

	// Verify env is clean after all goroutines complete.
	_, exists := os.LookupEnv(authKey)
	require.False(t, exists, "env var must be cleaned up after concurrent decrypts")
}
