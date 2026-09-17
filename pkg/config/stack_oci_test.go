package config_test

import (
	"bytes"
	"errors"
	"path/filepath"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/experiment"
	"github.com/gruntwork-io/terragrunt/internal/getter"
	"github.com/gruntwork-io/terragrunt/internal/strict/controls"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/gruntwork-io/terragrunt/internal/worker"
	"github.com/gruntwork-io/terragrunt/pkg/config"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/gruntwork-io/terragrunt/test/helpers/venvtest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The virtual paths and registry the in-memory stack fixtures use.
const (
	ociTestStackDir = "/virtual/stack"
	ociTestHome     = "/virtual/home"
	ociTestRegistry = "registry.example.com"
)

// ociStackFixture writes a terragrunt.stack.hcl whose single unit or stack is served from an oci:// registry.
func ociStackFixture(t *testing.T, fsys vfs.FS, kind, source string) string {
	t.Helper()

	body := kind + ` "vpc" {
  source = "` + source + `"
  path   = "vpc"
}
`

	stackPath := filepath.Join(ociTestStackDir, "terragrunt.stack.hcl")
	require.NoError(t, vfs.WriteFile(fsys, stackPath, []byte(body), 0o644))

	return stackPath
}

// generateOCIStack runs stack generation for an oci:// component against an unreachable registry.
func generateOCIStack(t *testing.T, kind, sourceScheme string) (string, error) {
	t.Helper()

	// An in-memory venv: no disk, and a fail-closed client so no fetch can leave the test.
	v := venvtest.New().
		WithEnv(map[string]string{"HOME": ociTestHome}).
		WithUserHomeDir(func() (string, error) { return ociTestHome, nil })

	source := sourceScheme + ociTestRegistry + "/terraform-modules/vpc?tag=1.0.0"
	stackPath := ociStackFixture(t, v.FS, kind, source)

	var logBuf bytes.Buffer

	l := logger.CreateLogger()
	l.SetOptions(log.WithOutput(&logBuf), log.WithLevel(log.DebugLevel))

	_, pctx := config.NewParsingContext(t.Context(), l, v, config.WithStrictControls(controls.New()))
	pctx.TerragruntStackConfigPath = stackPath
	pctx.RootWorkingDir = filepath.Dir(stackPath)
	pctx.WorkingDir = filepath.Dir(stackPath)
	pctx.TerragruntConfigPath = stackPath
	// CAS is on by default, so this also covers that an oci:// source bypasses it.
	pctx.CASCloneDepth = 1
	// Defaults only: the oci experiment is completed, so nothing is enabled here.
	pctx.Experiments = experiment.NewExperiments()

	pool := worker.NewWorkerPool(1)
	pool.Start()

	defer pool.Stop()

	// Components are generated on the pool, so the fetch error surfaces from Wait.
	genErr := config.GenerateStackFile(t.Context(), l, pctx, pool, stackPath)

	// Drain the pool before reading the buffer the workers log into.
	waitErr := pool.Wait()

	return logBuf.String(), errors.Join(genErr, waitErr)
}

// requireReachedOCIGetter fails unless the source was routed to the OCI getter.
func requireReachedOCIGetter(t *testing.T, err error, msg string) {
	t.Helper()

	var resolutionErr getter.OCIReferenceResolutionError
	require.ErrorAs(t, err, &resolutionErr, msg)
}

// TestGenerateStackOCIReachesGetter: an oci:// component reaches the OCI getter with stock defaults.
func TestGenerateStackOCIReachesGetter(t *testing.T) {
	t.Parallel()

	for _, kind := range []string{"unit", "stack"} {
		t.Run(kind, func(t *testing.T) {
			t.Parallel()

			logs, err := generateOCIStack(t, kind, "oci://")
			require.Error(t, err, "the venv's fail-closed HTTP client rejects every fetch")

			requireReachedOCIGetter(t, err,
				"the oci getter must run without any experiment being enabled")
			assert.NotContains(t, logs, "CAS processing failed",
				"an oci:// source must bypass the git-backed CAS path, not fail through it")
		})
	}
}

// TestGenerateStackOCIForcedFormReachesGetter: go-getter's oci:: forced form routes to the OCI getter.
func TestGenerateStackOCIForcedFormReachesGetter(t *testing.T) {
	t.Parallel()

	_, err := generateOCIStack(t, "unit", "oci::https://")
	require.Error(t, err)

	requireReachedOCIGetter(t, err, "the oci:: forced form must route to the oci getter")
}

// TestGenerateStackOCIUpperCaseSchemeReachesGetter: an upper-case scheme still routes to the OCI getter.
func TestGenerateStackOCIUpperCaseSchemeReachesGetter(t *testing.T) {
	t.Parallel()

	// go-getter matches the forced token exactly, so only the URL scheme folds.
	for _, scheme := range []string{"OCI://"} {
		t.Run(scheme, func(t *testing.T) {
			t.Parallel()

			_, err := generateOCIStack(t, "unit", scheme)
			require.Error(t, err)

			requireReachedOCIGetter(t, err, "an upper-case oci scheme must route to the oci getter")
		})
	}
}

// TestGenerateStackOCIUpperCaseForcedFormNotClaimed: OCI:: cannot dispatch, so the oci getter must not claim it.
func TestGenerateStackOCIUpperCaseForcedFormNotClaimed(t *testing.T) {
	t.Parallel()

	_, err := generateOCIStack(t, "unit", "OCI::https://")
	require.Error(t, err)

	var resolutionErr getter.OCIReferenceResolutionError
	assert.NotErrorAs(t, err, &resolutionErr,
		"an upper-case forced token is not an oci source, so the oci getter must not claim it")
}
