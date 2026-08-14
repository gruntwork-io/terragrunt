//go:build aws || gcp

package test_test

import (
	"io/fs"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
)

// cloudGetterLayout is the bucket contents the S3 and GCS mode cases read.
// `modules/vpc` is an object and a prefix at once, and `modules/vpc-old` is a
// sibling whose name starts with it, which is the pair that separates file
// mode from directory mode.
var cloudGetterLayout = map[string]string{
	"modules/vpc":               "vpc-object",
	"modules/vpc/":              "",
	"modules/vpc/main.tf":       "main",
	"modules/vpc/sub/nested.tf": "nested",
	"modules/vpc-old/main.tf":   "old",
	"modules/app/main.tf":       "app-main",
	"modules/app-old/main.tf":   "app-old",
}

// assertCloudDownloadTree compares everything under dst against want, which is
// keyed by slash-separated path relative to dst. Comparing the whole tree
// catches a sibling object or a directory placeholder that landed alongside
// the requested ones.
func assertCloudDownloadTree(t *testing.T, v *venv.Venv, dst string, want map[string]string) {
	t.Helper()

	got := map[string]string{}

	require.NoError(t, vfs.WalkDir(v.FS, dst, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}

		content, err := vfs.ReadFile(v.FS, path)
		if err != nil {
			return err
		}

		rel, err := filepath.Rel(dst, path)
		if err != nil {
			return err
		}

		got[filepath.ToSlash(rel)] = string(content)

		return nil
	}))

	assert.Equal(t, want, got)
}
