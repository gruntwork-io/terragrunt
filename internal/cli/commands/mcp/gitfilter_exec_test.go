//go:build exec

// Git-range filter parity test. It builds a repository on disk and spawns the
// real git binary, which the fail-closed venv the rest of the package runs on
// cannot do, so it stays behind the exec tag.

package mcp_test

import (
	"os/exec"
	"path/filepath"
	"testing"

	tgmcp "github.com/gruntwork-io/terragrunt/internal/cli/commands/mcp"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// gitInDir runs one git command in dir, carrying an identity so a commit does
// not depend on the machine's git configuration.
func gitInDir(t *testing.T, dir string, args ...string) {
	t.Helper()

	identity := []string{"-c", "user.email=test@example.com", "-c", "user.name=test"}

	argv := make([]string, 0, len(identity)+len(args))
	argv = append(argv, identity...)
	argv = append(argv, args...)

	cmd := exec.CommandContext(t.Context(), "git", argv...)
	cmd.Dir = dir

	out, err := cmd.CombinedOutput()
	require.NoError(t, err, string(out))
}

// TestExecGitRangeFilterFindsTheChangedUnit pins that a git-range filter
// resolves against a real repository. Terragrunt looks its own git up before
// spawning it, so the invocation arrives carrying an absolute path, and
// refusing it on that path left the filter matching nothing while the call
// still answered as though it had run.
func TestExecGitRangeFilterFindsTheChangedUnit(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	fsys := vfs.NewOSFS()

	writeUnit := func(name string) {
		require.NoError(t, fsys.MkdirAll(filepath.Join(dir, name), 0o755))
		require.NoError(t, vfs.WriteFile(
			fsys, filepath.Join(dir, name, "terragrunt.hcl"), []byte("\n"), 0o644))
	}

	writeUnit("a")
	gitInDir(t, dir, "init", "-q", "-b", "main")
	gitInDir(t, dir, "add", ".")
	gitInDir(t, dir, "commit", "-q", "-m", "a")

	writeUnit("b")
	gitInDir(t, dir, "add", ".")
	gitInDir(t, dir, "commit", "-q", "-m", "b")

	resolved, err := vfs.EvalSymlinks(fsys, dir)
	require.NoError(t, err)

	session := newRealExecSession(t, resolved, func(o *tgmcp.Options) {
		o.Allow = []string{"exec"}
	})

	var out discoverOutput

	callTool(t, session, "discover", map[string]any{"filter": []string{"[HEAD~1...HEAD]"}}, &out)

	paths := make([]string, 0, len(out.Units))
	for _, unit := range out.Units {
		paths = append(paths, unit.Path)
	}

	assert.Equal(t, []string{"b"}, paths, "the range names the unit the second commit added")
	assert.Empty(t, out.Degraded, "a git the operator's PATH resolves must not read as refused")
}
