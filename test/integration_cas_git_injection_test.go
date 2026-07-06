package test_test

import (
	"context"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/gruntwork-io/terragrunt/test/helpers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestCASGitSourceRefOptionInjection checks a git source ?ref= never runs a command during download.
func TestCASGitSourceRefOptionInjection(t *testing.T) {
	t.Parallel()

	marker := filepath.Join(helpers.TmpDirWOSymlinks(t), "injected")

	// ${IFS} avoids a literal space, which git rejects in a ref name.
	injectedRef := "--upload-pack=touch${IFS}" + marker

	repoDir := buildInjectionSourceRepo(t, injectedRef)

	liveDir := helpers.TmpDirWOSymlinks(t)
	source := "git::file://" + repoDir + "?ref=" + url.QueryEscape(injectedRef)

	config := "terraform {\n  source = \"" + source + "\"\n}\n"
	require.NoError(t, os.WriteFile(filepath.Join(liveDir, "terragrunt.hcl"), []byte(config), 0o644))

	// The injected command must never run, even if the download itself fails.
	_, _, _ = helpers.RunTerragruntCommandWithOutput(
		t,
		"terragrunt plan --source-update --non-interactive --working-dir "+liveDir,
	)

	assert.NoFileExists(t, marker)
}

// buildInjectionSourceRepo creates a file:// repo with one commit and a branch named injectedRef.
func buildInjectionSourceRepo(t *testing.T, injectedRef string) string {
	t.Helper()

	ctx := t.Context()

	dir := helpers.TmpDirWOSymlinks(t)

	helpers.CreateFile(t, dir, "main.tf")

	runInjectionGit(t, ctx, dir, "init", "-b", "main")
	runInjectionGit(t, ctx, dir, "config", "user.email", "test@example.com")
	runInjectionGit(t, ctx, dir, "config", "user.name", "Terragrunt Test")
	runInjectionGit(t, ctx, dir, "config", "commit.gpgsign", "false")
	runInjectionGit(t, ctx, dir, "add", "-A")
	runInjectionGit(t, ctx, dir, "commit", "-m", "initial commit")
	runInjectionGit(t, ctx, dir, "update-ref", "refs/heads/"+injectedRef, "HEAD")

	return dir
}

func runInjectionGit(t *testing.T, ctx context.Context, dir string, args ...string) {
	t.Helper()

	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir

	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "git %v failed: %s", args, string(out))
}
