//go:build sops

package config //nolint:testpackage // needs access to sopsCache internals

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gruntwork-io/terragrunt/internal/cache"
	"github.com/gruntwork-io/terragrunt/internal/locks"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/pkg/options"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// generateTestSecretFiles creates plain JSON files in a temp directory.
// No SOPS encryption needed — the test injects a mock decryptFn to read raw files.
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

// sopsDecryptFileImplBuggy replicates the ORIGINAL buggy implementation from main branch.
// The bug: lock is ONLY acquired when len(env) > 0, leaving goroutines without env vars
// unprotected. They can observe env var mutations from locked goroutines mid-operation.
// This function exists solely to demonstrate the race condition in tests.
func sopsDecryptFileImplBuggy(ctx context.Context, pctx *ParsingContext, _ log.Logger, path string, format string, decryptFn func(string, string) ([]byte, error)) (string, error) {
	env := pctx.TerragruntOptions.Env
	if len(env) > 0 {
		locks.EnvLock.Lock()
		defer locks.EnvLock.Unlock()

		for k, v := range env {
			if os.Getenv(k) == "" {
				os.Setenv(k, v)      //nolint:errcheck
				defer os.Unsetenv(k) //nolint:errcheck
			}
		}
	}
	// NO lock held here for goroutines without env vars — decryptFn runs unprotected

	if val, ok := sopsCache.Get(ctx, path); ok {
		return val, nil
	}

	rawData, err := decryptFn(path, format)
	if err != nil {
		return "", err
	}

	value := string(rawData)
	sopsCache.Put(ctx, path, value)

	return value, nil
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
// This test injects a delay into decryptFn to simulate KMS latency,
// then detects when env vars disappear mid-operation.
//
// Without the fix (conditional lock): FAILS — env vars change mid-decrypt.
// With the fix (always lock):         PASSES — operations are serialized.
func TestSOPSDecryptConcurrencyRace(t *testing.T) { //nolint:paralleltest // mutates package-global sopsCache and process env vars via os.Setenv/os.Unsetenv
	const testEnvKey = "SOPS_TEST_AUTH_TOKEN"

	origCache := sopsCache

	t.Cleanup(func() {
		sopsCache = origCache

		os.Unsetenv(testEnvKey) //nolint:errcheck
	})

	var envVarRaces atomic.Int32

	var decryptErrors atomic.Int32

	// Mock decrypt function that simulates KMS latency and detects env var races.
	mockDecryptFn := func(path string, format string) ([]byte, error) {
		// Check if WE are the goroutine that set the env var.
		// The production code does os.Setenv BEFORE calling decryptFn,
		// so if the env var is set here, we're the "setter" goroutine.
		if os.Getenv(testEnvKey) != "" {
			// Setter goroutine: short delay so our deferred os.Unsetenv
			// runs quickly, while unlocked goroutines are still mid-operation.
			time.Sleep(5 * time.Millisecond)
		} else {
			// Non-setter goroutine: longer delay to ensure we're still
			// inside decryptFn when setters' deferred os.Unsetenv runs.
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
	}

	// Generate plain JSON files in temp dir (no SOPS encryption needed)
	const numFiles = 10

	secretFiles := generateTestSecretFiles(t, numFiles)

	t.Logf("Using %d secret files to decrypt concurrently", len(secretFiles))

	// Run enough iterations to reliably trigger the race
	const iterations = 50

	for iter := 0; iter < iterations; iter++ {
		// Clear cache to force fresh decryption each iteration
		sopsCache = cache.NewCache[string](sopsCacheName)

		var wg sync.WaitGroup

		barrier := make(chan struct{})

		for i, sf := range secretFiles {
			wg.Add(1)

			go func(idx int, filePath string) {
				defer wg.Done()

				<-barrier

				opts, err := options.NewTerragruntOptionsForTest(filePath)
				if !assert.NoError(t, err, "NewTerragruntOptionsForTest") {
					return
				}

				opts.WorkingDir = filepath.Dir(filePath)

				// Half goroutines set env var via opts.Env (like auth-provider).
				// In buggy code only these acquire the lock.
				// The other half run unlocked — that's the race.
				if idx%2 == 0 {
					opts.Env = map[string]string{testEnvKey: "valid-token"}
				}

				l := logger.CreateLogger()
				ctx := context.Background()
				ctx = WithConfigValues(ctx)
				_, pctx := NewParsingContext(ctx, l, opts)

				// Call sopsDecryptFileImpl directly with mock decryptFn
				if _, decryptErr := sopsDecryptFileImpl(ctx, pctx, l, filePath, "json", mockDecryptFn); decryptErr != nil {
					decryptErrors.Add(1)
				}
			}(i, sf)
		}

		close(barrier)
		wg.Wait()
	}

	t.Logf("Env var races detected: %d, decrypt errors: %d (across %d iterations x %d files)",
		envVarRaces.Load(), decryptErrors.Load(), iterations, len(secretFiles))

	require.Zero(t, decryptErrors.Load(),
		"sopsDecryptFileImpl returned errors — possible regression in decrypt logic")

	require.Zero(t, envVarRaces.Load(),
		"Env vars changed during decrypt — race condition detected (issue #5515)")
}

// TestSOPSDecryptConcurrencyDataCorruption is an integration-style regression test
// for https://github.com/gruntwork-io/terragrunt/issues/5515
//
// This test verifies DATA CORRECTNESS under concurrent decryption. It uses a mock
// decryptFn that detects env var INSTABILITY during the decrypt operation — the
// signature of the race condition.
//
// The mock decryptFn captures the auth env var at START and END of the operation
// (with simulated KMS latency in between). If the value changes mid-operation
// (set→unset transition), that indicates a race — another goroutine's deferred
// os.Unsetenv ran while this goroutine was mid-decrypt. The mock returns corrupted
// output "{}" when this happens, simulating what customers see as wrong secret values.
//
// Part 1: Runs the BUGGY implementation (conditional lock) — expects data corruption.
// Part 2: Runs the FIXED implementation (unconditional lock) — expects all correct.
func TestSOPSDecryptConcurrencyDataCorruption(t *testing.T) { //nolint:paralleltest // mutates package-global sopsCache and process env vars via os.Setenv/os.Unsetenv
	const (
		testAuthKey   = "SOPS_TEST_KMS_TOKEN"
		testAuthValue = "valid-kms-token"
		numFiles      = 20
		iterations    = 30
	)

	origCache := sopsCache

	t.Cleanup(func() {
		sopsCache = origCache

		os.Unsetenv(testAuthKey) //nolint:errcheck
	})

	// Ensure clean env before test
	os.Unsetenv(testAuthKey) //nolint:errcheck

	secretFiles := generateTestSecretFiles(t, numFiles)

	// Mock decryptFn that detects env var instability during operation.
	// Simulates KMS latency with a sleep, then checks if the auth env var
	// changed between the start and end of the operation.
	//
	// Stable env var (same value at start and end) → return correct file content.
	// Unstable env var (changed mid-operation) → return corrupted "{}" output.
	raceDetectingDecryptFn := func(path string, format string) ([]byte, error) {
		// Capture env var state at START of decrypt
		startVal := os.Getenv(testAuthKey)

		// Simulate KMS network latency — widens the race window
		time.Sleep(10 * time.Millisecond)

		// Capture env var state at END of decrypt
		endVal := os.Getenv(testAuthKey)

		// Detect env var instability: if the value changed mid-operation,
		// another goroutine's deferred os.Unsetenv ran while we were
		// mid-decrypt — this is the race condition.
		if startVal != endVal {
			// Auth token appeared then disappeared (or vice versa) mid-operation.
			// In production this causes KMS auth failure mid-request, returning
			// empty/wrong secrets — exactly what customers report.
			return []byte("{}"), nil
		}

		// Env var was stable throughout the operation — return correct content
		return os.ReadFile(path)
	}

	// runConcurrentDecrypts runs all secretFiles through the given decryptImpl concurrently.
	// Returns count of files that got wrong (corrupted) results.
	runConcurrentDecrypts := func(
		t *testing.T,
		decryptImpl func(context.Context, *ParsingContext, log.Logger, string, string, func(string, string) ([]byte, error)) (string, error),
	) int {
		t.Helper()

		var corruptedResults atomic.Int32

		for iter := 0; iter < iterations; iter++ {
			sopsCache = cache.NewCache[string](sopsCacheName)

			var wg sync.WaitGroup

			barrier := make(chan struct{})

			for i, sf := range secretFiles {
				wg.Add(1)

				go func(idx int, filePath string) {
					defer wg.Done()

					<-barrier

					opts, optsErr := options.NewTerragruntOptionsForTest(filePath)
					if !assert.NoError(t, optsErr, "NewTerragruntOptionsForTest") {
						corruptedResults.Add(1)
						return
					}

					opts.WorkingDir = filepath.Dir(filePath)

					// Half goroutines have auth-provider env vars (like --auth-provider-cmd).
					// In buggy code, only THESE goroutines acquire the lock.
					// The other half run unlocked and can observe env var mutations
					// from locked goroutines during their decrypt operation.
					if idx%2 == 0 {
						opts.Env = map[string]string{testAuthKey: testAuthValue}
					}

					l := logger.CreateLogger()
					ctx := context.Background()
					ctx = WithConfigValues(ctx)
					_, pctx := NewParsingContext(ctx, l, opts)

					result, err := decryptImpl(ctx, pctx, l, filePath, "json", raceDetectingDecryptFn)
					if err != nil {
						corruptedResults.Add(1)
						return
					}

					// Verify correct file content was returned
					expectedPrefix := `{"value":"secret-from-unit-`
					if result == "{}" {
						corruptedResults.Add(1)
					} else if len(result) < len(expectedPrefix) || result[:len(expectedPrefix)] != expectedPrefix {
						corruptedResults.Add(1)
					}
				}(i, sf)
			}

			close(barrier)
			wg.Wait()
		}

		return int(corruptedResults.Load())
	}

	// --- Part 1: Buggy implementation (conditional lock) — observability only ---
	// Race conditions are probabilistic. Under favorable scheduling the race may not trigger,
	// so we only log the result instead of asserting. The important assertion is Part 2.
	t.Run("buggy_conditional_lock_causes_corruption", func(t *testing.T) { //nolint:paralleltest // shares sopsCache with other subtest
		corrupted := runConcurrentDecrypts(t, sopsDecryptFileImplBuggy)
		t.Logf("Buggy implementation: %d corrupted results out of %d total (%d iterations x %d files)",
			corrupted, iterations*numFiles, iterations, numFiles)

		if corrupted == 0 {
			t.Log("Race did not trigger this run — this is expected on fast machines or favorable scheduling")
		}
	})

	// --- Part 2: Fixed implementation (unconditional lock) should produce zero corruption ---
	t.Run("fixed_unconditional_lock_prevents_corruption", func(t *testing.T) { //nolint:paralleltest // shares sopsCache with other subtest
		corrupted := runConcurrentDecrypts(t, sopsDecryptFileImpl)
		t.Logf("Fixed implementation: %d corrupted results out of %d total (%d iterations x %d files)",
			corrupted, iterations*numFiles, iterations, numFiles)

		require.Zero(t, corrupted,
			"Fixed unconditional-lock code should never produce corrupted results")
	})
}
