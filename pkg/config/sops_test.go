//go:build sops

package config //nolint:testpackage // needs access to sopsDecryptFileImpl

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/strict/controls"
	"github.com/gruntwork-io/terragrunt/internal/vsops"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/gruntwork-io/terragrunt/test/helpers/venvtest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// generateTestSecretFiles creates plain JSON files in a temp directory.
// No SOPS encryption needed: the test injects a mock decrypter to read raw files.
func generateTestSecretFiles(t *testing.T, count int) []string {
	t.Helper()

	dir := t.TempDir()

	var files []string

	for i := 1; i <= count; i++ {
		unitDir := filepath.Join(dir, fmt.Sprintf("unit-%02d", i))
		require.NoError(t, os.MkdirAll(unitDir, 0755))

		secretFile := filepath.Join(unitDir, "secret.enc.json")
		require.NoError(t, os.WriteFile(secretFile,
			fmt.Appendf(nil, `{"value":"secret-from-unit-%02d"}`, i), 0644))

		files = append(files, secretFile)
	}

	return files
}

// TestSOPSDecryptEnvPropagation is a deterministic regression test for
// https://github.com/gruntwork-io/terragrunt/issues/5515, where
// sops_decrypt_file() could not authenticate to KMS because the auth provider's
// credentials had not reached the decrypt.
//
// A decrypt runs with the credentials the venv carries, and the config layer
// hands that environment to the decrypter as it stands. Publishing them into
// the process environment for the length of one decrypt belongs to the OS
// decrypter, so internal/vsops covers that window, and
// [TestSOPSDecryptConcurrencyWithRacing] covers units decrypting at once.
func TestSOPSDecryptEnvPropagation(t *testing.T) {
	t.Parallel()

	const authKey = "SOPS_TEST_AUTH_CRED"

	secretFile := generateTestSecretFiles(t, 1)[0]

	authRequiringDecrypter := vsops.NewMemDecrypter(
		func(env map[string]string, path string, _ string) ([]byte, error) {
			if env[authKey] == "" {
				return nil, errors.New("KMS auth failed: no credential set")
			}

			return os.ReadFile(path)
		})

	t.Run("creds_from_venv_reach_the_decrypter", func(t *testing.T) {
		t.Parallel()

		l := logger.CreateLogger()
		ctx := WithConfigValues(t.Context())
		v := venvtest.NewWithOSFS().WithEnv(map[string]string{authKey: "fresh-token"})

		_, pctx := NewParsingContext(ctx, l, v, WithStrictControls(controls.New()))
		pctx.WorkingDir = filepath.Dir(secretFile)

		result, err := sopsDecryptFileImpl(ctx, pctx, l, secretFile, "json", authRequiringDecrypter)
		require.NoError(t, err, "decrypt must succeed with credentials from the venv")
		assert.Contains(t, result, `"value":"secret-from-unit-01"`)
	})

	t.Run("missing_creds_fails_decrypt", func(t *testing.T) {
		t.Parallel()

		l := logger.CreateLogger()
		ctx := WithConfigValues(t.Context())
		v := venvtest.NewWithOSFS().WithEnv(map[string]string{})

		_, pctx := NewParsingContext(ctx, l, v, WithStrictControls(controls.New()))
		pctx.WorkingDir = filepath.Dir(secretFile)

		_, err := sopsDecryptFileImpl(ctx, pctx, l, secretFile, "json", authRequiringDecrypter)
		require.Error(t, err,
			"decrypt must fail without auth credentials, reproducing original issue #5515")
	})
}

// TestSOPSDecryptLeavesProcessEnvAlone pins that a credential the run was
// started with outlives a decrypt that carried its own value for the same name.
func TestSOPSDecryptLeavesProcessEnvAlone(t *testing.T) { //nolint:paralleltest // t.Setenv
	const authKey = "SOPS_TEST_UNTOUCHED_CRED"

	t.Setenv(authKey, "real-ci-token")

	secretFile := generateTestSecretFiles(t, 1)[0]

	d := vsops.NewMemDecrypter(func(_ map[string]string, path string, _ string) ([]byte, error) {
		return os.ReadFile(path)
	})

	l := logger.CreateLogger()
	ctx := WithConfigValues(t.Context())
	v := venvtest.NewWithOSFS().WithEnv(map[string]string{authKey: "venv-token"})

	_, pctx := NewParsingContext(ctx, l, v, WithStrictControls(controls.New()))
	pctx.WorkingDir = filepath.Dir(secretFile)

	_, err := sopsDecryptFileImpl(ctx, pctx, l, secretFile, "json", d)
	require.NoError(t, err)

	assert.Equal(t, "real-ci-token", os.Getenv(authKey))
}
