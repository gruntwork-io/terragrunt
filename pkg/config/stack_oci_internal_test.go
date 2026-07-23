package config

// Tests for the copyFiles helper when the source is an oci:// URL.
// These live in the internal test package so they can access copyFiles directly.

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/stretchr/testify/require"
)

// tmpDirWOSymlinks returns a temp directory with symlinks resolved so that
// filepath comparisons work correctly on macOS (where /tmp → /private/tmp).
func tmpDirWOSymlinks(t *testing.T) string {
	t.Helper()

	tmp := t.TempDir()
	resolved, err := filepath.EvalSymlinks(tmp)
	require.NoError(t, err)

	return resolved
}

// newOCIStackTestServer starts a minimal OCI Distribution Spec v2 server and
// returns its address plus a cleanup function. The server serves a single
// artifact built from the given files map.
//
// This mirrors the ociTestServer helpers in internal/getter but avoids an
// import cycle (getter_test ↔ config_test).

func TestCopyFiles_OCISource_ExtractsFiles(t *testing.T) {
	t.Parallel()

	srv, _ := newConfigOCITestServer(t, map[string]string{
		"main.tf":      `resource "null_resource" "a" {}`,
		"variables.tf": "variable \"name\" {}",
	})

	ref := srv.Listener.Addr().String() + "/org/module:v1.0.0"
	dst := tmpDirWOSymlinks(t)
	fs := vfs.NewOSFS()
	l := logger.CreateLogger()

	err := copyFiles(context.Background(), l, fs, "test-unit", t.TempDir(), "oci://"+ref, dst)
	require.NoError(t, err)
	require.FileExists(t, filepath.Join(dst, "main.tf"))
	require.FileExists(t, filepath.Join(dst, "variables.tf"))
}

func TestCopyFiles_OCISource_ExtractsSubdir(t *testing.T) {
	t.Parallel()

	srv, _ := newConfigOCITestServer(t, map[string]string{
		"modules/vpc/main.tf":   "# vpc module",
		"modules/other/main.tf": "# other module",
	})

	ref := srv.Listener.Addr().String() + "/org/modules:v1.0.0"
	dst := tmpDirWOSymlinks(t)
	fs := vfs.NewOSFS()
	l := logger.CreateLogger()

	err := copyFiles(context.Background(), l, fs, "test-unit", t.TempDir(), "oci://"+ref+"//modules/vpc", dst)
	require.NoError(t, err)
	require.FileExists(t, filepath.Join(dst, "main.tf"))
	require.NoFileExists(t, filepath.Join(dst, "modules", "other", "main.tf"),
		"sibling directories must not be present when a subdir is selected")
}
