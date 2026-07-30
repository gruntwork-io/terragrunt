package config_test

import (
	"bytes"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/experiment"
	"github.com/gruntwork-io/terragrunt/internal/getter"
	"github.com/gruntwork-io/terragrunt/internal/strict/controls"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/internal/worker"
	"github.com/gruntwork-io/terragrunt/pkg/config"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ociStackFixture writes a terragrunt.stack.hcl whose single unit or stack is served from an oci:// registry.
func ociStackFixture(t *testing.T, kind, source string) string {
	t.Helper()

	dir := t.TempDir()
	body := kind + ` "vpc" {
  source = "` + source + `"
  path   = "vpc"
}
`

	stackPath := filepath.Join(dir, "terragrunt.stack.hcl")
	require.NoError(t, os.WriteFile(stackPath, []byte(body), 0o644))

	return stackPath
}

// generateOCIStack runs stack generation for an oci:// component against a registry the test owns.
func generateOCIStack(t *testing.T, kind, sourceScheme string, ociEnabled bool) (string, error) {
	t.Helper()

	// A TLS server the test owns, whose self-signed cert the client always rejects.
	registry := httptest.NewTLSServer(http.NotFoundHandler())
	t.Cleanup(registry.Close)

	source := sourceScheme + registry.Listener.Addr().String() + "/terraform-modules/vpc?tag=1.0.0"
	stackPath := ociStackFixture(t, kind, source)

	// Hermetic home so a developer's own credentials cannot influence the result.
	hermeticHome := t.TempDir()
	v := venv.OSVenv().
		WithEnv(map[string]string{"HOME": hermeticHome}).
		WithUserHomeDir(func() (string, error) { return hermeticHome, nil })

	var logBuf bytes.Buffer

	l := logger.CreateLogger()
	l.SetOptions(log.WithOutput(&logBuf), log.WithLevel(log.DebugLevel))

	_, pctx := config.NewParsingContext(t.Context(), l, config.WithStrictControls(controls.New()))
	pctx.Venv = v
	pctx.TerragruntStackConfigPath = stackPath
	pctx.RootWorkingDir = filepath.Dir(stackPath)
	pctx.WorkingDir = filepath.Dir(stackPath)
	pctx.TerragruntConfigPath = stackPath
	// CAS is on by default, so this also covers that an oci:// source bypasses it.
	pctx.CASCloneDepth = 1
	pctx.Experiments = experiment.NewExperiments()

	if ociEnabled {
		require.NoError(t, pctx.Experiments.EnableExperiment(experiment.OCI))
	}

	pool := worker.NewWorkerPool(1)
	pool.Start()

	defer pool.Stop()

	// Components are generated on the pool, so the fetch error surfaces from Wait.
	genErr := config.GenerateStackFile(t.Context(), l, pctx, pool, stackPath)

	// Drain the pool before reading the buffer the workers log into.
	waitErr := pool.Wait()

	return logBuf.String(), errors.Join(genErr, waitErr)
}

// TestGenerateStackOCIRequiresExperiment: an oci:// component is rejected up front without the experiment.
func TestGenerateStackOCIRequiresExperiment(t *testing.T) {
	t.Parallel()

	for _, kind := range []string{"unit", "stack"} {
		t.Run(kind, func(t *testing.T) {
			t.Parallel()

			_, err := generateOCIStack(t, kind, "oci://", false)
			require.Error(t, err, "an oci:// source must not be fetched without the experiment")

			var resolutionErr getter.OCIReferenceResolutionError
			assert.NotErrorAs(
				t,
				err,
				&resolutionErr,
				"the oci getter must not run when the experiment is off",
			)

			var gateErr config.OCIExperimentRequiredError
			require.ErrorAs(t, err, &gateErr, "the gate must surface a typed error")
			assert.Equal(t, kind, gateErr.Kind)
		})
	}
}

// TestGenerateStackOCIReachesGetter: with the experiment on, an oci:// component reaches the real OCI getter.
func TestGenerateStackOCIReachesGetter(t *testing.T) {
	t.Parallel()

	for _, kind := range []string{"unit", "stack"} {
		t.Run(kind, func(t *testing.T) {
			t.Parallel()

			logs, err := generateOCIStack(t, kind, "oci://", true)
			require.Error(t, err, "the fake registry's cert is untrusted, so every fetch fails")

			var resolutionErr getter.OCIReferenceResolutionError
			require.ErrorAs(
				t,
				err,
				&resolutionErr,
				"the oci getter must run and surface a typed OCI error when the experiment is on",
			)
			assert.NotContains(t, logs, "CAS processing failed",
				"an oci:// source must bypass the git-backed CAS path, not fail through it")
		})
	}
}

// TestGenerateStackOCIForcedFormGated: go-getter's oci:: forced form is gated by the experiment too.
func TestGenerateStackOCIForcedFormGated(t *testing.T) {
	t.Parallel()

	_, err := generateOCIStack(t, "unit", "oci::https://", false)
	require.Error(t, err, "the oci:: forced form must not bypass the experiment gate")

	var gateErr config.OCIExperimentRequiredError
	require.ErrorAs(t, err, &gateErr, "the forced form must hit the same typed gate")
}

// TestGenerateStackOCIUpperCaseForcedFormNotClaimed: OCI:: cannot dispatch, so the gate must not claim it.
func TestGenerateStackOCIUpperCaseForcedFormNotClaimed(t *testing.T) {
	t.Parallel()

	_, err := generateOCIStack(t, "unit", "OCI::https://", false)
	require.Error(t, err)

	var gateErr config.OCIExperimentRequiredError
	assert.NotErrorAs(t, err, &gateErr,
		"an upper-case forced token is not an oci source, so the gate must not claim it")
}

// TestGenerateStackOCIUpperCaseSchemeGated: an upper-case scheme must not slip past the experiment gate.
func TestGenerateStackOCIUpperCaseSchemeGated(t *testing.T) {
	t.Parallel()

	// go-getter matches the forced token exactly, so only the URL scheme folds.
	tc := []string{"OCI://"}

	for _, scheme := range tc {
		t.Run(scheme, func(t *testing.T) {
			t.Parallel()

			_, err := generateOCIStack(t, "unit", scheme, false)
			require.Error(t, err, "an upper-case oci scheme must not bypass the experiment gate")

			var gateErr config.OCIExperimentRequiredError
			require.ErrorAs(t, err, &gateErr, "the upper-case form must hit the same typed gate")
		})
	}
}
