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

// TestSOPSDecryptEnvPropagation is a deterministic regression test for
// https://github.com/gruntwork-io/terragrunt/issues/5515
//
// The original customer-reported bug: sops_decrypt_file() during HCL evaluation
// couldn't authenticate to KMS because auth-provider credentials were not yet
// loaded into opts.Env. This caused SOPS to return empty/wrong secrets.
//
// This test verifies the env propagation contract of sopsDecryptFileImpl:
//   - Fresh credentials from opts.Env override stale process env during decrypt
//   - Process env is restored to original values after decrypt completes
//   - Without credentials, decrypt fails (reproduces the original bug)
//   - Concurrent goroutines with different credentials are properly isolated
func TestSOPSDecryptEnvPropagation(t *testing.T) { //nolint:paralleltest // mutates package-global sopsCache and process env vars
	const authKey = "SOPS_TEST_AUTH_CRED"

	origCache := sopsCache

	t.Cleanup(func() {
		sopsCache = origCache

		os.Unsetenv(authKey) //nolint:errcheck
	})

	secretFiles := generateTestSecretFiles(t, 1)
	secretFile := secretFiles[0]

	// Mock decryptFn that requires authKey=="fresh-token" — simulates KMS auth.
	authRequiringDecryptFn := func(path string, _ string) ([]byte, error) {
		token := os.Getenv(authKey)
		if token != "fresh-token" {
			return nil, fmt.Errorf("KMS auth failed: no valid credential (got %q)", token)
		}

		return os.ReadFile(path)
	}

	// Subtest 1: Fresh credentials from opts.Env must override stale process env.
	// Models: auth-provider returns fresh token, but process env has stale one from previous run.
	t.Run("fresh_creds_override_stale_process_env", func(t *testing.T) { //nolint:paralleltest // mutates sopsCache and process env
		sopsCache = cache.NewCache[string](sopsCacheName)

		t.Setenv(authKey, "stale-token")

		opts, err := options.NewTerragruntOptionsForTest(secretFile)
		require.NoError(t, err)

		opts.WorkingDir = filepath.Dir(secretFile)
		opts.Env = map[string]string{authKey: "fresh-token"}

		l := logger.CreateLogger()
		ctx := context.Background()
		ctx = WithConfigValues(ctx)
		_, pctx := NewParsingContext(ctx, l, opts)

		result, err := sopsDecryptFileImpl(ctx, pctx, l, secretFile, "json", authRequiringDecryptFn)
		require.NoError(t, err, "decrypt must succeed with fresh credentials from opts.Env")
		assert.Contains(t, result, `"value":"secret-from-unit-01"`)

		// Process env must be restored to stale value after decrypt
		assert.Equal(t, "stale-token", os.Getenv(authKey),
			"process env must be restored to original value after decrypt")
	})

	// Subtest 2: Credentials injected when absent from process env.
	// Models: first run, auth-provider loaded creds into opts.Env, process env was empty.
	t.Run("new_creds_set_when_absent_from_process_env", func(t *testing.T) { //nolint:paralleltest // mutates sopsCache and process env
		sopsCache = cache.NewCache[string](sopsCacheName)

		os.Unsetenv(authKey) //nolint:errcheck

		opts, err := options.NewTerragruntOptionsForTest(secretFile)
		require.NoError(t, err)

		opts.WorkingDir = filepath.Dir(secretFile)
		opts.Env = map[string]string{authKey: "fresh-token"}

		l := logger.CreateLogger()
		ctx := context.Background()
		ctx = WithConfigValues(ctx)
		_, pctx := NewParsingContext(ctx, l, opts)

		result, err := sopsDecryptFileImpl(ctx, pctx, l, secretFile, "json", authRequiringDecryptFn)
		require.NoError(t, err, "decrypt must succeed with fresh credentials from opts.Env")
		assert.Contains(t, result, `"value":"secret-from-unit-01"`)

		// Process env must be restored to empty after decrypt
		assert.Empty(t, os.Getenv(authKey),
			"process env must be restored to empty after decrypt")
	})

	// Subtest 3: Missing credentials cause decrypt failure.
	// Reproduces the ORIGINAL bug: auth-provider hasn't run yet, opts.Env has no
	// auth token, process env has no auth token → SOPS can't authenticate to KMS.
	t.Run("missing_creds_fails_decrypt", func(t *testing.T) { //nolint:paralleltest // mutates sopsCache and process env
		sopsCache = cache.NewCache[string](sopsCacheName)

		os.Unsetenv(authKey) //nolint:errcheck

		opts, err := options.NewTerragruntOptionsForTest(secretFile)
		require.NoError(t, err)

		opts.WorkingDir = filepath.Dir(secretFile)
		// Empty env — simulates auth-provider NOT having run (the original bug)
		opts.Env = map[string]string{}

		l := logger.CreateLogger()
		ctx := context.Background()
		ctx = WithConfigValues(ctx)
		_, pctx := NewParsingContext(ctx, l, opts)

		_, err = sopsDecryptFileImpl(ctx, pctx, l, secretFile, "json", authRequiringDecryptFn)
		require.Error(t, err,
			"decrypt must fail without auth credentials — reproduces original issue #5515")
	})

	// Subtest 4: Concurrent goroutines with DIFFERENT auth tokens are isolated.
	// Models production: multiple units decrypt in parallel, each with different
	// auth-provider credentials. The lock must ensure each sees its OWN token.
	t.Run("concurrent_different_creds_isolated", func(t *testing.T) { //nolint:paralleltest // mutates sopsCache and process env
		const numGoroutines = 5

		sopsCache = cache.NewCache[string](sopsCacheName)

		os.Unsetenv(authKey) //nolint:errcheck

		files := generateTestSecretFiles(t, numGoroutines)

		var wg sync.WaitGroup

		barrier := make(chan struct{})

		var failures atomic.Int32

		for i, f := range files {
			wg.Add(1)

			go func(idx int, filePath string) {
				defer wg.Done()

				<-barrier

				expectedToken := fmt.Sprintf("token-%d", idx)

				opts, err := options.NewTerragruntOptionsForTest(filePath)
				if !assert.NoError(t, err) {
					failures.Add(1)

					return
				}

				opts.WorkingDir = filepath.Dir(filePath)
				opts.Env = map[string]string{authKey: expectedToken}

				// Each goroutine's decryptFn verifies it sees ITS OWN token
				tokenCheckFn := func(path string, _ string) ([]byte, error) {
					actual := os.Getenv(authKey)
					if actual != expectedToken {
						return nil, fmt.Errorf("goroutine %d: expected %q, got %q", idx, expectedToken, actual)
					}

					return os.ReadFile(path)
				}

				l := logger.CreateLogger()
				ctx := context.Background()
				ctx = WithConfigValues(ctx)
				_, pctx := NewParsingContext(ctx, l, opts)

				result, decryptErr := sopsDecryptFileImpl(ctx, pctx, l, filePath, "json", tokenCheckFn)
				if decryptErr != nil {
					t.Logf("goroutine %d failed: %v", idx, decryptErr)
					failures.Add(1)

					return
				}

				expectedPrefix := `{"value":"secret-from-unit-`
				if len(result) < len(expectedPrefix) || result[:len(expectedPrefix)] != expectedPrefix {
					t.Logf("goroutine %d: wrong content: %s", idx, result)
					failures.Add(1)
				}
			}(i, f)
		}

		close(barrier)
		wg.Wait()

		require.Zero(t, failures.Load(),
			"all goroutines must see their own auth token during decrypt — env isolation failed")

		assert.Empty(t, os.Getenv(authKey),
			"process env must be clean after all concurrent decrypts")
	})
}
