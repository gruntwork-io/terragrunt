package config_test

import (
	"path/filepath"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/getter"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
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
func generateOCIStack(t *testing.T, kind, sourceScheme string) error {
	t.Helper()

	// An in-memory venv: no disk, and a fail-closed client so no fetch can leave the test.
	v := venvtest.New().
		WithEnv(map[string]string{"HOME": ociTestHome}).
		WithUserHomeDir(func() (string, error) { return ociTestHome, nil })

	source := sourceScheme + ociTestRegistry + "/terraform-modules/vpc?tag=1.0.0"
	stackPath := ociStackFixture(t, v.FS, kind, source)

	// The in-memory filesystem leaves the CAS disabled; only the routing to the oci getter is covered.
	return generateCatalogStack(t, v, stackPath)
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

			err := generateOCIStack(t, kind, "oci://")
			require.Error(t, err, "the venv's fail-closed HTTP client rejects every fetch")

			requireReachedOCIGetter(t, err,
				"the oci getter must run without any experiment being enabled")
		})
	}
}

// TestGenerateStackOCIForcedFormReachesGetter: go-getter's oci:: forced form routes to the OCI getter.
func TestGenerateStackOCIForcedFormReachesGetter(t *testing.T) {
	t.Parallel()

	err := generateOCIStack(t, "unit", "oci::https://")
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

			err := generateOCIStack(t, "unit", scheme)
			require.Error(t, err)

			requireReachedOCIGetter(t, err, "an upper-case oci scheme must route to the oci getter")
		})
	}
}

// TestGenerateStackOCIUpperCaseForcedFormNotClaimed: OCI:: cannot dispatch, so the oci getter must not claim it.
func TestGenerateStackOCIUpperCaseForcedFormNotClaimed(t *testing.T) {
	t.Parallel()

	err := generateOCIStack(t, "unit", "OCI::https://")
	require.Error(t, err)

	var resolutionErr getter.OCIReferenceResolutionError
	assert.NotErrorAs(t, err, &resolutionErr,
		"an upper-case forced token is not an oci source, so the oci getter must not claim it")
}
