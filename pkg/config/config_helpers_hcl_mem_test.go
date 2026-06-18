package config_test

import (
	"context"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/internal/vexec"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/gruntwork-io/terragrunt/pkg/config"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestHCLGetRepoRoot drives `get_repo_root()` through full HCL
// evaluation. The mem-backed exec stubs `git rev-parse --show-toplevel`
// so the test runs independently of any real git repository.
func TestHCLGetRepoRoot(t *testing.T) {
	t.Parallel()

	exec := vexec.NewMemExec(func(_ context.Context, inv vexec.Invocation) vexec.Result {
		assert.Equal(t, "git", inv.Name)
		assert.Equal(t, []string{"rev-parse", "--show-toplevel"}, inv.Args)

		return vexec.Result{Stdout: []byte("/synthetic/repo/root\n")}
	})

	l := logger.CreateLogger()
	ctx, pctx := newTestParsingContext(t, "/synthetic/repo/root/unit/terragrunt.hcl")
	ctx = config.WithConfigValues(ctx)
	pctx.Venv = venv.Venv{FS: vfs.NewMemMapFS(), Exec: exec}

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
	assert.Equal(t, "/synthetic/repo/root", out.Locals["repo"])
}

// TestHCLGetPathFromRepoRoot drives `get_path_from_repo_root()` through
// full HCL evaluation. The function computes the working dir relative
// to the git top-level dir, so the test stubs git to return a path that
// is an ancestor of pctx.WorkingDir.
func TestHCLGetPathFromRepoRoot(t *testing.T) {
	t.Parallel()

	exec := vexec.NewMemExec(func(_ context.Context, _ vexec.Invocation) vexec.Result {
		return vexec.Result{Stdout: []byte("/repo\n")}
	})

	l := logger.CreateLogger()
	ctx, pctx := newTestParsingContext(t, "/repo/services/api/terragrunt.hcl")
	ctx = config.WithConfigValues(ctx)
	pctx.Venv = venv.Venv{FS: vfs.NewMemMapFS(), Exec: exec}
	pctx.WorkingDir = "/repo/services/api"

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

	exec := vexec.NewMemExec(func(_ context.Context, _ vexec.Invocation) vexec.Result {
		return vexec.Result{Stdout: []byte("/repo\n")}
	})

	l := logger.CreateLogger()
	ctx, pctx := newTestParsingContext(t, "/repo/services/api/terragrunt.hcl")
	ctx = config.WithConfigValues(ctx)
	pctx.Venv = venv.Venv{FS: vfs.NewMemMapFS(), Exec: exec}
	pctx.WorkingDir = "/repo/services/api"

	const hcl = `locals {
  up = get_path_to_repo_root()
}`

	out, err := config.ParseConfigString(ctx, pctx, l, "test.hcl", hcl, nil)
	require.NoError(t, err)
	assert.Equal(t, "../..", out.Locals["up"])
}

// TestHCLGetRepoRootPropagatesGitError pins the contract that a failing
// `git rev-parse` surfaces as an error from ParseConfigString rather
// than silently producing an empty string.
func TestHCLGetRepoRootPropagatesGitError(t *testing.T) {
	t.Parallel()

	exec := vexec.NewMemExec(func(_ context.Context, _ vexec.Invocation) vexec.Result {
		return vexec.Result{ExitCode: 128, Stderr: []byte("fatal: not a git repository\n")}
	})

	l := logger.CreateLogger()
	ctx, pctx := newTestParsingContext(t, "/not/a/repo/terragrunt.hcl")
	ctx = config.WithConfigValues(ctx)
	pctx.Venv = venv.Venv{FS: vfs.NewMemMapFS(), Exec: exec}

	const hcl = `locals {
  repo = get_repo_root()
}`

	_, err := config.ParseConfigString(ctx, pctx, l, "test.hcl", hcl, nil)
	require.Error(t, err)
}

// TestHCLRunCmd drives `run_cmd()` through full HCL evaluation,
// replacing the old TestRunCommand harness with the mem-backed exec.
// The local resolves to the subprocess stdout.
func TestHCLRunCmd(t *testing.T) {
	t.Parallel()

	exec := vexec.NewMemExec(func(_ context.Context, inv vexec.Invocation) vexec.Result {
		assert.Equal(t, "describe", inv.Name)
		assert.Equal(t, []string{"--account", "prod"}, inv.Args)

		return vexec.Result{Stdout: []byte("account-1234\n")}
	})

	l := logger.CreateLogger()
	ctx, pctx := newTestParsingContext(t, t.TempDir()+"/terragrunt.hcl")
	ctx = config.WithConfigValues(ctx)
	pctx.Venv = venv.Venv{FS: vfs.NewMemMapFS(), Exec: exec}

	const hcl = `locals {
  account = run_cmd("--terragrunt-quiet", "describe", "--account", "prod")
}`

	out, err := config.ParseConfigString(ctx, pctx, l, "test.hcl", hcl, nil)
	require.NoError(t, err)
	assert.Equal(t, "account-1234", out.Locals["account"])
}
