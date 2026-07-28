package config_test

import (
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
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ociStackFixture writes a terragrunt.stack.hcl whose single component of the
// given kind ("unit" or "stack") is served from an oci:// registry.
func ociStackFixture(t *testing.T, kind, registryAddr string) string {
	t.Helper()

	dir := t.TempDir()
	body := kind + ` "vpc" {
  source = "oci://` + registryAddr + `/terraform-modules/vpc?tag=1.0.0"
  path   = "vpc"
}
`

	stackPath := filepath.Join(dir, "terragrunt.stack.hcl")
	require.NoError(t, os.WriteFile(stackPath, []byte(body), 0o644))

	return stackPath
}

// generateOCIStack runs stack generation for an oci:// component, with the oci
// experiment on or off, against a registry the test owns.
func generateOCIStack(t *testing.T, kind string, ociEnabled bool) error {
	t.Helper()

	// A TLS server the test owns: the client rejects its self-signed cert
	// deterministically, so the fetch fails without reaching the network.
	registry := httptest.NewTLSServer(http.NotFoundHandler())
	t.Cleanup(registry.Close)

	stackPath := ociStackFixture(t, kind, registry.Listener.Addr().String())

	// Hermetic home so a developer's Docker or tofu credentials cannot
	// influence how the source authenticates.
	hermeticHome := t.TempDir()
	v := venv.OSVenv().
		WithEnv(map[string]string{"HOME": hermeticHome}).
		WithUserHomeDir(func() (string, error) { return hermeticHome, nil })

	l := logger.CreateLogger()

	_, pctx := config.NewParsingContext(t.Context(), l, config.WithStrictControls(controls.New()))
	pctx.Venv = v
	pctx.TerragruntStackConfigPath = stackPath
	pctx.RootWorkingDir = filepath.Dir(stackPath)
	pctx.WorkingDir = filepath.Dir(stackPath)
	pctx.TerragruntConfigPath = stackPath
	// CAS is on by default, so this also covers that an oci:// source bypasses
	// the git-backed CAS path instead of failing through it.
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

	return errors.Join(genErr, pool.Wait())
}

// TestGenerateStackOCIRequiresExperiment: an oci:// component is rejected up front without the experiment.
func TestGenerateStackOCIRequiresExperiment(t *testing.T) {
	t.Parallel()

	for _, kind := range []string{"unit", "stack"} {
		t.Run(kind, func(t *testing.T) {
			t.Parallel()

			err := generateOCIStack(t, kind, false)
			require.Error(t, err, "an oci:// source must not be fetched without the experiment")

			var resolutionErr getter.OCIReferenceResolutionError
			assert.NotErrorAs(
				t,
				err,
				&resolutionErr,
				"the oci getter must not run when the experiment is off",
			)
			assert.ErrorContains(t, err, "oci experiment")
		})
	}
}

// TestGenerateStackOCIReachesGetter: with the experiment on, an oci:// component reaches the real OCI getter.
func TestGenerateStackOCIReachesGetter(t *testing.T) {
	t.Parallel()

	for _, kind := range []string{"unit", "stack"} {
		t.Run(kind, func(t *testing.T) {
			t.Parallel()

			err := generateOCIStack(t, kind, true)
			require.Error(t, err, "the fake registry's cert is untrusted, so every fetch fails")

			var resolutionErr getter.OCIReferenceResolutionError
			require.ErrorAs(
				t,
				err,
				&resolutionErr,
				"the oci getter must run and surface a typed OCI error when the experiment is on",
			)
		})
	}
}
