//go:build exec && !windows

package mcp_test

import (
	"path/filepath"
	"strings"
	"testing"

	tgmcp "github.com/gruntwork-io/terragrunt/internal/cli/commands/mcp"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/gruntwork-io/terragrunt/test/helpers"
	"github.com/gruntwork-io/terragrunt/test/helpers/venvtest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestExecCatalogClonesARepository pins that --allow=exec lets the tool clone
// with the real git, that the result names a source a terraform block can use,
// and that no clone is left behind.
func TestExecCatalogClonesARepository(t *testing.T) {
	t.Parallel()

	repo := writeTree(t, map[string]string{"modules/alpha/main.tf": "\n"})
	helpers.CreateGitRepo(t, repo)

	url := "git::file://" + repo
	dir := writeTree(t, map[string]string{"root.hcl": "catalog {\n  urls = [\"" + url + "\"]\n}\n"})

	scratch := t.TempDir()
	v := withCatalogDirs(venvtest.NewOSWithEmptyEnv(), scratch)
	session := newTestSessionWith(t, dir, v, func(o *tgmcp.Options) { o.Allow = []string{"exec"} }, nil)

	var out catalogOutput

	callTool(t, session, "catalog", map[string]any{}, &out)

	assert.Empty(t, out.Failures)
	require.Equal(t, []string{"modules/alpha"}, catalogDirs(&out))
	assert.Equal(t, url+"//modules/alpha", out.Components[0].ComponentSource)

	entries, err := vfs.ReadDir(v.FS, scratch)
	require.NoError(t, err)

	for _, entry := range entries {
		assert.False(t, strings.HasPrefix(entry.Name(), "catalog-"), "clone left behind: %s", entry.Name())
	}
}

// TestExecCatalogKeepsACredentialOutOfAClonedRepositorysComponents pins that a
// credential in a repository's address stays out of the components the clone
// gives, each of which names the address in its source. git ignores the
// userinfo of a file:// remote, so the clone succeeds without leaving the
// machine.
func TestExecCatalogKeepsACredentialOutOfAClonedRepositorysComponents(t *testing.T) {
	t.Parallel()

	repo := writeTree(t, map[string]string{"modules/alpha/main.tf": "\n"})
	helpers.CreateGitRepo(t, repo)

	url := "git::file://user:" + catalogToken + "@" + repo
	dir := writeTree(t, map[string]string{"root.hcl": "catalog {\n  urls = [\"" + url + "\"]\n}\n"})

	v := withCatalogDirs(venvtest.NewOSWithEmptyEnv(), t.TempDir())
	session := newTestSessionWith(t, dir, v, func(o *tgmcp.Options) { o.Allow = []string{"exec"} }, nil)

	var out catalogOutput

	callTool(t, session, "catalog", map[string]any{"include_docs": true}, &out)

	assert.Empty(t, out.Failures)
	require.Equal(t, []string{"modules/alpha"}, catalogDirs(&out))
	assert.Equal(t, "git::file://"+repo+"//modules/alpha", out.Components[0].ComponentSource)
	assert.NotContains(t, catalogMarshal(t, &out), catalogToken)
}

// TestExecCatalogKeepsACredentialOutOfGitsStderr pins redaction against what
// the real git prints. The remote is a closed loopback port, so the clone fails
// without leaving the machine.
func TestExecCatalogKeepsACredentialOutOfGitsStderr(t *testing.T) {
	t.Parallel()

	for name, url := range map[string]string{
		"in the query":    "git::https://127.0.0.1:1/org/modules.git?token=" + catalogToken,
		"in the userinfo": "git::https://user:" + catalogToken + "@127.0.0.1:1/org/modules.git",
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			dir := writeTree(t, map[string]string{"root.hcl": "catalog {\n  urls = [\"" + url + "\"]\n}\n"})
			v := withCatalogDirs(venvtest.NewOSWithEmptyEnv(), t.TempDir())
			session := newTestSessionWith(t, dir, v, func(o *tgmcp.Options) { o.Allow = []string{"exec"} }, nil)

			var out catalogOutput

			callTool(t, session, "catalog", map[string]any{}, &out)

			require.Len(t, out.Failures, 1)
			assert.NotContains(t, catalogMarshal(t, &out), catalogToken)
		})
	}
}

// TestExecCatalogRunsAProgramThePersonApproves pins the approval the tool
// shares with the other parse-only tools: the block's program is offered once,
// and the accepted run lists what the program named.
func TestExecCatalogRunsAProgramThePersonApproves(t *testing.T) {
	t.Parallel()

	for name, config := range map[string]string{
		"in the catalog block": `catalog { urls = [run_cmd("--terragrunt-quiet", "echo", "./repo")] }`,
		"in a local": `
locals { url = run_cmd("--terragrunt-quiet", "echo", "./repo") }
catalog { urls = [local.url] }
`,
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			dir := catalogTree(t, catalogAlphaReadme, "./repo")
			require.NoError(t, vfs.WriteFile(vfs.NewOSFS(), filepath.Join(dir, "root.hcl"), []byte(config), 0o644))

			var prompts []string

			v := withCatalogDirs(venvtest.NewOSWithEmptyEnv(), t.TempDir())
			session := newAnsweringSession(t, dir, v, "accept", &prompts,
				func(o *tgmcp.Options) { o.Allow = []string{"exec"} })

			var out catalogOutput

			callTool(t, session, "catalog", map[string]any{}, &out)

			assert.Len(t, prompts, 1)
			assert.Empty(t, out.Failures)
			assert.Equal(t, []string{"modules/alpha", "modules/bravo"}, catalogDirs(&out))
		})
	}
}
