package tui_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gruntwork-io/terragrunt/internal/cli/commands/catalog/tui"
	"github.com/gruntwork-io/terragrunt/internal/util"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/gruntwork-io/terragrunt/pkg/options"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestCreateCatalogTempPathResolvesSymlinkedTmpDir verifies the clone dir is
// anchored at a symlink-resolved temp dir. macOS reports os.TempDir() as a
// /var/folders symlink to /private/var/folders; an unresolved root makes
// discovery's filepath.Rel emit a "../" traversal that go-getter rejects.
func TestCreateCatalogTempPathResolvesSymlinkedTmpDir(t *testing.T) {
	// t.Setenv forbids t.Parallel.
	base, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)

	realTmp := filepath.Join(base, "real")
	require.NoError(t, os.Mkdir(realTmp, 0o755))

	// linkTmp -> realTmp stands in for /var/folders -> /private/var/folders.
	linkTmp := filepath.Join(base, "link")
	require.NoError(t, os.Symlink(realTmp, linkTmp))

	t.Setenv("TMPDIR", linkTmp)

	got, err := tui.CreateCatalogTempPath(vfs.NewOSFS(), "github.com/gruntwork-io/terragrunt-scale-catalog")
	require.NoError(t, err)

	// The clone dir must sit directly under the resolved temp dir, with no
	// symlink component left for filepath.Rel to trip over later.
	assert.Equal(t, realTmp, filepath.Dir(got),
		"CreateCatalogTempPath should anchor the clone at the symlink-resolved temp dir")
}

func TestCreateCatalogTempPathUsesFreshDirectory(t *testing.T) {
	t.Parallel()

	fsys := vfs.NewMemMapFS()
	repoURL := "github.com/gruntwork-io/terragrunt-scale-catalog"

	first, err := tui.CreateCatalogTempPath(fsys, repoURL)
	require.NoError(t, err)

	second, err := tui.CreateCatalogTempPath(fsys, repoURL)
	require.NoError(t, err)

	assert.NotEqual(t, first, second)

	expectedPrefix := "catalog-" + util.EncodeBase64Sha1(repoURL) + "-"
	assert.True(t, strings.HasPrefix(filepath.Base(first), expectedPrefix))
	assert.True(t, strings.HasPrefix(filepath.Base(second), expectedPrefix))

	firstExists, err := vfs.FileExists(fsys, first)
	require.NoError(t, err)
	assert.True(t, firstExists, "first temp dir should exist on the vfs")

	secondExists, err := vfs.FileExists(fsys, second)
	require.NoError(t, err)
	assert.True(t, secondExists, "second temp dir should exist on the vfs")
}

func TestLoadURLKeepsTempDirAfterEmittingComponentOnCancel(t *testing.T) {
	// t.Setenv forbids t.Parallel.
	base := t.TempDir()
	tempRoot := filepath.Join(base, "tmp")
	require.NoError(t, os.Mkdir(tempRoot, 0o755))
	t.Setenv("TMPDIR", tempRoot)

	repoDir := filepath.Join(base, "repo")
	writeCatalogRepo(t, repoDir)

	opts, err := options.NewTerragruntOptionsForTest("")
	require.NoError(t, err)

	v := venv.OSVenv()
	tracker := tui.NewTempDirTracker(v.FS)
	ctx, cancel := context.WithCancel(t.Context())
	componentCh := make(chan *tui.ComponentEntry)
	errCh := make(chan error, 1)

	go func() {
		errCh <- tui.LoadURL(ctx, logger.CreateLogger(), v, opts, tracker, repoDir, componentCh)
	}()

	select {
	case <-componentCh:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for first component")
	}

	cancel()

	select {
	case err := <-errCh:
		require.NoError(t, err)
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for LoadURL to return")
	}

	catalogDirs := catalogTempDirs(t, tempRoot)
	require.Len(t, catalogDirs, 1)
	assert.DirExists(t, catalogDirs[0])

	tracker.Cleanup(logger.CreateLogger())
	assert.NoDirExists(t, catalogDirs[0])
}

func writeCatalogRepo(t *testing.T, repoDir string) {
	t.Helper()

	require.NoError(t, os.MkdirAll(filepath.Join(repoDir, ".git"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(repoDir, ".git", "HEAD"), []byte("ref: refs/heads/main\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(repoDir, ".git", "config"), []byte(`[core]
	repositoryformatversion = 0
[remote "origin"]
	url = github.com/gruntwork-io/fake-repo
`), 0o644))

	require.NoError(t, os.MkdirAll(filepath.Join(repoDir, "alpha"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(repoDir, "alpha", "main.tf"), []byte("# alpha\n"), 0o644))
	require.NoError(t, os.MkdirAll(filepath.Join(repoDir, "bravo"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(repoDir, "bravo", "main.tf"), []byte("# bravo\n"), 0o644))
}

func catalogTempDirs(t *testing.T, tempRoot string) []string {
	t.Helper()

	entries, err := os.ReadDir(tempRoot)
	require.NoError(t, err)

	var dirs []string

	for _, entry := range entries {
		if !entry.IsDir() || !strings.HasPrefix(entry.Name(), "catalog-") {
			continue
		}

		dirs = append(dirs, filepath.Join(tempRoot, entry.Name()))
	}

	return dirs
}
