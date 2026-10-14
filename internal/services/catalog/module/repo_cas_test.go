package module_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gruntwork-io/terragrunt/internal/services/catalog/module"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/test/helpers"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/gruntwork-io/terragrunt/test/helpers/venvtest"
)

// sourceDefaultBranch is neither main nor master, so it cannot match the
// branch a fresh bare repository in the CAS store starts on.
const sourceDefaultBranch = "trunk"

// TestNewRepoThroughCASReportsSourceRemoteAndDefaultBranch pins that a
// repository materialized from the CAS store reports the remote it was cloned
// from and the branch that remote's HEAD points at, so module URLs link to the
// source rather than to a path in the clone directory.
func TestNewRepoThroughCASReportsSourceRemoteAndDefaultBranch(t *testing.T) {
	t.Parallel()

	tmpDir := helpers.TmpDirWOSymlinks(t)
	sourceDir := writeCatalogSource(t, tmpDir)
	helpers.InitGitRepoOnBranch(t, sourceDir, sourceDefaultBranch)

	repo, err := module.NewRepo(t.Context(), logger.CreateLogger(), casVenv(tmpDir), &module.RepoOpts{
		CloneURL:      "git::" + helpers.FileURL(sourceDir),
		Path:          filepath.Join(tmpDir, "clone"),
		AllowCAS:      true,
		CASCloneDepth: 1,
	})
	require.NoError(t, err)

	assert.Equal(t, helpers.FileURL(sourceDir), repo.RemoteURL)
	assert.Equal(t, sourceDefaultBranch, repo.BranchName)
}

// TestNewRepoThroughCASReportsRequestedRef pins that a CAS clone pinned to a
// ref reports that ref as its branch.
func TestNewRepoThroughCASReportsRequestedRef(t *testing.T) {
	t.Parallel()

	tmpDir := helpers.TmpDirWOSymlinks(t)
	sourceDir := writeCatalogSource(t, tmpDir)
	helpers.InitGitRepoWithBranchRef(t, sourceDir, "release")

	repo, err := module.NewRepo(t.Context(), logger.CreateLogger(), casVenv(tmpDir), &module.RepoOpts{
		CloneURL:      "git::" + helpers.FileURL(sourceDir) + "?ref=release",
		Path:          filepath.Join(tmpDir, "clone"),
		AllowCAS:      true,
		CASCloneDepth: 1,
	})
	require.NoError(t, err)

	assert.Equal(t, helpers.FileURL(sourceDir), repo.RemoteURL)
	assert.Equal(t, "release", repo.BranchName)
}

// TestNewRepoThroughCASLinksModulesUnderRequestedSubdir pins that a CAS clone
// of a //subdir links and sources its modules by their path from the
// repository root.
func TestNewRepoThroughCASLinksModulesUnderRequestedSubdir(t *testing.T) {
	t.Parallel()

	tmpDir := helpers.TmpDirWOSymlinks(t)
	sourceDir := writeCatalogSource(t, tmpDir)
	helpers.InitGitRepoOnBranch(t, sourceDir, sourceDefaultBranch)

	repo, err := module.NewRepo(t.Context(), logger.CreateLogger(), casVenv(tmpDir), &module.RepoOpts{
		CloneURL:      "git::" + helpers.FileURL(sourceDir) + "//modules",
		Path:          filepath.Join(tmpDir, "clone"),
		AllowCAS:      true,
		CASCloneDepth: 1,
	})
	require.NoError(t, err)
	require.FileExists(t, filepath.Join(repo.Path(), "vpc", "main.tf"))

	assert.Equal(t, helpers.FileURL(sourceDir), repo.RemoteURL)
	assert.Equal(
		t,
		"git::"+helpers.FileURL(sourceDir)+"//modules/vpc",
		module.SourcePath(repo.CloneURL(), "vpc"),
	)

	repo.RemoteURL = "https://github.com/acme/catalog"

	assert.Equal(
		t,
		"https://github.com/acme/catalog/tree/"+sourceDefaultBranch+"/modules/vpc",
		repo.ModuleURL("vpc"),
	)
}

// TestNewRepoThroughCASOfflineReportsHead pins that an offline CAS clone does
// not ask the remote for its default branch and reports HEAD instead.
func TestNewRepoThroughCASOfflineReportsHead(t *testing.T) {
	t.Parallel()

	tmpDir := helpers.TmpDirWOSymlinks(t)
	sourceDir := writeCatalogSource(t, tmpDir)
	helpers.InitGitRepoOnBranch(t, sourceDir, sourceDefaultBranch)

	v := casVenv(tmpDir)
	opts := module.RepoOpts{
		CloneURL:      "git::" + helpers.FileURL(sourceDir),
		AllowCAS:      true,
		CASCloneDepth: 1,
		CASProbeCache: true,
	}

	online := opts
	online.Path = filepath.Join(tmpDir, "online-clone")

	_, err := module.NewRepo(t.Context(), logger.CreateLogger(), v, &online)
	require.NoError(t, err)

	offline := opts
	offline.Path = filepath.Join(tmpDir, "offline-clone")
	offline.CASOffline = true

	repo, err := module.NewRepo(t.Context(), logger.CreateLogger(), v, &offline)
	require.NoError(t, err)

	assert.Equal(t, "HEAD", repo.BranchName)
}

// writeCatalogSource writes a catalog with one module under dir and returns
// the directory to make a git repository of.
func writeCatalogSource(t *testing.T, dir string) string {
	t.Helper()

	sourceDir := filepath.Join(dir, "catalog")
	moduleDir := filepath.Join(sourceDir, "modules", "vpc")

	require.NoError(t, os.MkdirAll(moduleDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(moduleDir, "main.tf"), nil, 0o644))

	return sourceDir
}

// casVenv returns the OS venv with the CAS store under dir.
func casVenv(dir string) *venv.Venv {
	return venvtest.NewOSWithEmptyEnv().
		WithUserCacheDir(func() (string, error) { return filepath.Join(dir, "cache"), nil })
}
