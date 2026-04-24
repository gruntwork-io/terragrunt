package hclparse_test

import (
	"fmt"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/hclparse"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// assertPanicsContaining asserts the given call panics with a message
// containing the expected substring. Uses Contains so the descriptive
// panic texts remain flexible to small wording edits. fmt.Sprint handles
// string, error, fmt.Stringer, and arbitrary panic values uniformly.
func assertPanicsContaining(t *testing.T, want string, fn func()) {
	t.Helper()

	defer func() {
		r := recover()
		require.NotNil(t, r, "expected panic containing %q, got no panic", want)
		assert.Contains(t, fmt.Sprint(r), want)
	}()

	fn()
}

func TestParseStackFile_NilFS_Panics(t *testing.T) {
	t.Parallel()
	assertPanicsContaining(t, "hclparse.ParseStackFile: fs is nil", func() {
		_, _ = hclparse.ParseStackFile(nil, &hclparse.ParseStackFileInput{StackDir: "/x"})
	})
}

func TestParseStackFile_NilInput_Panics(t *testing.T) {
	t.Parallel()
	assertPanicsContaining(t, "hclparse.ParseStackFile: input is nil", func() {
		_, _ = hclparse.ParseStackFile(vfs.NewMemMapFS(), nil)
	})
}

func TestParseStackFile_EmptyStackDir_Panics(t *testing.T) {
	t.Parallel()
	assertPanicsContaining(t, "hclparse.ParseStackFile: input.StackDir is empty", func() {
		_, _ = hclparse.ParseStackFile(vfs.NewMemMapFS(), &hclparse.ParseStackFileInput{})
	})
}

func TestParseStackFileFromPath_NilFS_Panics(t *testing.T) {
	t.Parallel()
	assertPanicsContaining(t, "hclparse.ParseStackFileFromPath: fs is nil", func() {
		_, _ = hclparse.ParseStackFileFromPath(nil, "/x")
	})
}

func TestParseStackFileFromPath_EmptyStackDir_Panics(t *testing.T) {
	t.Parallel()
	assertPanicsContaining(t, "hclparse.ParseStackFileFromPath: stackDir is empty", func() {
		_, _ = hclparse.ParseStackFileFromPath(vfs.NewMemMapFS(), "")
	})
}

func TestUnitPathsFromStackDir_NilFS_Panics(t *testing.T) {
	t.Parallel()
	assertPanicsContaining(t, "hclparse.UnitPathsFromStackDir: fs is nil", func() {
		hclparse.UnitPathsFromStackDir(nil, "/x")
	})
}

func TestUnitPathsFromStackDir_EmptyStackDir_Panics(t *testing.T) {
	t.Parallel()
	assertPanicsContaining(t, "hclparse.UnitPathsFromStackDir: stackDir is empty", func() {
		hclparse.UnitPathsFromStackDir(vfs.NewMemMapFS(), "")
	})
}

func TestDiscoverStackChildUnits_NilFS_Panics(t *testing.T) {
	t.Parallel()
	assertPanicsContaining(t, "hclparse.DiscoverStackChildUnits: fs is nil", func() {
		hclparse.DiscoverStackChildUnits(nil, "/src", "/gen")
	})
}

func TestDiscoverStackChildUnits_EmptySource_Panics(t *testing.T) {
	t.Parallel()
	assertPanicsContaining(t, "hclparse.DiscoverStackChildUnits: stackSourceDir is empty", func() {
		hclparse.DiscoverStackChildUnits(vfs.NewMemMapFS(), "", "/gen")
	})
}

func TestDiscoverStackChildUnits_EmptyGen_Panics(t *testing.T) {
	t.Parallel()
	assertPanicsContaining(t, "hclparse.DiscoverStackChildUnits: stackGenDir is empty", func() {
		hclparse.DiscoverStackChildUnits(vfs.NewMemMapFS(), "/src", "")
	})
}

func TestAutoIncludeDependencyPaths_NilFS_Panics(t *testing.T) {
	t.Parallel()
	assertPanicsContaining(t, "hclparse.AutoIncludeDependencyPaths: fs is nil", func() {
		_, _ = hclparse.AutoIncludeDependencyPaths(nil, "/x")
	})
}

func TestAutoIncludeDependencyPaths_EmptyUnitDir_Panics(t *testing.T) {
	t.Parallel()
	assertPanicsContaining(t, "hclparse.AutoIncludeDependencyPaths: unitDir is empty", func() {
		_, _ = hclparse.AutoIncludeDependencyPaths(vfs.NewMemMapFS(), "")
	})
}

func TestGenerateAutoIncludeFile_NilFS_Panics(t *testing.T) {
	t.Parallel()
	assertPanicsContaining(t, "hclparse.GenerateAutoIncludeFile: fs is nil", func() {
		_ = hclparse.GenerateAutoIncludeFile(nil, nil, "/x", nil, nil)
	})
}

func TestGenerateAutoIncludeFile_EmptyTargetDir_Panics(t *testing.T) {
	t.Parallel()
	assertPanicsContaining(t, "hclparse.GenerateAutoIncludeFile: targetDir is empty", func() {
		_ = hclparse.GenerateAutoIncludeFile(vfs.NewMemMapFS(), nil, "", nil, nil)
	})
}

func TestGenerateAutoIncludeFile_NilResolved_ReturnsNoError(t *testing.T) {
	t.Parallel()

	// Nil resolved is a legitimate no-op (no autoinclude block in stack file).
	err := hclparse.GenerateAutoIncludeFile(vfs.NewMemMapFS(), nil, "/target", nil, nil)
	require.NoError(t, err)
}
