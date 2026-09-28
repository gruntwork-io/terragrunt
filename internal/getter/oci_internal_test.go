package getter

import (
	"archive/zip"
	"bytes"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
)

// TestOCIGetterExtractModuleOnMemFS pins that module extraction, including subdir promotion, never leaves g.FS.
func TestOCIGetterExtractModuleOnMemFS(t *testing.T) {
	t.Parallel()

	files := map[string]string{
		"main.tf":             `output "root" {}`,
		"modules/sub/main.tf": `output "sub" {}`,
	}

	testCases := []struct {
		name        string
		subDir      string
		wantPresent []string
		wantAbsent  []string
	}{
		{
			name:        "whole tree",
			subDir:      "",
			wantPresent: []string{"main.tf", filepath.Join("modules", "sub", "main.tf")},
		},
		{
			name:        "subdir promotes one subtree",
			subDir:      "modules/sub",
			wantPresent: []string{"main.tf"},
			wantAbsent:  []string{filepath.Join("modules", "sub", "main.tf")},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			g := &OCIGetter{Logger: logger.CreateLogger(), FS: vfs.NewMemMapFS()}

			zipPath := "/blobs/module.zip"
			require.NoError(t, vfs.WriteFile(g.FS, zipPath, zipBytesFor(t, files), 0o600))

			dst := "/work/module"
			require.NoError(t, g.extractModule(zipPath, tc.subDir, dst, "oci://registry.opentofu.org/x", 0))

			for _, name := range tc.wantPresent {
				exists, err := vfs.FileExists(g.FS, filepath.Join(dst, name))
				require.NoError(t, err)
				require.True(t, exists, "expected %s on the getter's filesystem", name)
			}

			for _, name := range tc.wantAbsent {
				exists, err := vfs.FileExists(g.FS, filepath.Join(dst, name))
				require.NoError(t, err)
				require.False(t, exists, "expected %s to stay out of the destination", name)
			}
		})
	}
}

// TestOCIGetterExtractModuleMissingSubdirOnMemFS surfaces a typed error without touching the OS filesystem.
func TestOCIGetterExtractModuleMissingSubdirOnMemFS(t *testing.T) {
	t.Parallel()

	g := &OCIGetter{Logger: logger.CreateLogger(), FS: vfs.NewMemMapFS()}

	zipPath := "/blobs/module.zip"
	zipBytes := zipBytesFor(t, map[string]string{"main.tf": `output "root" {}`})
	require.NoError(t, vfs.WriteFile(g.FS, zipPath, zipBytes, 0o600))

	err := g.extractModule(zipPath, "modules/absent", "/work/module", "oci://registry.opentofu.org/x", 0)

	var downloadErr ModuleDownloadErr
	require.ErrorAs(t, err, &downloadErr, "a missing subdir must surface the typed download error")
}

// zipBytesFor builds an in-memory module zip holding files keyed by relative path.
func zipBytesFor(t *testing.T, files map[string]string) []byte {
	t.Helper()

	var buf bytes.Buffer

	zw := zip.NewWriter(&buf)

	for name, body := range files {
		w, err := zw.Create(name)
		require.NoError(t, err)

		_, err = w.Write([]byte(body))
		require.NoError(t, err)
	}

	require.NoError(t, zw.Close())

	return buf.Bytes()
}
