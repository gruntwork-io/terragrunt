//go:build docker || aws || gcp

package test_test

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"slices"
	"testing"

	"github.com/stretchr/testify/require"
)

// makeModuleArchive packs a minimal terragrunt-friendly module into a
// gzipped tarball. Used by the CAS-over-S3 and CAS-over-GCS
// integration tests so go-getter's archive detection recognizes the
// `.tar.gz` URL and extracts on download. The fixed content lets
// every variant test assert against the same materialized layout.
func makeModuleArchive(t *testing.T) []byte {
	t.Helper()

	files := map[string]string{
		"main.tf":  `resource "null_resource" "test" {}`,
		"vars.tf":  `variable "x" { type = string }`,
		"README":   "module readme",
		"sub/x.tf": "# nested file",
	}

	var buf bytes.Buffer

	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)

	// Sort entry names so the resulting archive bytes are stable across
	// runs. Map iteration order otherwise breaks any caller that pins
	// behavior on the archive checksum.
	names := make([]string, 0, len(files))
	for name := range files {
		names = append(names, name)
	}

	slices.Sort(names)

	for _, name := range names {
		body := files[name]

		require.NoError(t, tw.WriteHeader(&tar.Header{
			Name: name,
			Mode: 0o644,
			Size: int64(len(body)),
		}))

		_, err := tw.Write([]byte(body))
		require.NoError(t, err)
	}

	require.NoError(t, tw.Close())
	require.NoError(t, gz.Close())

	return buf.Bytes()
}
