package hclparse_test

import (
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/hclparse"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseStackFile_NilFS_Panics(t *testing.T) {
	t.Parallel()

	assert.PanicsWithValue(t, "hclparse: fs must not be nil", func() {
		_, _ = hclparse.ParseStackFile(nil, &hclparse.ParseStackFileInput{StackDir: "/x"})
	})
}

func TestParseStackFile_NilInput_Panics(t *testing.T) {
	t.Parallel()

	assert.PanicsWithValue(t, "hclparse: input must not be nil", func() {
		_, _ = hclparse.ParseStackFile(vfs.NewMemMapFS(), nil)
	})
}

func TestParseStackFile_EmptyStackDir_Panics(t *testing.T) {
	t.Parallel()

	assert.PanicsWithValue(t, "hclparse: input.StackDir must not be empty", func() {
		_, _ = hclparse.ParseStackFile(vfs.NewMemMapFS(), &hclparse.ParseStackFileInput{})
	})
}

func TestParseStackFileFromPath_NilFS_Panics(t *testing.T) {
	t.Parallel()

	assert.PanicsWithValue(t, "hclparse: fs must not be nil", func() {
		_, _ = hclparse.ParseStackFileFromPath(nil, "/x")
	})
}

func TestParseStackFileFromPath_EmptyStackDir_Panics(t *testing.T) {
	t.Parallel()

	assert.PanicsWithValue(t, "hclparse: stackDir must not be empty", func() {
		_, _ = hclparse.ParseStackFileFromPath(vfs.NewMemMapFS(), "")
	})
}

func TestUnitPathsFromStackDir_NilFS_Panics(t *testing.T) {
	t.Parallel()

	assert.PanicsWithValue(t, "hclparse: fs must not be nil", func() {
		hclparse.UnitPathsFromStackDir(nil, "/x")
	})
}

func TestUnitPathsFromStackDir_EmptyStackDir_Panics(t *testing.T) {
	t.Parallel()

	assert.PanicsWithValue(t, "hclparse: stackDir must not be empty", func() {
		hclparse.UnitPathsFromStackDir(vfs.NewMemMapFS(), "")
	})
}

func TestDiscoverStackChildUnits_NilFS_Panics(t *testing.T) {
	t.Parallel()

	assert.PanicsWithValue(t, "hclparse: fs must not be nil", func() {
		hclparse.DiscoverStackChildUnits(nil, "/src", "/gen")
	})
}

func TestDiscoverStackChildUnits_EmptySource_Panics(t *testing.T) {
	t.Parallel()

	assert.PanicsWithValue(t, "hclparse: stackSourceDir must not be empty", func() {
		hclparse.DiscoverStackChildUnits(vfs.NewMemMapFS(), "", "/gen")
	})
}

func TestDiscoverStackChildUnits_EmptyGen_Panics(t *testing.T) {
	t.Parallel()

	assert.PanicsWithValue(t, "hclparse: stackGenDir must not be empty", func() {
		hclparse.DiscoverStackChildUnits(vfs.NewMemMapFS(), "/src", "")
	})
}

func TestAutoIncludeDependencyPaths_NilFS_Panics(t *testing.T) {
	t.Parallel()

	assert.PanicsWithValue(t, "hclparse: fs must not be nil", func() {
		_, _ = hclparse.AutoIncludeDependencyPaths(nil, "/x")
	})
}

func TestAutoIncludeDependencyPaths_EmptyUnitDir_Panics(t *testing.T) {
	t.Parallel()

	assert.PanicsWithValue(t, "hclparse: unitDir must not be empty", func() {
		_, _ = hclparse.AutoIncludeDependencyPaths(vfs.NewMemMapFS(), "")
	})
}

func TestGenerateAutoIncludeFile_NilFS_Panics(t *testing.T) {
	t.Parallel()

	assert.PanicsWithValue(t, "hclparse: fs must not be nil", func() {
		_ = hclparse.GenerateAutoIncludeFile(nil, nil, "/x", nil, nil)
	})
}

func TestGenerateAutoIncludeFile_EmptyTargetDir_Panics(t *testing.T) {
	t.Parallel()

	assert.PanicsWithValue(t, "hclparse: targetDir must not be empty", func() {
		_ = hclparse.GenerateAutoIncludeFile(vfs.NewMemMapFS(), nil, "", nil, nil)
	})
}

func TestGenerateAutoIncludeFile_NilResolved_ReturnsNoError(t *testing.T) {
	t.Parallel()

	// Nil resolved is a legitimate no-op (no autoinclude block in stack file).
	err := hclparse.GenerateAutoIncludeFile(vfs.NewMemMapFS(), nil, "/target", nil, nil)
	require.NoError(t, err)
}
