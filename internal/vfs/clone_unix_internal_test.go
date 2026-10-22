//go:build linux || darwin

package vfs

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/sys/unix"
)

func TestCloneErr(t *testing.T) {
	t.Parallel()

	t.Run("an errno meaning no clone is possible reads as ErrNoCloneFile", func(t *testing.T) {
		t.Parallel()

		for _, errno := range cloneUnsupportedErrnos {
			err := cloneErr(errno)

			require.ErrorIs(t, err, ErrNoCloneFile)
			require.ErrorIs(t, err, errno, "the original errno must stay matchable")
		}
	})

	t.Run("any other error is returned unchanged", func(t *testing.T) {
		t.Parallel()

		err := cloneErr(unix.EACCES)

		assert.Equal(t, unix.EACCES, err)
		assert.NotErrorIs(t, err, ErrNoCloneFile)
	})
}
