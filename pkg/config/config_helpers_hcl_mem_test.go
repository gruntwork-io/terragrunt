package config_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/vexec"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/gruntwork-io/terragrunt/pkg/config"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/gruntwork-io/terragrunt/test/helpers/venvtest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestHCLGetRepoRoot drives `get_repo_root()` through full HCL evaluation
// against an in-memory repository, so the test runs independently of any
// checkout on the machine.
func TestHCLGetRepoRoot(t *testing.T) {
	t.Parallel()

	l := logger.CreateLogger()
	repoRoot := venvtest.Root("/synthetic/repo/root")
	v := venvtest.New().WithFS(memRepoFS(t, repoRoot, "unit"))
	ctx, pctx := newTestParsingContext(t, v, filepath.Join(repoRoot, "unit", "terragrunt.hcl"))
	ctx = config.WithConfigValues(ctx)

	const hcl = `locals {
  repo = get_repo_root()
}
terraform {
  source = local.repo
}`

	out, err := config.ParseConfigString(ctx, pctx, l, "test.hcl", hcl, nil)
	require.NoError(t, err)
	require.NotNil(t, out)
	require.NotNil(t, out.Locals)
	assert.Equal(t, repoRoot, out.Locals["repo"])
}

// TestHCLGetPathFromRepoRoot drives `get_path_from_repo_root()` through
// full HCL evaluation. The function computes the working dir relative
// to the git top-level dir, so the test stubs git to return a path that
// is an ancestor of pctx.WorkingDir.
func TestHCLGetPathFromRepoRoot(t *testing.T) {
	t.Parallel()

	l := logger.CreateLogger()
	workingDir := venvtest.Root("/repo/services/api")
	v := venvtest.New().WithFS(memRepoFS(t, venvtest.Root("/repo"), "services/api"))
	ctx, pctx := newTestParsingContext(t, v, filepath.Join(workingDir, "terragrunt.hcl"))
	ctx = config.WithConfigValues(ctx)
	pctx.WorkingDir = workingDir

	const hcl = `locals {
  rel = get_path_from_repo_root()
}`

	out, err := config.ParseConfigString(ctx, pctx, l, "test.hcl", hcl, nil)
	require.NoError(t, err)
	assert.Equal(t, "services/api", out.Locals["rel"])
}

// TestHCLGetPathToRepoRoot drives `get_path_to_repo_root()` through
// full HCL evaluation. It is the inverse of get_path_from_repo_root:
// the path from the working dir back up to the repo root.
func TestHCLGetPathToRepoRoot(t *testing.T) {
	t.Parallel()

	l := logger.CreateLogger()
	workingDir := venvtest.Root("/repo/services/api")
	v := venvtest.New().WithFS(memRepoFS(t, venvtest.Root("/repo"), "services/api"))
	ctx, pctx := newTestParsingContext(t, v, filepath.Join(workingDir, "terragrunt.hcl"))
	ctx = config.WithConfigValues(ctx)
	pctx.WorkingDir = workingDir

	const hcl = `locals {
  up = get_path_to_repo_root()
}`

	out, err := config.ParseConfigString(ctx, pctx, l, "test.hcl", hcl, nil)
	require.NoError(t, err)
	assert.Equal(t, "../..", out.Locals["up"])
}

// TestHCLGetRepoRootPropagatesLookupFailure pins the contract that a working
// directory outside any repository surfaces as an error from
// ParseConfigString rather than silently producing an empty string.
func TestHCLGetRepoRootPropagatesLookupFailure(t *testing.T) {
	t.Parallel()

	l := logger.CreateLogger()
	v := venvtest.New().
		WithFS(venvtest.NewFS(t, venvtest.Root("/not/a/repo"), map[string]string{"terragrunt.hcl": ""}))
	ctx, pctx := newTestParsingContext(t, v, venvtest.Root("/not/a/repo/terragrunt.hcl"))
	ctx = config.WithConfigValues(ctx)

	const hcl = `locals {
  repo = get_repo_root()
}`

	_, err := config.ParseConfigString(ctx, pctx, l, "test.hcl", hcl, nil)
	require.Error(t, err)
}

// TestHCLRunCmd drives `run_cmd()` through full HCL evaluation,
// replacing the old TestTFRunCommand harness with the mem-backed exec.
// The local resolves to the subprocess stdout.
func TestHCLRunCmd(t *testing.T) {
	t.Parallel()

	exec := vexec.NewMemExec(func(_ context.Context, inv vexec.Invocation) vexec.Result {
		assert.Equal(t, "describe", inv.Name)
		assert.Equal(t, []string{"--account", "prod"}, inv.Args)

		return vexec.Result{Stdout: []byte("account-1234\n")}
	})

	l := logger.CreateLogger()
	ctx, pctx := newTestParsingContext(
		t,
		venvtest.New().WithExec(exec),
		t.TempDir()+"/terragrunt.hcl",
	)
	ctx = config.WithConfigValues(ctx)

	const hcl = `locals {
  account = run_cmd("--terragrunt-quiet", "describe", "--account", "prod")
}`

	out, err := config.ParseConfigString(ctx, pctx, l, "test.hcl", hcl, nil)
	require.NoError(t, err)
	assert.Equal(t, "account-1234", out.Locals["account"])
}

// memRepoFS returns an in-memory repository rooted at root, with unitDir
// beneath it. `.git` is written as a file, the shape a submodule and a linked
// worktree both use.
func memRepoFS(t *testing.T, root, unitDir string) vfs.FS {
	t.Helper()

	return venvtest.NewFS(t, root, map[string]string{
		".git":                                   "gitdir: /elsewhere/.git\n",
		filepath.Join(unitDir, "terragrunt.hcl"): "",
	})
}
