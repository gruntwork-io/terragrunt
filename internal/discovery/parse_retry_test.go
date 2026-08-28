package discovery_test

import (
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gruntwork-io/terragrunt/internal/component"
	"github.com/gruntwork-io/terragrunt/internal/discovery"
	"github.com/gruntwork-io/terragrunt/internal/errorconfig"
	"github.com/gruntwork-io/terragrunt/internal/experiment"
	"github.com/gruntwork-io/terragrunt/internal/vsops"
	"github.com/gruntwork-io/terragrunt/pkg/options"
	"github.com/gruntwork-io/terragrunt/test/helpers"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/gruntwork-io/terragrunt/test/helpers/venvtest"
)

// TestDiscovery_RetryParseErrorsRecoversTransientSops pins the discovery half
// of issue #6755: with the retry-parse-errors experiment on, a transient
// sops_decrypt_file failure during the discovery parse is retried instead of
// aborting the walk; without it the first failure still surfaces.
func TestDiscovery_RetryParseErrorsRecoversTransientSops(t *testing.T) {
	t.Parallel()

	transientErrors := func() *errorconfig.Config {
		return &errorconfig.Config{
			Retry: map[string]*errorconfig.RetryConfig{
				"transient": {
					Name:            "transient",
					RetryableErrors: []*errorconfig.Pattern{{Pattern: regexp.MustCompile(`(?s).*i/o timeout.*`)}},
					MaxAttempts:     3,
				},
			},
		}
	}

	writeUnit := func(t *testing.T) string {
		t.Helper()

		tmpDir := helpers.TmpDirWOSymlinks(t)
		appDir := filepath.Join(tmpDir, "app")
		require.NoError(t, os.MkdirAll(appDir, 0755))
		require.NoError(t, os.WriteFile(
			filepath.Join(appDir, "terragrunt.hcl"),
			[]byte(`locals {
  secret = sops_decrypt_file("secret.enc.json")
}`),
			0644,
		))

		return tmpDir
	}

	flakySops := func(failures int) (vsops.Decrypter, *atomic.Int32) {
		var calls atomic.Int32

		return vsops.NewMemDecrypter(func(map[string]string, string, string) ([]byte, error) {
			if int(calls.Add(1)) <= failures {
				return nil, errors.New("dial tcp 1.2.3.4:443: i/o timeout")
			}

			return []byte(`{"value":"ok"}`), nil
		}), &calls
	}

	t.Run("experiment on retries the discovery parse", func(t *testing.T) {
		t.Parallel()

		tmpDir := writeUnit(t)
		decrypter, calls := flakySops(1)

		opts, err := options.NewTerragruntOptionsForTest(filepath.Join(tmpDir, "terragrunt.hcl"))
		require.NoError(t, err)

		opts.WorkingDir = tmpDir
		opts.RootWorkingDir = tmpDir
		opts.Errors = transientErrors()
		require.NoError(t, opts.Experiments.EnableExperiment(experiment.RetryParseErrors))

		d := discovery.NewDiscovery(tmpDir).
			WithDiscoveryContext(&component.DiscoveryContext{WorkingDir: tmpDir}).
			WithRelationships()

		v := venvtest.NewOSWithEmptyEnv().WithSops(decrypter)

		components, err := d.Discover(t.Context(), logger.CreateLogger(), v, opts)
		require.NoError(t, err, "the transient decrypt failure is retried away")
		assert.Len(t, components, 1)
		assert.EqualValues(t, 2, calls.Load(), "one failed decrypt, then the one that succeeds")
	})

	t.Run("experiment off fails discovery on the first parse error", func(t *testing.T) {
		t.Parallel()

		tmpDir := writeUnit(t)
		decrypter, calls := flakySops(1)

		opts, err := options.NewTerragruntOptionsForTest(filepath.Join(tmpDir, "terragrunt.hcl"))
		require.NoError(t, err)

		opts.WorkingDir = tmpDir
		opts.RootWorkingDir = tmpDir
		opts.Errors = transientErrors()

		d := discovery.NewDiscovery(tmpDir).
			WithDiscoveryContext(&component.DiscoveryContext{WorkingDir: tmpDir}).
			WithRelationships()

		_, err = d.Discover(t.Context(), logger.CreateLogger(), venvtest.NewOSWithEmptyEnv().WithSops(decrypter), opts)
		require.Error(t, err, "without the experiment the parse failure surfaces")
		assert.EqualValues(t, 1, calls.Load(), "the discovery parse is not retried")
	})
}
