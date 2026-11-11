//go:build exec

package mcp_test

import (
	"path/filepath"
	"testing"

	tgmcp "github.com/gruntwork-io/terragrunt/internal/cli/commands/mcp"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestExecScaffoldRefusesASourceLinkingOutsideItself pins that a source whose
// clone holds a link out of it is refused before anything is written. A local
// copy follows links, so the source is a git repository, which keeps them.
func TestExecScaffoldRefusesASourceLinkingOutsideItself(t *testing.T) {
	t.Parallel()

	fsys := vfs.NewOSFS()

	outside := writeTree(t, map[string]string{"secret.txt": "secret\n"})
	repo := writeTree(t, map[string]string{"terragrunt.hcl": unitValuesConfig})
	require.NoError(
		t,
		vfs.Symlink(fsys, filepath.Join(outside, "secret.txt"), filepath.Join(repo, "secret.txt")),
	)

	gitInDir(t, repo, "init", "-q", "-b", "main")
	gitInDir(t, repo, "add", ".")
	gitInDir(t, repo, "commit", "-q", "-m", "unit")

	dir := writeTree(t, map[string]string{})

	session := newRealExecSession(t, dir, func(o *tgmcp.Options) {
		o.Allow = []string{"exec"}
	})

	callToolRefused(t, session, "scaffold", map[string]any{
		"source":        "git::file://" + filepath.ToSlash(repo) + "?ref=main",
		"output_folder": "live/app",
		"values":        map[string]any{"name": `"app"`},
	}, tgmcp.ErrScaffoldUnsafeTemplate)

	assert.NoDirExists(t, filepath.Join(dir, "live"))
}
