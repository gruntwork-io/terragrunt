package cas_test

import (
	"context"
	"net/url"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/cas"
	"github.com/gruntwork-io/terragrunt/internal/getter"
	"github.com/gruntwork-io/terragrunt/test/helpers"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestCASGetterRefOptionInjection checks the CAS git source path never runs a command from a crafted ref.
func TestCASGetterRefOptionInjection(t *testing.T) {
	t.Parallel()

	ctx := t.Context()

	marker := filepath.Join(helpers.TmpDirWOSymlinks(t), "injected")

	// ${IFS} avoids a literal space, which git rejects in a ref name.
	injectedRef := "--upload-pack=touch${IFS}" + marker

	repoDir := setupInjectionRepo(t, injectedRef)

	storePath := filepath.Join(helpers.TmpDirWOSymlinks(t), "store")

	c, err := cas.New(cas.WithStorePath(storePath))
	require.NoError(t, err)

	v, err := cas.OSVenv()
	require.NoError(t, err)

	g := getter.NewCASGetter(logger.CreateLogger(), c, v, &cas.CloneOptions{Depth: 1})
	client := getter.Client{Getters: []getter.Getter{g}}

	src := "git::file://" + repoDir + "?ref=" + url.QueryEscape(injectedRef)

	dst := helpers.TmpDirWOSymlinks(t)

	// The injected command must never run, whether the fetch succeeds or fails.
	_, _ = client.Get(ctx, &getter.Request{Src: src, Dst: dst})

	assert.NoFileExists(t, marker)
}

// setupInjectionRepo creates a file:// repo with one commit and a branch named injectedRef.
func setupInjectionRepo(t *testing.T, injectedRef string) string {
	t.Helper()

	ctx := t.Context()

	dir := helpers.TmpDirWOSymlinks(t)

	runGit(t, ctx, dir, "init", "-b", "main")
	runGit(t, ctx, dir, "config", "user.email", "test@example.com")
	runGit(t, ctx, dir, "config", "user.name", "Terragrunt Test")
	runGit(t, ctx, dir, "config", "commit.gpgsign", "false")

	helpers.CreateFile(t, dir, "main.tf")

	runGit(t, ctx, dir, "add", "-A")
	runGit(t, ctx, dir, "commit", "-m", "initial commit")
	runGit(t, ctx, dir, "update-ref", "refs/heads/"+injectedRef, "HEAD")

	return dir
}

func runGit(t *testing.T, ctx context.Context, dir string, args ...string) {
	t.Helper()

	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir

	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "git %v failed: %s", args, string(out))
}
