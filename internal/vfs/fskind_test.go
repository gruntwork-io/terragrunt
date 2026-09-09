package vfs_test

import (
	"path/filepath"
	"runtime"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/stretchr/testify/assert"
)

func TestDetectFSKind(t *testing.T) {
	t.Parallel()

	t.Run("temp dir resolves to a known kind", func(t *testing.T) {
		t.Parallel()

		switch runtime.GOOS {
		case "linux", "darwin", "windows":
		default:
			t.Skip("no filesystem probe on " + runtime.GOOS)
		}

		kind := vfs.DetectFSKind(vfs.NewOSFS(), t.TempDir())

		assert.NotEqual(t, vfs.FSUnknown, kind,
			"the filesystem behind the temp dir is unrecognized; add it to the probe if it is worth tuning for")
	})

	// A path the probe cannot describe is worth re-asking about at an
	// ancestor; one on an unnamed filesystem is not. Only the first
	// reports FSUnprobed.
	t.Run("missing path is unprobed, not unknown", func(t *testing.T) {
		t.Parallel()

		missing := filepath.Join(t.TempDir(), "no-such-dir", "deeper")

		assert.Equal(t, vfs.FSUnprobed, vfs.DetectFSKind(vfs.NewOSFS(), missing))
	})

	t.Run("a filesystem that is not the real disk never reaches the kernel", func(t *testing.T) {
		t.Parallel()

		// The path exists on the real disk, so a probe that ignored fsys
		// would answer with the host's filesystem instead of admitting
		// the caller is not on it.
		real := t.TempDir()

		assert.Equal(t, vfs.FSVirtual, vfs.DetectFSKind(vfs.NewMemMapFS(), real))
		assert.NotEqual(t, vfs.FSVirtual, vfs.DetectFSKind(vfs.NewOSFS(), real))
		assert.Equal(t, vfs.DefaultFSWorkers, vfs.FSWorkersFor(vfs.NewMemMapFS(), real))
	})

	t.Run("kind names are stable", func(t *testing.T) {
		t.Parallel()

		testCases := []struct {
			want string
			kind vfs.FSKind
		}{
			{kind: vfs.FSUnknown, want: "unknown"},
			{kind: vfs.FSUnprobed, want: "unprobed"},
			{kind: vfs.FSVirtual, want: "virtual"},
			{kind: vfs.FSAPFS, want: "apfs"},
			{kind: vfs.FSExt4, want: "ext4"},
			{kind: vfs.FSXFS, want: "xfs"},
			{kind: vfs.FSBtrfs, want: "btrfs"},
			{kind: vfs.FSOverlay, want: "overlayfs"},
			{kind: vfs.FSNetwork, want: "network"},
		}

		for _, tc := range testCases {
			assert.Equal(t, tc.want, tc.kind.String())
		}
	})

	// A kind added without a name would otherwise report "unknown" and
	// pass for a filesystem the probe could not place.
	t.Run("every declared kind is named", func(t *testing.T) {
		t.Parallel()

		seen := make(map[string]vfs.FSKind, len(vfs.FSKinds()))

		for _, kind := range vfs.FSKinds() {
			name := kind.String()

			assert.NotEmpty(t, name, "kind %d has no name", kind)

			if kind != vfs.FSUnknown {
				assert.NotEqual(t, vfs.FSUnknown.String(), name,
					"kind %d reports itself as unknown", kind)
			}

			if other, ok := seen[name]; ok {
				t.Errorf("kinds %d and %d share the name %q", other, kind, name)
			}

			seen[name] = kind
		}
	})

	t.Run("a kind outside the declared set panics", func(t *testing.T) {
		t.Parallel()

		assert.Panics(t, func() {
			_ = vfs.FSKind(200).String()
		})
	})
}

func TestFSWorkersForKind(t *testing.T) {
	t.Parallel()

	t.Run("measured filesystems get their own ceiling", func(t *testing.T) {
		t.Parallel()

		assert.Equal(t, vfs.MaxFSWorkers, vfs.FSWorkersForKind(vfs.FSExt4))
		assert.Equal(t, vfs.XFSWorkers, vfs.FSWorkersForKind(vfs.FSXFS))
		assert.Equal(t, vfs.BTRFSWorkers, vfs.FSWorkersForKind(vfs.FSBtrfs))
		assert.Equal(t, vfs.APFSWorkers, vfs.FSWorkersForKind(vfs.FSAPFS))
	})

	t.Run("an unmeasured filesystem gets the conservative ceiling", func(t *testing.T) {
		t.Parallel()

		assert.Equal(t, vfs.DefaultFSWorkers, vfs.FSWorkersForKind(vfs.FSUnknown))
		assert.Equal(t, vfs.DefaultFSWorkers, vfs.FSWorkersForKind(vfs.FSUnprobed))
		assert.Equal(t, vfs.DefaultFSWorkers, vfs.FSWorkersForKind(vfs.FSVirtual))
		assert.Equal(t, vfs.DefaultFSWorkers, vfs.FSWorkersForKind(vfs.FSNetwork))
	})

	t.Run("a probed path is bounded", func(t *testing.T) {
		t.Parallel()

		workers := vfs.FSWorkersFor(vfs.NewOSFS(), t.TempDir())

		assert.GreaterOrEqual(t, workers, vfs.BTRFSWorkers)
		assert.LessOrEqual(t, workers, vfs.MaxFSWorkers)
	})
}

func TestFSWorkersForNonexistentPath(t *testing.T) {
	t.Parallel()

	existing := t.TempDir()
	missing := filepath.Join(existing, "not-created-yet", "deeper")

	assert.Equal(t, vfs.FSWorkersFor(vfs.NewOSFS(), existing), vfs.FSWorkersFor(vfs.NewOSFS(), missing))
}
