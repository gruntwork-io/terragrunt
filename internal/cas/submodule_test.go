package cas_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/cas"
	"github.com/gruntwork-io/terragrunt/test/helpers"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// startSubmoduleServer commits the given files to a fresh test server,
// starts it, and returns its URL together with the HEAD commit hash.
func startSubmoduleServer(t *testing.T, files map[string]string) (string, string) {
	t.Helper()

	srv := newEmptyTestServer(t)

	for path, content := range files {
		require.NoError(t, srv.CommitFile(path, []byte(content), "add "+path))
	}

	head, err := srv.Head()
	require.NoError(t, err)

	url, err := srv.Start(t.Context())
	require.NoError(t, err)

	return url, head
}

func TestCAS_CloneRepoWithSubmodule(t *testing.T) {
	t.Parallel()

	subURL, subHead := startSubmoduleServer(t, map[string]string{
		"module.tf":     `# child module`,
		"sub/nested.tf": `# nested file in child`,
	})

	srv := newEmptyTestServer(t)
	require.NoError(t, srv.CommitFile("main.tf", []byte(`# parent`), "add main.tf"))
	require.NoError(t, srv.CommitSubmodule("modules/child", subURL, subHead, "add submodule"))

	repoURL, err := srv.Start(t.Context())
	require.NoError(t, err)

	l := logger.CreateLogger()

	v, err := cas.OSVenv()
	require.NoError(t, err)

	tempDir := helpers.TmpDirWOSymlinks(t)
	storePath := filepath.Join(tempDir, "store")

	c, err := cas.New(cas.WithStorePath(storePath))
	require.NoError(t, err)

	assertClone := func(t *testing.T, targetPath string) {
		t.Helper()

		data, err := os.ReadFile(filepath.Join(targetPath, "main.tf"))
		require.NoError(t, err)
		assert.Equal(t, "# parent", string(data))

		_, err = os.Stat(filepath.Join(targetPath, ".gitmodules"))
		require.NoError(t, err)

		data, err = os.ReadFile(filepath.Join(targetPath, "modules", "child", "module.tf"))
		require.NoError(t, err)
		assert.Equal(t, "# child module", string(data))

		data, err = os.ReadFile(filepath.Join(targetPath, "modules", "child", "sub", "nested.tf"))
		require.NoError(t, err)
		assert.Equal(t, "# nested file in child", string(data))
	}

	firstTarget := filepath.Join(tempDir, "repo")
	err = c.Clone(t.Context(), l, v, repoURL, cas.WithDir(firstTarget), cas.WithDepth(-1))
	require.NoError(t, err)
	assertClone(t, firstTarget)

	// A second clone hits the tree store and materializes the submodule
	// without refetching anything.
	secondTarget := filepath.Join(tempDir, "repo-cached")
	err = c.Clone(t.Context(), l, v, repoURL, cas.WithDir(secondTarget), cas.WithDepth(-1))
	require.NoError(t, err)
	assertClone(t, secondTarget)
}

func TestCAS_CloneRepoWithNestedSubmodules(t *testing.T) {
	t.Parallel()

	grandchildURL, grandchildHead := startSubmoduleServer(t, map[string]string{
		"leaf.tf": `# grandchild`,
	})

	childSrv := newEmptyTestServer(t)
	require.NoError(t, childSrv.CommitFile("module.tf", []byte(`# child`), "add module.tf"))
	require.NoError(t, childSrv.CommitSubmodule("vendor/leaf", grandchildURL, grandchildHead, "add grandchild"))

	childHead, err := childSrv.Head()
	require.NoError(t, err)

	childURL, err := childSrv.Start(t.Context())
	require.NoError(t, err)

	srv := newEmptyTestServer(t)
	require.NoError(t, srv.CommitFile("main.tf", []byte(`# parent`), "add main.tf"))
	require.NoError(t, srv.CommitSubmodule("modules/child", childURL, childHead, "add child"))

	repoURL, err := srv.Start(t.Context())
	require.NoError(t, err)

	v, err := cas.OSVenv()
	require.NoError(t, err)

	tempDir := helpers.TmpDirWOSymlinks(t)
	targetPath := filepath.Join(tempDir, "repo")

	c, err := cas.New(cas.WithStorePath(filepath.Join(tempDir, "store")))
	require.NoError(t, err)

	err = c.Clone(t.Context(), logger.CreateLogger(), v, repoURL, cas.WithDir(targetPath), cas.WithDepth(-1))
	require.NoError(t, err)

	data, err := os.ReadFile(filepath.Join(targetPath, "modules", "child", "module.tf"))
	require.NoError(t, err)
	assert.Equal(t, "# child", string(data))

	data, err = os.ReadFile(filepath.Join(targetPath, "modules", "child", "vendor", "leaf", "leaf.tf"))
	require.NoError(t, err)
	assert.Equal(t, "# grandchild", string(data))
}

// TestCAS_CloneRepoWithUnregisteredGitlink pins the behavior for a
// gitlink with no .gitmodules entry, the shape left behind by
// accidentally committing a nested repository: `git clone` produces an
// empty directory there, and so must CAS.
func TestCAS_CloneRepoWithUnregisteredGitlink(t *testing.T) {
	t.Parallel()

	srv := newEmptyTestServer(t)
	require.NoError(t, srv.CommitFile("main.tf", []byte(`# parent`), "add main.tf"))

	// The pinned hash is never fetched, so any well-formed SHA works.
	const danglingHash = "0123456789abcdef0123456789abcdef01234567"

	require.NoError(t, srv.CommitSubmodule("vendor/orphan", "", danglingHash, "add orphan gitlink"))

	repoURL, err := srv.Start(t.Context())
	require.NoError(t, err)

	v, err := cas.OSVenv()
	require.NoError(t, err)

	tempDir := helpers.TmpDirWOSymlinks(t)
	targetPath := filepath.Join(tempDir, "repo")

	c, err := cas.New(cas.WithStorePath(filepath.Join(tempDir, "store")))
	require.NoError(t, err)

	err = c.Clone(t.Context(), logger.CreateLogger(), v, repoURL, cas.WithDir(targetPath), cas.WithDepth(-1))
	require.NoError(t, err)

	_, err = os.ReadFile(filepath.Join(targetPath, "main.tf"))
	require.NoError(t, err)

	info, err := os.Stat(filepath.Join(targetPath, "vendor", "orphan"))
	require.NoError(t, err)
	assert.True(t, info.IsDir())

	entries, err := os.ReadDir(filepath.Join(targetPath, "vendor", "orphan"))
	require.NoError(t, err)
	assert.Empty(t, entries)
}

// TestCAS_CloneSubmoduleWithRelativeURL pins relative .gitmodules URL
// resolution end to end: the submodule URL is declared relative to the
// parent repository URL and must be resolved before fetching. The
// child repository is mounted on the parent's server so the resolved
// URL lands on the same host, just like sibling repos on a forge.
func TestCAS_CloneSubmoduleWithRelativeURL(t *testing.T) {
	t.Parallel()

	childSrv := newEmptyTestServer(t)
	require.NoError(t, childSrv.CommitFile("module.tf", []byte(`# child module`), "add module.tf"))

	childHead, err := childSrv.Head()
	require.NoError(t, err)

	srv := newEmptyTestServer(t)
	srv.Mount("/child.git", childSrv)

	require.NoError(t, srv.CommitFile("main.tf", []byte(`# parent`), "add main.tf"))
	require.NoError(t, srv.CommitSubmodule("modules/child", "../child.git", childHead, "add submodule"))

	baseURL, err := srv.Start(t.Context())
	require.NoError(t, err)

	// The "../child.git" in .gitmodules resolves against this URL to
	// "<baseURL>/child.git", where the child repository is mounted.
	repoURL := baseURL + "/parent.git"

	v, err := cas.OSVenv()
	require.NoError(t, err)

	tempDir := helpers.TmpDirWOSymlinks(t)
	targetPath := filepath.Join(tempDir, "repo")

	c, err := cas.New(cas.WithStorePath(filepath.Join(tempDir, "store")))
	require.NoError(t, err)

	err = c.Clone(t.Context(), logger.CreateLogger(), v, repoURL, cas.WithDir(targetPath), cas.WithDepth(-1))
	require.NoError(t, err)

	data, err := os.ReadFile(filepath.Join(targetPath, "modules", "child", "module.tf"))
	require.NoError(t, err)
	assert.Equal(t, "# child module", string(data))
}
