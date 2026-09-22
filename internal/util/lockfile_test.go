package util_test

import (
	"os"
	"path/filepath"
	"runtime"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/sync/errgroup"

	"github.com/gruntwork-io/terragrunt/internal/util"
)

func TestLockfileTryLockHeld(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "test.lock")

	first := util.NewLockfile(path)
	require.NoError(t, first.TryLock())

	second := util.NewLockfile(path)
	require.ErrorIs(t, second.TryLock(), util.ErrLockfileHeld)

	require.NoError(t, first.Unlock())

	require.NoError(t, second.TryLock())
	require.NoError(t, second.Unlock())
}

// TestLockfileUnlockKeepsFile pins the no-unlink contract that makes the
// lock race-free across processes: every holder must contend on the
// same inode for the lifetime of the path.
func TestLockfileUnlockKeepsFile(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "test.lock")

	lf := util.NewLockfile(path)
	require.NoError(t, lf.TryLock())
	require.NoError(t, lf.Unlock())

	_, err := os.Stat(path)
	require.NoError(t, err, "lock file must survive release")
}

// TestLockfileMutualExclusionWithRacing runs several holders, each with
// its own handle on one path, through repeated acquire and release
// cycles and asserts that no two are ever inside the lock at once.
func TestLockfileMutualExclusionWithRacing(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "test.lock")

	const (
		holders = 8
		rounds  = 25
	)

	var (
		inside   atomic.Int32
		overlaps atomic.Int32
		entries  atomic.Int32
	)

	var g errgroup.Group

	for range holders {
		g.Go(func() error {
			lf := util.NewLockfile(path)

			for range rounds {
				if err := lf.Lock(); err != nil {
					return err
				}

				if inside.Add(1) != 1 {
					overlaps.Add(1)
				}

				entries.Add(1)
				runtime.Gosched()
				inside.Add(-1)

				if err := lf.Unlock(); err != nil {
					return err
				}
			}

			return nil
		})
	}

	require.NoError(t, g.Wait())
	assert.Zero(t, overlaps.Load(), "two holders were inside the lock at once")
	assert.Equal(t, int32(holders*rounds), entries.Load())
}
