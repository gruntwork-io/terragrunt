package cas_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/cas"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/test/helpers"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// stubbedOCIFetch returns a cas.OCIFetchFunc whose extracted directory is
// populated from files, mimicking a real OCI artifact download.
func stubbedOCIFetch(files map[string]string, digest string) cas.OCIFetchFunc {
	return func(_ context.Context, _ log.Logger, fs vfs.FS, _ string) (string, string, func(), error) {
		tmpDir, err := vfs.MkdirTemp(fs, "", "oci-stub-")
		if err != nil {
			return "", "", nil, err
		}

		cleanup := func() { _ = fs.RemoveAll(tmpDir) }

		for name, content := range files {
			path := filepath.Join(tmpDir, name)
			if err := fs.MkdirAll(filepath.Dir(path), 0o755); err != nil {
				cleanup()
				return "", "", nil, err
			}

			f, err := fs.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
			if err != nil {
				cleanup()
				return "", "", nil, err
			}

			if _, err := fmt.Fprint(f, content); err != nil {
				_ = f.Close()
				cleanup()
				return "", "", nil, err
			}

			if err := f.Close(); err != nil {
				cleanup()
				return "", "", nil, err
			}
		}

		return tmpDir, digest, cleanup, nil
	}
}

// ---------------------------------------------------------------------------
// processOCIStackComponent — basic extraction
// ---------------------------------------------------------------------------

func TestProcessStackComponent_OCISourceExtractsFiles(t *testing.T) {
	t.Parallel()

	storePath := filepath.Join(helpers.TmpDirWOSymlinks(t), "cas")
	c, err := cas.New(
		cas.WithStorePath(storePath),
		cas.WithOCIFetch(stubbedOCIFetch(map[string]string{
			"main.tf": `resource "null_resource" "a" {}`,
		}, "sha256:deadbeef00000000000000000000000000000000000000000000000000000000")),
	)
	require.NoError(t, err)

	v, err := cas.OSVenv()
	require.NoError(t, err)

	l := logger.CreateLogger()

	result, err := c.ProcessStackComponent(
		context.Background(), l, v,
		"oci://ghcr.io/org/module:v1.0.0",
		"unit",
	)
	require.NoError(t, err)

	defer result.Cleanup()

	require.FileExists(t, filepath.Join(result.ContentDir, "main.tf"))
}

// ---------------------------------------------------------------------------
// processOCIStackComponent — subdir selector
// ---------------------------------------------------------------------------

func TestProcessStackComponent_OCISourceSubdir(t *testing.T) {
	t.Parallel()

	digest := "sha256:" + fmt.Sprintf("%064x", 1) //nolint:mnd
	storePath := filepath.Join(helpers.TmpDirWOSymlinks(t), "cas")
	c, err := cas.New(
		cas.WithStorePath(storePath),
		cas.WithOCIFetch(stubbedOCIFetch(map[string]string{
			"modules/vpc/main.tf":   "# vpc",
			"modules/other/main.tf": "# other",
		}, digest)),
	)
	require.NoError(t, err)

	v, err := cas.OSVenv()
	require.NoError(t, err)

	result, err := c.ProcessStackComponent(
		context.Background(), logger.CreateLogger(), v,
		"oci://ghcr.io/org/modules:v1.0.0//modules/vpc",
		"unit",
	)
	require.NoError(t, err)

	defer result.Cleanup()

	require.FileExists(t, filepath.Join(result.ContentDir, "main.tf"))
	require.NoFileExists(t, filepath.Join(result.ContentDir, "modules", "other", "main.tf"),
		"sibling directories must not be present when a subdir is selected")
}

// ---------------------------------------------------------------------------
// processOCIStackComponent — missing OCIFetch returns clear error
// ---------------------------------------------------------------------------

func TestProcessStackComponent_OCISourceWithoutFetchFuncFails(t *testing.T) {
	t.Parallel()

	storePath := filepath.Join(helpers.TmpDirWOSymlinks(t), "cas")
	c, err := cas.New(cas.WithStorePath(storePath)) // no WithOCIFetch
	require.NoError(t, err)

	v, err := cas.OSVenv()
	require.NoError(t, err)

	_, err = c.ProcessStackComponent(
		context.Background(), logger.CreateLogger(), v,
		"oci://ghcr.io/org/module:v1.0.0",
		"unit",
	)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "WithOCIFetch")
}

// ---------------------------------------------------------------------------
// processOCIStackComponent — deterministic: same digest → same synth tree
// ---------------------------------------------------------------------------

func TestProcessStackComponent_OCIDeterministicOutput(t *testing.T) {
	t.Parallel()

	digest := "sha256:aabbccdd00000000000000000000000000000000000000000000000000000000"

	files := map[string]string{
		"main.tf": `resource "null_resource" "a" {}`,
	}

	// Both runs share the same CAS store — the second should not fail even
	// though the synthetic tree was already written by the first.
	storePath := filepath.Join(helpers.TmpDirWOSymlinks(t), "cas")

	for i := range 2 {
		c, err := cas.New(
			cas.WithStorePath(storePath),
			cas.WithOCIFetch(stubbedOCIFetch(files, digest)),
		)
		require.NoError(t, err)

		v, err := cas.OSVenv()
		require.NoError(t, err)

		result, err := c.ProcessStackComponent(
			context.Background(), logger.CreateLogger(), v,
			"oci://ghcr.io/org/module:v1.0.0",
			"unit",
		)
		require.NoError(t, err, "run %d", i)

		require.FileExists(t, filepath.Join(result.ContentDir, "main.tf"), "run %d", i)
		result.Cleanup()
	}
}
