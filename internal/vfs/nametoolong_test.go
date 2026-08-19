package vfs_test

import (
	"errors"
	"io/fs"
	"os"
	"strings"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TooLongName is a name well past any platform's per-component limit.
const TooLongName = 700

func TestIsNameTooLongClassifiesUnusableName(t *testing.T) {
	t.Parallel()

	// Stat'd for real so the error is whatever the OS produces rather than a
	// stand-in. Platforms disagree on which code that is: what has to hold
	// everywhere is that a name too long to use never reaches a caller as a
	// failure to read something that might have been there.
	_, err := os.Stat(strings.Repeat("A", TooLongName))
	require.Error(t, err)

	assert.True(t, vfs.IsNameTooLong(err) || errors.Is(err, fs.ErrNotExist))
}

func TestIsNameTooLongRejectsOtherErrors(t *testing.T) {
	t.Parallel()

	_, err := os.Stat("does-not-exist")
	require.Error(t, err)

	assert.False(t, vfs.IsNameTooLong(err))
	assert.False(t, vfs.IsNameTooLong(os.ErrPermission))
	assert.False(t, vfs.IsNameTooLong(nil))
}
