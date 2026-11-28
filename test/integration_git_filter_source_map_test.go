package test_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gruntwork-io/terragrunt/test/helpers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	// gitFilterSourceMapUnresolvableSource is a unit source that can only be fetched through --source-map.
	gitFilterSourceMapUnresolvableSource = "git::https://example.invalid/catalog.git//unit?ref=v1"
	gitFilterSourceMapSourcePrefix       = "git::https://example.invalid/catalog.git"
)

// setupGitFilterSourceMapRepo creates a Git repository with two commits that differ in a stack file
// whose unit source is unreachable unless it is mapped, plus the local catalog it maps to.
func setupGitFilterSourceMapRepo(t *testing.T) (repoDir, catalogDir string) {
	t.Helper()

	root := helpers.TmpDirWOSymlinks(t)
	catalogDir = filepath.Join(root, "catalog")
	repoDir = filepath.Join(root, "repo")

	require.NoError(t, os.MkdirAll(filepath.Join(catalogDir, "unit"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(catalogDir, "unit", "terragrunt.hcl"), []byte("inputs = {}\n"), 0o644))
	require.NoError(t, os.MkdirAll(repoDir, 0o755))

	runner := helpers.InitTestGitRunner(t, repoDir)
	stackFile := filepath.Join(repoDir, "terragrunt.stack.hcl")
	stack := `unit "app" {
  source = "` + gitFilterSourceMapUnresolvableSource + `"
  path   = "app"
}
`

	require.NoError(t, os.WriteFile(filepath.Join(repoDir, ".gitignore"), []byte(".terragrunt-stack\n"), 0o644))
	require.NoError(t, os.WriteFile(stackFile, []byte(stack), 0o644))
	require.NoError(t, runner.Add(t.Context(), "."))
	require.NoError(t, runner.Commit(t.Context(), "one"))

	require.NoError(t, os.WriteFile(stackFile, []byte(stack+"# changed\n"), 0o644))
	require.NoError(t, runner.Add(t.Context(), "."))
	require.NoError(t, runner.Commit(t.Context(), "two"))

	return repoDir, catalogDir
}

// TestGitFilterAppliesSourceMapToWorktreeStacks pins that the stacks `find` and `list` generate in
// the worktrees of a Git filter resolve their unit sources through --source-map and TG_SOURCE_MAP.
func TestGitFilterAppliesSourceMapToWorktreeStacks(t *testing.T) {
	// Not parallel: the env case sets a process-wide variable.
	t.Setenv("GIT_TERMINAL_PROMPT", "0")

	testCases := []struct {
		name    string
		command string
		env     bool
	}{
		{name: "find flag", command: "find"},
		{name: "find env", command: "find", env: true},
		{name: "list flag", command: "list"},
		{name: "list env", command: "list", env: true},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			repoDir, catalogDir := setupGitFilterSourceMapRepo(t)
			mapping := gitFilterSourceMapSourcePrefix + "=" + catalogDir

			args := ""

			if tc.env {
				t.Setenv("TG_SOURCE_MAP", mapping)
			} else {
				args = " --source-map '" + mapping + "'"
			}

			stdout, stderr, err := helpers.RunTerragruntCommandWithOutput(
				t,
				"terragrunt "+tc.command+" --no-color --working-dir "+repoDir+" --filter '[HEAD~1...HEAD]'"+args,
			)
			require.NoError(t, err, "stderr: %s", stderr)

			assert.Equal(t, ".", strings.TrimSpace(stdout))
		})
	}
}
