package mcp_test

import (
	"path/filepath"
	"slices"
	"testing"

	tgmcp "github.com/gruntwork-io/terragrunt/internal/cli/commands/mcp"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	unformattedHCL = "inputs   =   {}\n"
	formattedHCL   = "inputs = {}\n"
)

type fmtFile struct {
	Path string `json:"path"`
	Diff string `json:"diff"`
}

type fmtOutput struct {
	Changed     []fmtFile `json:"changed"`
	ParseErrors []string  `json:"parse_errors"`
	Skipped     []string  `json:"skipped"`
	FileCount   int       `json:"file_count"`
	Formatted   bool      `json:"formatted"`
}

func TestFmtCheckReportsWithoutWriting(t *testing.T) {
	t.Parallel()

	dir := writeTree(t, map[string]string{
		"root.hcl":            formattedHCL,
		"unit/terragrunt.hcl": unformattedHCL,
	})

	session := newTestSession(t, dir)

	var out fmtOutput

	callTool(t, session, "fmt", map[string]any{"check": true, "diff": true}, &out)

	assert.False(t, out.Formatted)
	assert.Equal(t, 2, out.FileCount)
	require.Len(t, out.Changed, 1)
	assert.Equal(t, filepath.Join("unit", "terragrunt.hcl"), out.Changed[0].Path)
	assert.Contains(t, out.Changed[0].Diff, "+"+formattedHCL)
	assert.Equal(t, unformattedHCL, readTreeFile(t, filepath.Join(dir, "unit", "terragrunt.hcl")))
}

func TestFmtRewritesUnformattedFiles(t *testing.T) {
	t.Parallel()

	dir := writeTree(t, map[string]string{
		"a/terragrunt.hcl": unformattedHCL,
		"b/terragrunt.hcl": unformattedHCL,
	})

	before, err := vfs.NewOSFS().Stat(filepath.Join(dir, "a", "terragrunt.hcl"))
	require.NoError(t, err)

	session := newTestSession(t, dir)

	var first fmtOutput

	callTool(t, session, "fmt", map[string]any{"filter": []string{"./a/**"}}, &first)

	assert.Equal(t, []fmtFile{{Path: filepath.Join("a", "terragrunt.hcl")}}, first.Changed)
	assert.Equal(t, formattedHCL, readTreeFile(t, filepath.Join(dir, "a", "terragrunt.hcl")))
	assert.Equal(t, unformattedHCL, readTreeFile(t, filepath.Join(dir, "b", "terragrunt.hcl")))

	after, err := vfs.NewOSFS().Stat(filepath.Join(dir, "a", "terragrunt.hcl"))
	require.NoError(t, err)
	assert.Equal(t, before.Mode(), after.Mode(), "rewriting a file must keep its mode")

	var second fmtOutput

	callTool(t, session, "fmt", map[string]any{"file": "b/terragrunt.hcl"}, &second)

	assert.Equal(t, []fmtFile{{Path: filepath.Join("b", "terragrunt.hcl")}}, second.Changed)
	assert.Equal(t, formattedHCL, readTreeFile(t, filepath.Join(dir, "b", "terragrunt.hcl")))

	var again fmtOutput

	callTool(t, session, "fmt", map[string]any{"check": true}, &again)

	assert.True(t, again.Formatted)
	assert.Empty(t, again.Changed)
}

func TestFmtLeavesAFileThatDoesNotParse(t *testing.T) {
	t.Parallel()

	const broken = "inputs   =   {\n"

	dir := writeTree(t, map[string]string{
		"broken/terragrunt.hcl": broken,
		"ok/terragrunt.hcl":     unformattedHCL,
	})

	session := newTestSession(t, dir)

	var out fmtOutput

	callTool(t, session, "fmt", map[string]any{}, &out)

	assert.False(t, out.Formatted)
	assert.Len(t, out.ParseErrors, 1)
	assert.Equal(t, []fmtFile{{Path: filepath.Join("ok", "terragrunt.hcl")}}, out.Changed)
	assert.Equal(t, broken, readTreeFile(t, filepath.Join(dir, "broken", "terragrunt.hcl")))
	assert.Equal(t, formattedHCL, readTreeFile(t, filepath.Join(dir, "ok", "terragrunt.hcl")))
}

func TestFmtIsConfinedToTheServerRoot(t *testing.T) {
	t.Parallel()

	parent := writeTree(t, map[string]string{
		"outside/terragrunt.hcl": unformattedHCL,
		"root/unit/root.hcl":     formattedHCL,
	})
	dir := filepath.Join(parent, "root")

	session := newTestSession(t, dir)

	for _, args := range []map[string]any{
		{"working_dir": ".."},
		{"file": "../outside/terragrunt.hcl"},
		{"file": filepath.Join(parent, "outside", "terragrunt.hcl")},
		{"working_dir": "unit", "file": "../../outside/terragrunt.hcl"},
		{"file": "unit/root.hcl", "filter": []string{"./unit/**"}},
		{"file": "unit"},
	} {
		res, err := session.CallTool(t.Context(), &mcp.CallToolParams{Name: "fmt", Arguments: args})
		require.NoError(t, err)
		assert.True(t, res.IsError, "fmt with %v must be refused", args)
	}

	assert.Equal(t, unformattedHCL, readTreeFile(t, filepath.Join(parent, "outside", "terragrunt.hcl")))
}

func TestFmtDoesNotFollowSymlinksOutOfTheServerRoot(t *testing.T) {
	t.Parallel()

	fsys := vfs.NewOSFS()
	outside := writeTree(t, map[string]string{"terragrunt.hcl": unformattedHCL})
	dir := writeTree(t, map[string]string{"unit/terragrunt.hcl": formattedHCL})

	require.NoError(t, vfs.Symlink(fsys, filepath.Join(outside, "terragrunt.hcl"), filepath.Join(dir, "link.hcl")))
	require.NoError(t, vfs.Symlink(fsys, outside, filepath.Join(dir, "linkdir")))

	session := newTestSession(t, dir)

	var out fmtOutput

	callTool(t, session, "fmt", map[string]any{}, &out)

	assert.Equal(t, []string{"link.hcl"}, out.Skipped)
	assert.Empty(t, out.Changed)

	var single fmtOutput

	callTool(t, session, "fmt", map[string]any{"file": "link.hcl"}, &single)

	assert.Equal(t, []string{"link.hcl"}, single.Skipped)

	res, err := session.CallTool(t.Context(), &mcp.CallToolParams{
		Name:      "fmt",
		Arguments: map[string]any{"file": "linkdir/terragrunt.hcl"},
	})
	require.NoError(t, err)
	assert.True(t, res.IsError, "a file under a symlinked directory that leaves the root must be refused")

	assert.Equal(t, unformattedHCL, readTreeFile(t, filepath.Join(outside, "terragrunt.hcl")))
}

// TestFmtIsAnnotatedAsALocalWrite pins the annotations a client reads before
// letting a model call fmt: it changes files, but only by reformatting them,
// and never leaves the machine whatever the server was granted.
func TestFmtIsAnnotatedAsALocalWrite(t *testing.T) {
	t.Parallel()

	session := newTestSessionWithOptions(t, newTestTree(t), func(o *tgmcp.Options) {
		o.Allow = []string{"exec", "http"}
	})

	res, err := session.ListTools(t.Context(), nil)
	require.NoError(t, err)

	idx := slices.IndexFunc(res.Tools, func(tool *mcp.Tool) bool { return tool.Name == "fmt" })
	require.GreaterOrEqual(t, idx, 0, "the fmt tool must be advertised")

	annotations := res.Tools[idx].Annotations

	assert.False(t, annotations.ReadOnlyHint)
	assert.True(t, annotations.IdempotentHint)
	require.NotNil(t, annotations.DestructiveHint)
	assert.False(t, *annotations.DestructiveHint)
	require.NotNil(t, annotations.OpenWorldHint)
	assert.False(t, *annotations.OpenWorldHint)
}
