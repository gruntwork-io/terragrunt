package vfs_test

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gruntwork-io/terragrunt/internal/vfs"
)

func TestStatusChangeOfIsAbsentInMemory(t *testing.T) {
	t.Parallel()

	fsys := vfs.NewMemMapFS()
	path := filepath.Join(vfs.MemRoot, "file")
	require.NoError(t, vfs.WriteFile(fsys, path, []byte("content"), 0o644))

	info, err := fsys.Stat(path)
	require.NoError(t, err)

	_, ok := vfs.StatusChangeOf(info)
	assert.False(t, ok)
}
