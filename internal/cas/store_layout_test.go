package cas_test

import (
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gruntwork-io/terragrunt/internal/cas"
	"github.com/gruntwork-io/terragrunt/test/helpers"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/gruntwork-io/terragrunt/test/helpers/venvtest"
)

// TestCAS_ColdIngestLeavesOnlyObjects pins the store layout after a cold
// ingest: every object partition holds nothing but the objects
// themselves, with no lock files or leftover temp files beside them.
func TestCAS_ColdIngestLeavesOnlyObjects(t *testing.T) {
	t.Parallel()

	const fileCount = 32

	files := make(map[string][]byte, fileCount)
	for i := range fileCount {
		name := fmt.Sprintf("dir%d/file%d.tf", i%4, i)
		files[name] = fmt.Appendf(nil, "# file %d\n", i)
	}

	srv := newEmptyTestServer(t)
	require.NoError(t, srv.CommitFiles(t.Context(), files, "add files"))

	repoURL, err := srv.Start(t.Context())
	require.NoError(t, err)

	l := logger.CreateLogger()
	v := venvtest.NewOSWithEmptyEnv()

	tempDir := helpers.TmpDirWOSymlinks(t)
	storePath := filepath.Join(tempDir, "store")

	c, err := cas.New(venvtest.NewWithOSFS(), cas.WithStorePath(storePath))
	require.NoError(t, err)

	require.NoError(t, c.Clone(t.Context(), l, v, repoURL,
		cas.WithDir(filepath.Join(tempDir, "repo")),
		cas.WithDepth(-1),
		cas.WithIncludedGitFiles([]string{"HEAD", "config"})))

	blobs := 0

	for _, store := range []*cas.Store{c.BlobStore(), c.TreeStore(), c.SynthStore(), c.GitFileStore()} {
		err := filepath.WalkDir(store.Path(), func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}

			if d.IsDir() {
				return nil
			}

			name := d.Name()
			partition := filepath.Base(filepath.Dir(path))

			assert.Falsef(t, strings.HasSuffix(name, ".lock"), "lock file left in store: %s", path)
			assert.NotContainsf(t, name, ".tmp", "temp file left in store: %s", path)
			assert.Equalf(t, partition, name[:2], "object %s is filed under the wrong partition", path)

			if store == c.BlobStore() {
				blobs++
			}

			return nil
		})
		require.NoError(t, err)
	}

	assert.GreaterOrEqual(t, blobs, fileCount, "every distinct file must be stored as a blob")
}
