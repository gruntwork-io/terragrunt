//go:build sops

package config_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gruntwork-io/terragrunt/pkg/config"
	"github.com/gruntwork-io/terragrunt/pkg/options"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/stretchr/testify/require"
)

// generateTestSecretFiles creates plain JSON files in a temp directory.
// No SOPS encryption needed — the test injects sopsDecryptFn to read raw files.
func generateTestSecretFiles(t *testing.T, count int) []string {
	t.Helper()

	dir := t.TempDir()

	var files []string

	for i := 1; i <= count; i++ {
		unitDir := filepath.Join(dir, fmt.Sprintf("unit-%02d", i))
		require.NoError(t, os.MkdirAll(unitDir, 0755))

		secretFile := filepath.Join(unitDir, "secret.enc.json")
		require.NoError(t, os.WriteFile(secretFile,
			[]byte(fmt.Sprintf(`{"value":"secret-from-unit-%02d"}`, i)), 0644))

		files = append(files, secretFile)
	}

	return files
}

// TestSOPSDecryptConcurrencyRace is a regression test for
// https://github.com/gruntwork-io/terragrunt/issues/5515
//
// The bug: sopsDecryptFileImpl only acquires EnvLock when len(env) > 0.
// Goroutines without env vars run unlocked, and can observe env var changes
// made by locked goroutines (set via os.Setenv, then deferred os.Unsetenv).
// With KMS-based decryption, the network latency makes this race window large
// enough to hit reliably.
//
// This test injects a delay into sopsDecryptFn to simulate KMS latency,
// then detects when env vars disappear mid-operation.
//
// Without the fix (conditional lock): FAILS — env vars change mid-decrypt.
// With the fix (always lock):         PASSES — operations are serialized.
func TestSOPSDecryptConcurrencyRace(t *testing.T) {
	t.Parallel()

	const testEnvKey = "SOPS_TEST_AUTH_TOKEN"

	// Inject delay into decrypt function to simulate KMS network latency.
	var envVarRaces atomic.Int32

	origFn := config.GetSopsDecryptFn()

	t.Cleanup(func() { config.SetSopsDecryptFn(origFn) })

	config.SetSopsDecryptFn(func(path string, format string) ([]byte, error) {
		// Check if WE are the goroutine that set the env var.
		// The production code does os.Setenv BEFORE calling sopsDecryptFn,
		// so if the env var is set here, we're the "setter" goroutine.
		if os.Getenv(testEnvKey) != "" {
			// Setter goroutine: short delay so our deferred os.Unsetenv
			// runs quickly, while unlocked goroutines are still mid-operation.
			time.Sleep(5 * time.Millisecond)
		} else {
			// Non-setter goroutine: longer delay to ensure we're still
			// inside sopsDecryptFn when setters' deferred os.Unsetenv runs.
			// Poll the env var to detect the set→unset transition.
			deadline := time.Now().Add(50 * time.Millisecond)
			sawSet := false

			for time.Now().Before(deadline) {
				val := os.Getenv(testEnvKey)
				if val != "" {
					sawSet = true
				} else if sawSet {
					// Env var was set by another goroutine, now it's
					// gone because their deferred os.Unsetenv ran
					// while we're unprotected by the lock — race!
					envVarRaces.Add(1)

					break
				}

				time.Sleep(50 * time.Microsecond)
			}
		}

		// Return raw file content — no real SOPS decryption needed for race detection.
		return os.ReadFile(path)
	})

	// Generate plain JSON files in temp dir (no SOPS encryption needed)
	const numFiles = 10

	secretFiles := generateTestSecretFiles(t, numFiles)

	t.Logf("Using %d secret files to decrypt concurrently", len(secretFiles))

	// Run enough iterations to reliably trigger the race
	const iterations = 50

	for iter := 0; iter < iterations; iter++ {
		// Clear cache to force fresh decryption each iteration
		config.ResetSopsCache()

		var wg sync.WaitGroup

		barrier := make(chan struct{})

		for i, sf := range secretFiles {
			wg.Add(1)

			go func(idx int, filePath string) {
				defer wg.Done()

				<-barrier

				opts, _ := options.NewTerragruntOptionsForTest(filePath)
				opts.WorkingDir = filepath.Dir(filePath)

				// Half goroutines set env var via opts.Env (like auth-provider).
				// In buggy code only these acquire the lock.
				// The other half run unlocked — that's the race.
				if idx%2 == 0 {
					opts.Env = map[string]string{testEnvKey: "valid-token"}
				}

				l := logger.CreateLogger()
				ctx := context.Background()
				ctx = config.WithConfigValues(ctx)
				_, pctx := config.NewParsingContext(ctx, l, opts)

				// Call production code end-to-end
				config.SopsDecryptFile(ctx, pctx, l, []string{filePath})
			}(i, sf)
		}

		close(barrier)
		wg.Wait()
	}

	t.Logf("Env var races detected: %d (across %d iterations x %d files)",
		envVarRaces.Load(), iterations, len(secretFiles))

	require.Zero(t, envVarRaces.Load(),
		"Env vars changed during decrypt — race condition detected (issue #5515)")
}
