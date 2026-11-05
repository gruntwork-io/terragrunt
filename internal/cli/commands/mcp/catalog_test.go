package mcp_test

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"unicode/utf8"

	tgmcp "github.com/gruntwork-io/terragrunt/internal/cli/commands/mcp"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/internal/vexec"
	"github.com/gruntwork-io/terragrunt/test/helpers/venvtest"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// catalogToken is the credential in [catalogRemoteURL].
const catalogToken = "s3cr3t-token"

// catalogRemoteURL is a repository the catalog can reach only by running git.
const catalogRemoteURL = "git::https://user:" + catalogToken + "@example.invalid/org/modules.git"

// catalogQueryTokenURL is a repository with its credential in the query, which
// git prints back as it was given.
const catalogQueryTokenURL = "git::https://example.invalid/org/modules.git?token=" + catalogToken

const catalogAlphaReadme = `---
name: Alpha
description: The first module.
---

Alpha body.
`

type catalogComponent struct {
	Kind            string `json:"kind"`
	Title           string `json:"title"`
	Description     string `json:"description"`
	Dir             string `json:"dir"`
	Source          string `json:"source"`
	URL             string `json:"url"`
	ComponentSource string `json:"component_source"`
	Doc             string `json:"doc"`
}

type catalogFailure struct {
	Source string `json:"source"`
	Error  string `json:"error"`
}

type catalogOutput struct {
	Components     []catalogComponent `json:"components"`
	Failures       []catalogFailure   `json:"failures"`
	Degraded       []string           `json:"degraded"`
	ComponentCount int                `json:"component_count"`
}

// catalogTree returns a server root whose root.hcl names urls in its catalog
// block, beside a checked-out repository at ./repo holding the modules alpha
// and bravo, with alphaReadme as alpha's README.
func catalogTree(t *testing.T, alphaReadme string, urls ...string) string {
	t.Helper()

	return writeTree(t, map[string]string{
		"root.hcl":                     "catalog {\n  urls = [\"" + strings.Join(urls, "\", \"") + "\"]\n}\n",
		"repo/.git/HEAD":               "ref: refs/heads/main\n",
		"repo/.git/config":             "[remote \"origin\"]\n\turl = github.com/acme/repo\n",
		"repo/modules/alpha/main.tf":   "\n",
		"repo/modules/alpha/README.md": alphaReadme,
		"repo/modules/bravo/main.tf":   "\n",
	})
}

// newCatalogSession is [newTestSession] with real temp and cache dirs, which a
// catalog load creates its clone and its CAS store in.
func newCatalogSession(t *testing.T, dir string) *mcp.ClientSession {
	t.Helper()

	return newTestSessionWith(t, dir, withCatalogDirs(venvtest.NewWithOSFS(), t.TempDir()), func(*tgmcp.Options) {}, nil)
}

// withCatalogDirs points v's temp and cache dirs under scratch.
func withCatalogDirs(v *venv.Venv, scratch string) *venv.Venv {
	return v.
		WithTempDir(func() string { return scratch }).
		WithUserCacheDir(func() (string, error) { return filepath.Join(scratch, "cache"), nil })
}

// newFailingGitSession serves a client under --allow=exec with a git that fails
// every command with the stderr that say returns for its arguments.
func newFailingGitSession(t *testing.T, dir string, say func(args []string) string) *mcp.ClientSession {
	t.Helper()

	failGit := func(_ context.Context, inv vexec.Invocation) vexec.Result {
		return vexec.Result{ExitCode: 128, Stderr: []byte(say(inv.Args))}
	}

	v := withCatalogDirs(venvtest.NewWithOSFS().WithExec(vexec.NewMemExec(failGit)), t.TempDir())

	return newTestSessionWith(t, dir, v, func(o *tgmcp.Options) { o.Allow = []string{"exec"} }, nil)
}

// catalogMarshal returns out as the agent receives it.
func catalogMarshal(t *testing.T, out *catalogOutput) string {
	t.Helper()

	data, err := json.Marshal(out)
	require.NoError(t, err)

	return string(data)
}

// catalogDirs returns the dir of each component, sorted: repositories load
// concurrently, so the order components arrive in varies between runs.
func catalogDirs(out *catalogOutput) []string {
	dirs := make([]string, 0, len(out.Components))

	for i := range out.Components {
		dirs = append(dirs, out.Components[i].Dir)
	}

	slices.Sort(dirs)

	return dirs
}

// TestCatalogListsTheConfiguredCatalog pins that the catalog tool returns the
// components of the repository the catalog block names, with every capability
// denied, and leaves out README bodies until a call asks for them.
func TestCatalogListsTheConfiguredCatalog(t *testing.T) {
	t.Parallel()

	dir := catalogTree(t, catalogAlphaReadme, "./repo")
	session := newCatalogSession(t, dir)

	var out catalogOutput

	callTool(t, session, "catalog", map[string]any{}, &out)

	assert.Equal(t, []string{"modules/alpha", "modules/bravo"}, catalogDirs(&out))
	assert.Equal(t, 2, out.ComponentCount)
	assert.Empty(t, out.Failures)
	assert.Empty(t, out.Degraded)

	for _, c := range out.Components {
		assert.Equal(t, "module", c.Kind, c.Dir)
		assert.Empty(t, c.Doc, c.Dir)
	}

	var withDocs catalogOutput

	callTool(t, session, "catalog", map[string]any{"include_docs": true}, &withDocs)

	i := slices.IndexFunc(withDocs.Components, func(c catalogComponent) bool { return c.Dir == "modules/alpha" })
	require.GreaterOrEqual(t, i, 0, "alpha is missing: %v", withDocs.Components)

	alpha := withDocs.Components[i]
	assert.Equal(t, "Alpha", alpha.Title)
	assert.Equal(t, "The first module.", alpha.Description)
	assert.Equal(t, "Alpha body.\n", alpha.Doc)
	assert.Equal(t, filepath.Join(dir, "repo")+"//modules/alpha", alpha.ComponentSource)
}

// TestCatalogReportsTheKindOfEachComponent pins that the tool lists every kind
// of component a repository can hold.
func TestCatalogReportsTheKindOfEachComponent(t *testing.T) {
	t.Parallel()

	session := newCatalogSession(t, writeTree(t, map[string]string{
		"root.hcl":                      "catalog {\n  urls = [\"./repo\"]\n}\n",
		"repo/.git/HEAD":                "ref: refs/heads/main\n",
		"repo/.git/config":              "[remote \"origin\"]\n\turl = github.com/acme/repo\n",
		"repo/modules/a-module/main.tf": "\n",
		"repo/modules/a-template/.boilerplate/boilerplate.yml": "variables: []\n",
		"repo/modules/a-unit/terragrunt.hcl":                   "\n",
		"repo/modules/a-stack/terragrunt.stack.hcl":            "\n",
	}))

	var out catalogOutput

	callTool(t, session, "catalog", map[string]any{}, &out)

	kinds := map[string]string{}
	for _, c := range out.Components {
		kinds[c.Dir] = c.Kind
	}

	assert.Equal(t, map[string]string{
		"modules/a-module":   "module",
		"modules/a-template": "template",
		"modules/a-unit":     "unit",
		"modules/a-stack":    "stack",
	}, kinds)
}

// TestCatalogFindsTheBlockInARootTerragruntHCL pins that a tree whose root
// config is still named terragrunt.hcl is read the way root.hcl is.
func TestCatalogFindsTheBlockInARootTerragruntHCL(t *testing.T) {
	t.Parallel()

	session := newCatalogSession(t, writeTree(t, map[string]string{
		"terragrunt.hcl":             "catalog {\n  urls = [\"./repo\"]\n}\n",
		"repo/.git/HEAD":             "ref: refs/heads/main\n",
		"repo/.git/config":           "[remote \"origin\"]\n\turl = github.com/acme/repo\n",
		"repo/modules/alpha/main.tf": "\n",
	}))

	var out catalogOutput

	callTool(t, session, "catalog", map[string]any{}, &out)

	assert.Equal(t, []string{"modules/alpha"}, catalogDirs(&out))
}

// TestCatalogReadsTheBlockFromTheWorkingDir pins that working_dir picks which
// configuration is read: the tree's root names no catalog, and team does.
func TestCatalogReadsTheBlockFromTheWorkingDir(t *testing.T) {
	t.Parallel()

	session := newCatalogSession(t, writeTree(t, map[string]string{
		"team/root.hcl":                   "catalog {\n  urls = [\"./repo\"]\n}\n",
		"team/repo/.git/HEAD":             "ref: refs/heads/main\n",
		"team/repo/.git/config":           "[remote \"origin\"]\n\turl = github.com/acme/repo\n",
		"team/repo/modules/alpha/main.tf": "\n",
	}))

	var atRoot, inTeam catalogOutput

	callTool(t, session, "catalog", map[string]any{}, &atRoot)
	assert.Empty(t, atRoot.Components)

	callTool(t, session, "catalog", map[string]any{"working_dir": "team"}, &inTeam)
	assert.Equal(t, []string{"modules/alpha"}, catalogDirs(&inTeam))
}

// TestCatalogLoadsTheRepositoriesUnitSourcesPointAt pins that a tree with no
// catalog block still lists what its units' terraform sources name.
func TestCatalogLoadsTheRepositoriesUnitSourcesPointAt(t *testing.T) {
	t.Parallel()

	session := newCatalogSession(t, writeTree(t, map[string]string{
		"app/terragrunt.hcl": "terraform {\n  source = \"github.com/acme/modules//vpc\"\n}\n",
	}))

	var out catalogOutput

	callTool(t, session, "catalog", map[string]any{}, &out)

	require.Len(t, out.Failures, 1)
	assert.Equal(t, "github.com/acme/modules", out.Failures[0].Source)
}

// TestCatalogLimitCapsComponentsButNotTheCount pins that limit bounds the
// components returned while component_count still counts all of them, so the
// agent can tell the list was cut.
func TestCatalogLimitCapsComponentsButNotTheCount(t *testing.T) {
	t.Parallel()

	session := newCatalogSession(t, catalogTree(t, catalogAlphaReadme, "./repo"))

	var out catalogOutput

	callTool(t, session, "catalog", map[string]any{"limit": 1}, &out)

	assert.Len(t, out.Components, 1)
	assert.Equal(t, 2, out.ComponentCount)
}

// TestCatalogLimitDefaultsToItsMaximum pins that a call naming no limit stops
// at 500 components, and that 500 is itself a limit a call can ask for.
func TestCatalogLimitDefaultsToItsMaximum(t *testing.T) {
	t.Parallel()

	const found = 501

	files := map[string]string{
		"root.hcl":         "catalog {\n  urls = [\"./repo\"]\n}\n",
		"repo/.git/HEAD":   "ref: refs/heads/main\n",
		"repo/.git/config": "[remote \"origin\"]\n\turl = github.com/acme/repo\n",
	}

	for i := range found {
		files[fmt.Sprintf("repo/modules/m%d/main.tf", i)] = "\n"
	}

	session := newCatalogSession(t, writeTree(t, files))

	for _, args := range []map[string]any{{}, {"limit": 500}} {
		var out catalogOutput

		callTool(t, session, "catalog", args, &out)

		assert.Len(t, out.Components, 500)
		assert.Equal(t, found, out.ComponentCount)
	}
}

func TestCatalogRejectsALimitOutOfRange(t *testing.T) {
	t.Parallel()

	session := newCatalogSession(t, catalogTree(t, catalogAlphaReadme, "./repo"))

	for _, limit := range []int{-1, 501, 1_000_000} {
		res, err := session.CallTool(t.Context(), &mcp.CallToolParams{
			Name:      "catalog",
			Arguments: map[string]any{"limit": limit},
		})
		require.NoError(t, err)
		assert.True(t, res.IsError, "limit %d must be refused", limit)
	}
}

// TestCatalogCutsALongReadme pins that a README body comes back bounded, since
// the repository decides how long it is.
func TestCatalogCutsALongReadme(t *testing.T) {
	t.Parallel()

	body := strings.Repeat("x", 100_000)
	session := newCatalogSession(t, catalogTree(t, "---\nname: Alpha\n---\n\n"+body+"\n", "./repo"))

	var out catalogOutput

	callTool(t, session, "catalog", map[string]any{"include_docs": true}, &out)

	i := slices.IndexFunc(out.Components, func(c catalogComponent) bool { return c.Dir == "modules/alpha" })
	require.GreaterOrEqual(t, i, 0, "alpha is missing: %v", out.Components)

	doc := out.Components[i].Doc
	assert.LessOrEqual(t, utf8.RuneCountInString(doc), 16_001, "16,000 runes of README and the mark of the cut")
	assert.True(t, strings.HasSuffix(doc, "…"), "a cut README must say it was cut")
}

// TestCatalogKeepsACredentialOutOfAFailedClonesError pins that a credential in
// a repository's query stays out of the result when git prints the URL back.
func TestCatalogKeepsACredentialOutOfAFailedClonesError(t *testing.T) {
	t.Parallel()

	session := newFailingGitSession(t, catalogTree(t, catalogAlphaReadme, catalogQueryTokenURL),
		func(args []string) string {
			i := slices.Index(args, "--")
			if i < 0 {
				return "fatal: failed"
			}

			return "fatal: unable to access '" + args[i+1] + "/': Could not resolve host\n"
		})

	var out catalogOutput

	callTool(t, session, "catalog", map[string]any{}, &out)

	require.Len(t, out.Failures, 1)
	assert.NotContains(t, catalogMarshal(t, &out), catalogToken)
}

// TestCatalogBoundsTheErrorOfAFailedClone pins that a failed clone's error
// comes back cut, since a remote decides how much git prints.
func TestCatalogBoundsTheErrorOfAFailedClone(t *testing.T) {
	t.Parallel()

	session := newFailingGitSession(t, catalogTree(t, catalogAlphaReadme, "git::https://example.invalid/m.git"),
		func([]string) string { return strings.Repeat("x", 100_000) })

	var out catalogOutput

	callTool(t, session, "catalog", map[string]any{}, &out)

	require.Len(t, out.Failures, 1)
	assert.LessOrEqual(t, utf8.RuneCountInString(out.Failures[0].Error), 1001)
}

// TestCatalogReportsABlockItCannotRead pins that a broken catalog block shows
// up in failures, so an agent can tell it from a catalog with nothing in it.
func TestCatalogReportsABlockItCannotRead(t *testing.T) {
	t.Parallel()

	for name, config := range map[string]string{
		"syntax error":     "catalog {\n  urls = [\n",
		"wrong type":       "catalog {\n  urls = \"./repo\"\n}\n",
		"unknown function": "catalog {\n  urls = [nope()]\n}\n",
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			session := newCatalogSession(t, writeTree(t, map[string]string{"root.hcl": config}))

			var out catalogOutput

			callTool(t, session, "catalog", map[string]any{}, &out)

			assert.Empty(t, out.Components)
			require.Len(t, out.Failures, 1)
			assert.Equal(t, "root.hcl", out.Failures[0].Source)
			assert.NotEmpty(t, out.Failures[0].Error)
		})
	}
}

// TestCatalogStillTriesOtherSourcesWhenTheBlockIsUnreadable pins that a broken
// catalog block does not stop the repositories discovery found elsewhere.
func TestCatalogStillTriesOtherSourcesWhenTheBlockIsUnreadable(t *testing.T) {
	t.Parallel()

	session := newCatalogSession(t, writeTree(t, map[string]string{
		"root.hcl":           "catalog {\n  urls = [\n",
		"app/terragrunt.hcl": "terraform {\n  source = \"github.com/acme/modules//vpc\"\n}\n",
	}))

	var out catalogOutput

	callTool(t, session, "catalog", map[string]any{}, &out)

	require.Len(t, out.Failures, 2)
	assert.Equal(t, "github.com/acme/modules", out.Failures[0].Source)
	assert.Equal(t, "root.hcl", out.Failures[1].Source)
}

// TestCatalogIsAnnotatedAsAReadOnlyTool pins the hints a client reads to decide
// whether a call needs a person's approval.
func TestCatalogIsAnnotatedAsAReadOnlyTool(t *testing.T) {
	t.Parallel()

	session := newCatalogSession(t, catalogTree(t, catalogAlphaReadme, "./repo"))

	res, err := session.ListTools(t.Context(), nil)
	require.NoError(t, err)

	i := slices.IndexFunc(res.Tools, func(tool *mcp.Tool) bool { return tool.Name == "catalog" })
	require.GreaterOrEqual(t, i, 0)

	assert.True(t, res.Tools[i].Annotations.ReadOnlyHint)
	assert.True(t, res.Tools[i].Annotations.IdempotentHint)
}

// TestCatalogReportsARepositoryItCannotCloneWithoutAllowExecWithRacing pins
// that a remote repository is not cloned when the server was not started with
// --allow=exec. The refusal is reported without the credential the URL holds,
// and the components of the local repository, which loads alongside it, still
// come back.
func TestCatalogReportsARepositoryItCannotCloneWithoutAllowExecWithRacing(t *testing.T) {
	t.Parallel()

	session := newCatalogSession(t, catalogTree(t, catalogAlphaReadme, "./repo", catalogRemoteURL))

	var out catalogOutput

	callTool(t, session, "catalog", map[string]any{}, &out)

	assert.Equal(t, []string{"modules/alpha", "modules/bravo"}, catalogDirs(&out))

	require.Len(t, out.Failures, 1)
	assert.Contains(t, out.Failures[0].Source, "example.invalid/org/modules.git")
	assert.NotContains(t, out.Failures[0].Source, catalogToken)
	assert.NotContains(t, out.Failures[0].Error, catalogToken)

	assert.True(t, slices.ContainsFunc(out.Degraded, func(note string) bool {
		return strings.HasPrefix(note, `denied "git `) &&
			strings.HasSuffix(note, `": sources that need it were not downloaded`)
	}), "no note reports the refused clone: %v", out.Degraded)

	for _, note := range out.Degraded {
		assert.NotContains(t, note, catalogToken)
	}
}

func TestCatalogIsConfinedToTheServerRoot(t *testing.T) {
	t.Parallel()

	session := newCatalogSession(t, catalogTree(t, catalogAlphaReadme, "./repo"))

	res, err := session.CallTool(t.Context(), &mcp.CallToolParams{
		Name:      "catalog",
		Arguments: map[string]any{"working_dir": ".."},
	})
	require.NoError(t, err)
	assert.True(t, res.IsError, "a catalog read outside the server root must be refused")
}
