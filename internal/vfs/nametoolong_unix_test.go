//go:build !windows

package vfs_test

import (
	"io/fs"
	"os"
	"strings"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIsNameTooLongIsDistinctFromNotExist(t *testing.T) {
	t.Parallel()

	_, err := os.Stat(strings.Repeat("A", TooLongName))
	require.Error(t, err)

	// The distinction the callers rely on: too long is not the same as absent,
	// so a not-exist check alone cannot stand in for this one.
	assert.True(t, vfs.IsNameTooLong(err))
	assert.NotErrorIs(t, err, fs.ErrNotExist)
}
