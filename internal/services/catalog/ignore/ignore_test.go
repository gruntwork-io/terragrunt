package ignore_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/services/catalog/ignore"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMatcher_MissingFileIsEmpty(t *testing.T) {
	t.Parallel()

	m, err := ignore.Load(t.TempDir())
	require.NoError(t, err)
	require.NotNil(t, m)
	assert.True(t, m.Empty())
	assert.False(t, m.Match("examples"))
}

func TestMatcher_RepoRootNeverMatches(t *testing.T) {
	t.Parallel()

	m, err := ignore.Parse(strings.NewReader("**\n"))
	require.NoError(t, err)
	assert.False(t, m.Match(""))
}

func TestMatcher_CommentsAndBlanks(t *testing.T) {
	t.Parallel()

	m, err := ignore.Parse(strings.NewReader("\n# this is a comment\n\nexamples\n"))
	require.NoError(t, err)
	assert.True(t, m.Match("examples"))
	assert.False(t, m.Match("modules/vpc"))
}

func TestMatcher_TrailingSlashStripped(t *testing.T) {
	t.Parallel()

	m, err := ignore.Parse(strings.NewReader("examples/\n"))
	require.NoError(t, err)
	assert.True(t, m.Match("examples"))
}

func TestMatcher_SeparatorAwareness(t *testing.T) {
	t.Parallel()

	// Single * does not cross /.
	m, err := ignore.Parse(strings.NewReader("examples/*\n"))
	require.NoError(t, err)

	assert.True(t, m.Match("examples/vpc"))
	assert.False(t, m.Match("examples/vpc/sub"), "single * must not cross /")

	// ** does cross /.
	m2, err := ignore.Parse(strings.NewReader("examples/**\n"))
	require.NoError(t, err)
	assert.True(t, m2.Match("examples/vpc"))
	assert.True(t, m2.Match("examples/vpc/sub"))
}

func TestMatcher_NegationLastWins(t *testing.T) {
	t.Parallel()

	m, err := ignore.Parse(strings.NewReader("test/**\n!test/keep\n"))
	require.NoError(t, err)

	assert.True(t, m.Match("test/drop"))
	assert.False(t, m.Match("test/keep"))
}

func TestMatcher_InvalidPatternReturnsError(t *testing.T) {
	t.Parallel()

	_, err := ignore.Parse(strings.NewReader("[\n"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "line 1")
}

func TestLoadFile_MissingFileIsError(t *testing.T) {
	t.Parallel()

	_, err := ignore.LoadFile(filepath.Join(t.TempDir(), "does-not-exist"))
	require.Error(t, err)
}

func TestLoadFile_ReadsExistingFile(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "extra-ignore")
	require.NoError(t, os.WriteFile(path, []byte("examples\n"), 0644))

	m, err := ignore.LoadFile(path)
	require.NoError(t, err)
	assert.True(t, m.Match("examples"))
}

func TestMerge_AppendsRulesLastWins(t *testing.T) {
	t.Parallel()

	base, err := ignore.Parse(strings.NewReader("examples\nstash/**\n"))
	require.NoError(t, err)

	extra, err := ignore.Parse(strings.NewReader("integration/**\n!stash/keep\n"))
	require.NoError(t, err)

	base.Merge(extra)

	assert.True(t, base.Match("examples"))
	assert.True(t, base.Match("integration/vpc"))
	assert.False(t, base.Match("stash/keep"), "extra negation should re-include stash/keep")
	assert.True(t, base.Match("stash/drop"))
}

func TestLoad_ReadsFile(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, ignore.FileName), []byte("examples\n"), 0644))

	m, err := ignore.Load(dir)
	require.NoError(t, err)
	assert.False(t, m.Empty())
	assert.True(t, m.Match("examples"))
}
