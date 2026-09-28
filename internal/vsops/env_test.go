package vsops_test

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/vsops"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestOSDecrypterEnvIsolationWithRacing is a regression test for
// https://github.com/gruntwork-io/terragrunt/issues/5515. The CI "Race" job
// runs tests matching .*WithRacing with -race, which is what catches two
// decrypts holding the process environment at once.
//
// The files are not sops documents, so every decrypt fails. The failure is the
// point: the environment has to be restored on the error path too.
func TestOSDecrypterEnvIsolationWithRacing(t *testing.T) {
	t.Parallel()

	const (
		authKey       = "SOPS_RACE_TEST_TOKEN"
		numGoroutines = 10
	)

	dir := t.TempDir()
	d := vsops.NewOSDecrypter()

	files := make([]string, 0, numGoroutines)

	for i := range numGoroutines {
		path := filepath.Join(dir, fmt.Sprintf("secret-%02d.json", i))
		require.NoError(t, os.WriteFile(path, fmt.Appendf(nil, `{"value":%d}`, i), 0o600))

		files = append(files, path)
	}

	var (
		wg      sync.WaitGroup
		barrier = make(chan struct{})
	)

	for i, path := range files {
		wg.Go(func() {
			<-barrier

			env := map[string]string{authKey: fmt.Sprintf("token-%d", i)}

			_, err := d.DecryptFile(env, path, "json")
			assert.Error(t, err)
		})
	}

	close(barrier)
	wg.Wait()

	_, exists := os.LookupEnv(authKey)
	assert.False(t, exists, "the decrypt window must not outlive the decrypt")
}

// TestOSDecrypterRestoresDisplacedEnv pins the restore half of the window: a
// variable the process already carried has to come back with its own value,
// not the one a decrypt borrowed the slot for.
func TestOSDecrypterRestoresDisplacedEnv(t *testing.T) {
	const (
		key   = "SOPS_RESTORE_TEST_TOKEN"
		prior = "value-the-process-started-with"
	)

	t.Setenv(key, prior)

	path := filepath.Join(t.TempDir(), "secret.json")
	require.NoError(t, os.WriteFile(path, []byte(`{"value":1}`), 0o600))

	_, err := vsops.NewOSDecrypter().DecryptFile(map[string]string{key: "value-from-the-venv"}, path, "json")
	require.Error(t, err)

	assert.Equal(t, prior, os.Getenv(key))
}

// TestPublishEnvHoldsTheVenvValue pins what the process environment holds
// while the window is open, including the blank case [vsops.PublishEnv]
// documents.
//
// Each case reads the variable while the window is open, closes it, and
// asserts afterwards, so a failed expectation cannot leave [vsops.PublishEnv]
// holding the lock.
func TestPublishEnvHoldsTheVenvValue(t *testing.T) {
	const key = "SOPS_PUBLISH_TEST_TOKEN"

	testCases := []struct {
		name      string
		prior     string
		published string
		wantPrior bool
	}{
		{
			name:      "venv value displaces the process value",
			prior:     "process-credential",
			published: "venv-credential",
			wantPrior: true,
		},
		{
			name:      "blank displaces the process value",
			prior:     "session-token-the-process-started-with",
			published: "",
			wantPrior: true,
		},
		{
			name:      "venv value fills a name the process lacks",
			published: "venv-credential",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.wantPrior {
				t.Setenv(key, tc.prior)
			} else {
				require.NoError(t, os.Unsetenv(key))
			}

			restore := vsops.PublishEnv(map[string]string{key: tc.published})
			published, publishedSet := os.LookupEnv(key)

			restore()

			restored, restoredSet := os.LookupEnv(key)

			assert.True(t, publishedSet, "%q has to be set inside the window", key)
			assert.Equal(t, tc.published, published, "the venv value has to win inside the window")
			assert.Equal(t, tc.wantPrior, restoredSet, "presence of %q changed across the window", key)
			assert.Equal(t, tc.prior, restored)
		})
	}
}
