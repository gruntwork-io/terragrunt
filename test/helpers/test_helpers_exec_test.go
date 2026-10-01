package helpers_test

import (
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gruntwork-io/terragrunt/test/helpers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCreateGitRepo(t *testing.T) {
	t.Parallel()

	thTRequireGit(t)

	dir := helpers.TmpDirWOSymlinks(t)
	thTWriteFile(t, filepath.Join(dir, "main.tf"), "")

	helpers.CreateGitRepo(t, dir)

	assert.Equal(t, "initial commit", thTGit(t, dir, "log", "--format=%s"))
	assert.Equal(t, "main.tf", thTGit(t, dir, "ls-files"))
	assert.Equal(t, "false", thTGit(t, dir, "config", "commit.gpgsign"))
}

func TestInitGitRepoWithBranchRef(t *testing.T) {
	t.Parallel()

	thTRequireGit(t)

	dir := helpers.TmpDirWOSymlinks(t)
	thTWriteFile(t, filepath.Join(dir, "terragrunt.hcl"), "")

	helpers.InitGitRepoWithBranchRef(t, dir, "feature/ref")

	head := thTGit(t, dir, "rev-parse", "HEAD")

	assert.Equal(t, "main", thTGit(t, dir, "rev-parse", "--abbrev-ref", "HEAD"))
	assert.Equal(t, head, thTGit(t, dir, "rev-parse", "refs/heads/feature/ref"))
	assert.Equal(t, "terragrunt.hcl", thTGit(t, dir, "ls-files"))
}

func TestCloneGitRepo(t *testing.T) {
	t.Parallel()

	thTRequireGit(t)

	src := helpers.TmpDirWOSymlinks(t)
	thTWriteFile(t, filepath.Join(src, "terragrunt.hcl"), "")
	helpers.InitGitRepoWithBranchRef(t, src, "main")

	dst := helpers.CloneGitRepo(t, src)

	assert.NotEqual(t, src, dst)
	assert.FileExists(t, filepath.Join(dst, "terragrunt.hcl"))
	assert.Equal(t, thTGit(t, src, "rev-parse", "HEAD"), thTGit(t, dst, "rev-parse", "HEAD"))

	resolved, err := filepath.EvalSymlinks(dst)
	require.NoError(t, err)
	assert.Equal(t, resolved, dst)
}

func TestLocalGitRemote(t *testing.T) {
	t.Parallel()

	thTRequireGit(t)

	url := helpers.LocalGitRemote(t, "../fixtures/download/hello-world")

	require.True(t, strings.HasPrefix(url, "file://"), "URL must keep its file:// scheme: %q", url)

	dst := t.TempDir()
	thTGit(t, dst, "clone", "--branch", "main", url, "clone")

	assert.FileExists(t, filepath.Join(dst, "clone", "main.tf"))
}

func TestExecWithTestLogger(t *testing.T) {
	t.Parallel()

	thTRequireGit(t)

	// A zero exit passes; output is only logged.
	helpers.ExecWithTestLogger(t, t.TempDir(), "git", "--version")
}

func TestExecAndCaptureOutput(t *testing.T) {
	t.Parallel()

	thTRequireGit(t)

	stdout, stderr := helpers.ExecAndCaptureOutput(t, t.TempDir(), "git", "--version")

	assert.True(t, strings.HasPrefix(stdout, "git version "), "stdout: %q", stdout)
	assert.Empty(t, stderr)
}

func TestExecWithMiseAndTestLogger(t *testing.T) {
	t.Parallel()

	repoRoot := thTRequireMiseGo(t)

	helpers.ExecWithMiseAndTestLogger(t, repoRoot, "go", "version")
}

func TestExecWithMiseAndCaptureOutput(t *testing.T) {
	t.Parallel()

	repoRoot := thTRequireMiseGo(t)

	stdout, _ := helpers.ExecWithMiseAndCaptureOutput(t, repoRoot, "go", "version")

	assert.True(t, strings.HasPrefix(stdout, "go version "), "stdout: %q", stdout)
}

// thTRequireGit skips the test without git on PATH, and every CI job has git, so CI still runs these tests.
func thTRequireGit(t *testing.T) {
	t.Helper()

	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not on PATH")
	}
}

// thTRequireMiseGo returns the repo root, skipping the test unless mise resolves go there without installing it.
func thTRequireMiseGo(t *testing.T) string {
	t.Helper()

	if _, err := exec.LookPath("mise"); err != nil {
		t.Skip("mise is not on PATH")
	}

	repoRoot := helpers.MustAbs(t, filepath.Join("..", ".."))

	cmd := exec.CommandContext(t.Context(), "mise", "which", "go")
	cmd.Dir = repoRoot

	if out, err := cmd.CombinedOutput(); err != nil {
		t.Skipf("mise does not provide go in %s: %v: %s", repoRoot, err, out)
	}

	return repoRoot
}

// thTGit runs git in dir and returns its trimmed standard output.
func thTGit(t *testing.T, dir string, args ...string) string {
	t.Helper()

	cmd := exec.CommandContext(t.Context(), "git", args...)
	cmd.Dir = dir

	out, err := cmd.Output()
	require.NoError(t, err, "git %v", args)

	return strings.TrimSpace(string(out))
}
