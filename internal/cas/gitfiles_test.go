package cas_test

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/sync/errgroup"

	"github.com/gruntwork-io/terragrunt/internal/cas"
	"github.com/gruntwork-io/terragrunt/internal/git"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/test/helpers"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/gruntwork-io/terragrunt/test/helpers/venvtest"
)

// TestCAS_IncludedGitFilesPerCaller pins the tree cache key to the commit
// alone: whichever caller ingests a commit first, every later caller
// against the same store receives exactly the .git files it named, and
// the stored tree never carries a .git entry.
func TestCAS_IncludedGitFilesPerCaller(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		first  []string
		second []string
	}{
		{
			name:   "subset then superset",
			first:  []string{"HEAD"},
			second: []string{"HEAD", "config"},
		},
		{
			name:   "superset then subset",
			first:  []string{"HEAD", "config"},
			second: []string{"HEAD"},
		},
		{
			name:   "disjoint lists",
			first:  []string{"config"},
			second: []string{"HEAD"},
		},
		{
			name:   "empty then non-empty",
			first:  nil,
			second: []string{"HEAD", "config"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			l := logger.CreateLogger()
			v := venvtest.NewOSWithEmptyEnv()
			repoURL := startTestServer(t)
			hash := resolveHead(t, repoURL)

			tempDir := helpers.TmpDirWOSymlinks(t)
			storePath := filepath.Join(tempDir, "store")

			// Each clone runs through its own CAS instance so the store on
			// disk is the only state the callers share, as it is between
			// processes.
			clone := func(name string, files []string) string {
				t.Helper()

				c, err := cas.New(venvtest.NewWithOSFS(), cas.WithStorePath(storePath))
				require.NoError(t, err)

				dir := filepath.Join(tempDir, name)

				require.NoError(t, c.Clone(t.Context(), l, v, repoURL,
					cas.WithDir(dir),
					cas.WithDepth(-1),
					cas.WithIncludedGitFiles(files)))
				require.FileExists(t, filepath.Join(dir, "README.md"))

				return dir
			}

			assertGitDir(t, clone("first", tc.first), tc.first)
			assertGitDir(t, clone("second", tc.second), tc.second)
			assertGitDir(t, clone("none", nil), nil)
			assertGitDir(t, clone("first-again", tc.first), tc.first)

			c, err := cas.New(venvtest.NewWithOSFS(), cas.WithStorePath(storePath))
			require.NoError(t, err)

			assertTreeHasNoGitEntries(t, c, v, hash)
		})
	}
}

// TestCAS_IncludedGitFilesFromFoldedTree covers a store written before
// the .git files moved to records of their own, whose tree for the commit
// carries the list the caller that ingested it asked for. Later callers
// receive the files their own lists name, and the cached commit is not
// thrown away to get there.
func TestCAS_IncludedGitFilesFromFoldedTree(t *testing.T) {
	t.Parallel()

	l := logger.CreateLogger()
	v := venvtest.NewOSWithEmptyEnv()
	repoURL := startTestServer(t)
	hash := resolveHead(t, repoURL)

	tempDir := helpers.TmpDirWOSymlinks(t)
	storePath := filepath.Join(tempDir, "store")

	newStoreCAS := func() *cas.CAS {
		t.Helper()

		c, err := cas.New(venvtest.NewWithOSFS(), cas.WithStorePath(storePath))
		require.NoError(t, err)

		return c
	}

	seed := newStoreCAS()
	require.NoError(t, seed.Clone(t.Context(), l, v, repoURL,
		cas.WithDir(filepath.Join(tempDir, "seed")),
		cas.WithDepth(-1),
		cas.WithIncludedGitFiles([]string{"HEAD", "config"})))

	foldGitFilesIntoTree(t, seed, v, l, hash, []string{"HEAD", "config"})

	clone := func(name string, files []string) string {
		t.Helper()

		dir := filepath.Join(tempDir, name)

		require.NoError(t, newStoreCAS().Clone(t.Context(), l, v, repoURL,
			cas.WithDir(dir),
			cas.WithDepth(-1),
			cas.WithIncludedGitFiles(files)))
		require.FileExists(t, filepath.Join(dir, "README.md"))

		return dir
	}

	assertGitDir(t, clone("none", nil), nil)
	assertGitDir(t, clone("head-only", []string{"HEAD"}), []string{"HEAD"})
	assertGitDir(t, clone("both", []string{"HEAD", "config"}), []string{"HEAD", "config"})
}

// foldGitFilesIntoTree rewrites the tree stored under hash into the shape
// releases before per-caller .git files wrote: each named file appended to
// the tree as a ".git/<name>" entry, with no record of its own left behind.
func foldGitFilesIntoTree(
	t *testing.T,
	c *cas.CAS,
	v *venv.Venv,
	l log.Logger,
	hash string,
	names []string,
) {
	t.Helper()

	records := cas.NewContent(c.GitFileStore())
	trees := cas.NewContent(c.TreeStore())

	treeData, err := trees.Read(v, hash)
	require.NoError(t, err)

	for _, name := range names {
		key := cas.GitFileKey(hash, name)

		record, err := records.Read(v, key)
		require.NoError(t, err)

		folded := strings.Replace(string(record), "\t"+name, "\t.git/"+name, 1)
		require.NotEqual(t, string(record), folded)

		treeData = append(treeData, folded...)

		require.NoError(t, os.Remove(filepath.Join(c.GitFileStore().Path(), key[:2], key)))
	}

	require.NoError(t, trees.Store(l, v, hash, treeData, cas.StoredFilePerms))
}

// TestCAS_IncludedGitFilesConcurrentCallersWithRacing runs callers with
// different lists against one cold store at the same time. Each must
// still receive exactly its own list.
func TestCAS_IncludedGitFilesConcurrentCallersWithRacing(t *testing.T) {
	t.Parallel()

	l := logger.CreateLogger()
	v := venvtest.NewOSWithEmptyEnv()
	repoURL := startTestServer(t)

	tempDir := helpers.TmpDirWOSymlinks(t)
	storePath := filepath.Join(tempDir, "store")

	c, err := cas.New(venvtest.NewWithOSFS(), cas.WithStorePath(storePath))
	require.NoError(t, err)

	lists := [][]string{
		{"HEAD"},
		{"HEAD", "config"},
		nil,
		{"config"},
	}

	dirs := make([]string, len(lists))

	g, ctx := errgroup.WithContext(t.Context())

	for i, files := range lists {
		dirs[i] = filepath.Join(tempDir, "clone-"+strings.Join(files, "-"))

		g.Go(func() error {
			return c.Clone(ctx, l, v, repoURL,
				cas.WithDir(dirs[i]),
				cas.WithDepth(-1),
				cas.WithIncludedGitFiles(files))
		})
	}

	require.NoError(t, g.Wait())

	for i, files := range lists {
		assertGitDir(t, dirs[i], files)
	}
}

// TestCAS_IncludedGitFilesFallbackClone covers the temporary bare clone
// path: with the central git store blocked, the .git files still come
// from the fallback repository, and a later caller with a shorter list
// is served from the store.
func TestCAS_IncludedGitFilesFallbackClone(t *testing.T) {
	t.Parallel()

	l := logger.CreateLogger()
	v := venvtest.NewOSWithEmptyEnv()
	repoURL := startTestServer(t)

	tempDir := helpers.TmpDirWOSymlinks(t)
	storePath := filepath.Join(tempDir, "store")

	gitStoreRoot := filepath.Join(storePath, "git")
	require.NoError(t, os.MkdirAll(gitStoreRoot, 0o755))

	entry := cas.EntryPathForURL(gitStoreRoot, repoURL, cas.HashSHA256)
	require.NoError(t, os.WriteFile(entry, []byte("not a directory"), 0o644))

	c, err := cas.New(venvtest.NewWithOSFS(), cas.WithStorePath(storePath))
	require.NoError(t, err)

	first := filepath.Join(tempDir, "first")
	require.NoError(t, c.Clone(t.Context(), l, v, repoURL,
		cas.WithDir(first),
		cas.WithDepth(-1),
		cas.WithIncludedGitFiles([]string{"HEAD", "config"})))
	assertGitDir(t, first, []string{"HEAD", "config"})

	second := filepath.Join(tempDir, "second")
	require.NoError(t, c.Clone(t.Context(), l, v, repoURL,
		cas.WithDir(second),
		cas.WithDepth(-1),
		cas.WithIncludedGitFiles([]string{"HEAD"})))
	assertGitDir(t, second, []string{"HEAD"})
}

// TestCAS_IncludedGitFilesRejectsDirectory pins the contract that the
// list names files: a directory inside the bare repository fails the
// clone with a typed error instead of being skipped.
func TestCAS_IncludedGitFilesRejectsDirectory(t *testing.T) {
	t.Parallel()

	l := logger.CreateLogger()
	v := venvtest.NewOSWithEmptyEnv()
	repoURL := startTestServer(t)

	tempDir := helpers.TmpDirWOSymlinks(t)

	c, err := cas.New(venvtest.NewWithOSFS(), cas.WithStorePath(filepath.Join(tempDir, "store")))
	require.NoError(t, err)

	err = c.Clone(t.Context(), l, v, repoURL,
		cas.WithDir(filepath.Join(tempDir, "repo")),
		cas.WithDepth(-1),
		cas.WithIncludedGitFiles([]string{"refs"}))
	require.ErrorIs(t, err, cas.ErrIncludedGitFileIsDir)
}

// TestFetchSource_IncludedGitFilesWithoutRecordsFails pins the
// [cas.CAS.FetchSource] contract for fetchers that record no git files:
// asking for one is an error, not a silently empty .git directory.
func TestFetchSource_IncludedGitFilesWithoutRecordsFails(t *testing.T) {
	t.Parallel()

	c, v := newCAS(t)
	l := logger.CreateLogger()

	var fetchCalls atomic.Int32

	fetch := fakeFetcher(c, map[string]string{"main.tf": "content"}, &fetchCalls)

	opts := &cas.CloneOptions{
		Dir:              filepath.Join(t.TempDir(), "dst"),
		IncludedGitFiles: []string{"HEAD"},
	}

	err := c.FetchSource(t.Context(), l, v, opts, cas.SourceRequest{
		Scheme: "http",
		URL:    "https://example.com/mod.tgz",
		Fetch:  fetch,
	})
	require.ErrorIs(t, err, cas.ErrGitFileNotStored)
	assert.Equal(t, int32(1), fetchCalls.Load())
}

// assertGitDir checks that dir/.git holds exactly the named files, each
// with content, and that no .git directory exists when want is empty.
func assertGitDir(t *testing.T, dir string, want []string) {
	t.Helper()

	gitDir := filepath.Join(dir, ".git")

	if len(want) == 0 {
		_, err := os.Stat(gitDir)
		require.ErrorIs(t, err, os.ErrNotExist, "no .git directory expected in %s", dir)

		return
	}

	entries, err := os.ReadDir(gitDir)
	require.NoError(t, err)

	got := make([]string, 0, len(entries))
	for _, entry := range entries {
		got = append(got, entry.Name())
	}

	wantSorted := slices.Clone(want)
	slices.Sort(wantSorted)
	slices.Sort(got)

	assert.Equal(t, wantSorted, got, ".git contents in %s", dir)

	for _, name := range want {
		data, err := os.ReadFile(filepath.Join(gitDir, name))
		require.NoError(t, err)
		assert.NotEmpty(t, data, ".git/%s in %s", name, dir)
	}
}

// assertTreeHasNoGitEntries reads the tree stored under hash and checks
// that ingest left it exactly as git describes it, with no .git entries.
func assertTreeHasNoGitEntries(t *testing.T, c *cas.CAS, v *venv.Venv, hash string) {
	t.Helper()

	data, err := cas.NewContent(c.TreeStore()).Read(v, hash)
	require.NoError(t, err)

	tree, err := git.ParseTree(data, "")
	require.NoError(t, err)
	require.NotEmpty(t, tree.Entries())

	for _, entry := range tree.Entries() {
		assert.False(t, strings.HasPrefix(entry.Path, ".git/"),
			"stored tree %s carries a .git entry: %s", hash, entry.Path)
	}
}
