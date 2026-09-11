package cas_test

import (
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gruntwork-io/terragrunt/internal/cas"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/gruntwork-io/terragrunt/test/helpers/venvtest"
)

const testHashValue = "abcdef123456"

func TestContent_Store(t *testing.T) {
	t.Parallel()

	l := logger.CreateLogger()

	t.Run("store new content", func(t *testing.T) {
		t.Parallel()

		v := venvtest.New()
		require.NoError(t, v.FS.MkdirAll("/store", 0755))

		store := cas.NewStore("/store")

		content := cas.NewContent(store)
		testHash := testHashValue
		testData := []byte("test content")

		err := content.Store(l, v, testHash, testData, cas.StoredFilePerms)
		require.NoError(t, err)

		// Verify content was stored
		partitionDir := filepath.Join(store.Path(), testHash[:2])
		storedPath := filepath.Join(partitionDir, testHash)
		storedData, err := vfs.ReadFile(v.FS, storedPath)
		require.NoError(t, err)
		assert.Equal(t, testData, storedData)
	})

	t.Run("stores under the requested perm with write bits cleared", func(t *testing.T) {
		t.Parallel()

		v := venvtest.NewOSWithEmptyEnv()

		storeDir := t.TempDir()
		store := cas.NewStore(storeDir)

		content := cas.NewContent(store)
		testHash := testHashValue

		require.NoError(t, content.Store(l, v, testHash, []byte("test content"), 0o600))

		info, err := os.Stat(filepath.Join(storeDir, testHash[:2], testHash))
		require.NoError(t, err)
		assert.Equal(t, os.FileMode(0o400), info.Mode().Perm(),
			"a caller asking for an owner-only blob must not get a world-readable one")
	})

	t.Run("recovers from a stale read-only temp file", func(t *testing.T) {
		t.Parallel()

		v := venvtest.NewOSWithEmptyEnv()

		storeDir := t.TempDir()
		store := cas.NewStore(storeDir)

		content := cas.NewContent(store)
		testHash := testHashValue
		testData := []byte("test content")

		partitionDir := filepath.Join(storeDir, testHash[:2])
		require.NoError(t, os.MkdirAll(partitionDir, 0o755))

		// An interrupted run can leave a read-only temp file behind. Opening
		// it for writing fails with EACCES, so Store must not reuse it.
		storedPath := filepath.Join(partitionDir, testHash)
		require.NoError(t, os.WriteFile(storedPath+".tmp", []byte("partial"), 0o400))

		require.NoError(t, content.Store(l, v, testHash, testData, cas.StoredFilePerms))

		got, err := os.ReadFile(storedPath)
		require.NoError(t, err)
		assert.Equal(t, testData, got)
	})

	t.Run("ensure existing content", func(t *testing.T) {
		t.Parallel()

		v := venvtest.New()
		require.NoError(t, v.FS.MkdirAll("/store", 0755))

		store := cas.NewStore("/store")

		content := cas.NewContent(store)
		testHash := testHashValue
		testData := []byte("test content")
		differentData := []byte("different content")

		// Store content twice
		err := content.Ensure(l, v, testHash, testData, cas.StoredFilePerms)
		require.NoError(t, err)
		err = content.Ensure(l, v, testHash, differentData, cas.StoredFilePerms)
		require.NoError(t, err)

		// Verify original content remains
		partitionDir := filepath.Join(store.Path(), testHash[:2])
		storedPath := filepath.Join(partitionDir, testHash)
		storedData, err := vfs.ReadFile(v.FS, storedPath)
		require.NoError(t, err)
		assert.Equal(t, testData, storedData)
	})

	t.Run("overwrite existing content", func(t *testing.T) {
		t.Parallel()

		v := venvtest.New()
		require.NoError(t, v.FS.MkdirAll("/store", 0755))

		store := cas.NewStore("/store")

		content := cas.NewContent(store)
		testHash := testHashValue
		testData := []byte("test content")
		differentData := []byte("different content")

		// Store content twice
		err := content.Store(l, v, testHash, testData, cas.StoredFilePerms)
		require.NoError(t, err)
		err = content.Store(l, v, testHash, differentData, cas.StoredFilePerms)
		require.NoError(t, err)

		// Verify content was overwritten
		partitionDir := filepath.Join(store.Path(), testHash[:2])
		storedPath := filepath.Join(partitionDir, testHash)
		storedData, err := vfs.ReadFile(v.FS, storedPath)
		require.NoError(t, err)
		assert.Equal(t, differentData, storedData)
	})
}

// TestContent_WriteLeavesInFlightTempFile pins that a store write leaves
// alone the temp file another writer of the same hash is still filling.
// [cas.Store.Lock] serializes only writers sharing one Store, so a writer
// in another CAS instance or process can hold a temp file for the same
// object.
func TestContent_WriteLeavesInFlightTempFile(t *testing.T) {
	t.Parallel()

	testData := []byte("test content")

	for _, tt := range contentWriters(testData) {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			v := venvtest.NewOSWithEmptyEnv()

			storeDir := t.TempDir()
			content := cas.NewContent(cas.NewStore(storeDir))

			inFlight, err := content.GetTmpHandle(v, testHashValue)
			require.NoError(t, err)

			_, err = inFlight.Write([]byte("partial"))
			require.NoError(t, err)

			// Closed before any assertion can fail, since Windows cannot
			// remove the test's temp dir while the handle is open.
			writeErr := tt.write(t, v, content)
			require.NoError(t, inFlight.Close())
			require.NoError(t, writeErr)

			got, err := os.ReadFile(inFlight.Name())
			require.NoError(t, err)
			assert.Equal(t, []byte("partial"), got)

			got, err = os.ReadFile(filepath.Join(storeDir, testHashValue[:2], testHashValue))
			require.NoError(t, err)
			assert.Equal(t, testData, got)
		})
	}
}

// TestContent_WriteToleratesConcurrentPublish pins that a store write
// succeeds when another writer publishes the same object between its
// existence check and its rename, on a filesystem that refuses to replace
// the read-only object, and that the losing write removes its temp file.
func TestContent_WriteToleratesConcurrentPublish(t *testing.T) {
	t.Parallel()

	testData := []byte("test content")

	for _, tt := range contentWriters(testData) {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			v := venvtest.NewOSWithEmptyEnv().WithFS(&publishFirstFS{FS: vfs.NewOSFS()})

			storeDir := t.TempDir()
			content := cas.NewContent(cas.NewStore(storeDir))

			require.NoError(t, tt.write(t, v, content))

			partitionDir := filepath.Join(storeDir, testHashValue[:2])

			got, err := os.ReadFile(filepath.Join(partitionDir, testHashValue))
			require.NoError(t, err)
			assert.Equal(t, testData, got)

			entries, err := os.ReadDir(partitionDir)
			require.NoError(t, err)
			require.Len(t, entries, 1, "temp file left beside the object")
			assert.Equal(t, testHashValue, entries[0].Name())
		})
	}
}

// TestContent_WritePanicRemovesTempFile pins that a store write which
// panics after creating its temp file still removes it.
func TestContent_WritePanicRemovesTempFile(t *testing.T) {
	t.Parallel()

	testData := []byte("test content")

	for _, tt := range contentWriters(testData) {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			v := venvtest.NewOSWithEmptyEnv().WithFS(&chmodPanicFS{FS: vfs.NewOSFS()})

			storeDir := t.TempDir()
			content := cas.NewContent(cas.NewStore(storeDir))

			assert.Panics(t, func() {
				assert.NoError(t, tt.write(t, v, content))
			})

			entries, err := os.ReadDir(filepath.Join(storeDir, testHashValue[:2]))
			require.NoError(t, err)
			assert.Empty(t, entries)
		})
	}
}

// contentWriter is one [cas.Content] method that writes a fresh object.
type contentWriter struct {
	write func(t *testing.T, v *venv.Venv, content *cas.Content) error
	name  string
}

// contentWriters returns the [cas.Content] methods that write an object
// holding data at testHashValue.
func contentWriters(data []byte) []contentWriter {
	l := logger.CreateLogger()

	return []contentWriter{
		{
			name: "store",
			write: func(_ *testing.T, v *venv.Venv, content *cas.Content) error {
				return content.Store(l, v, testHashValue, data, cas.StoredFilePerms)
			},
		},
		{
			name: "ensure copy",
			write: func(t *testing.T, v *venv.Venv, content *cas.Content) error {
				t.Helper()

				src := filepath.Join(t.TempDir(), "src")
				require.NoError(t, os.WriteFile(src, data, 0o644))

				return content.EnsureCopy(l, v, testHashValue, src)
			},
		},
	}
}

// publishFirstFS publishes the object itself on every rename and then
// refuses the rename the way Windows refuses to replace a read-only file,
// standing in for another writer that wins the race.
type publishFirstFS struct {
	vfs.FS
}

func (fsys *publishFirstFS) Rename(oldname, newname string) error {
	data, err := vfs.ReadFile(fsys.FS, oldname)
	if err != nil {
		return err
	}

	if err := vfs.WriteFile(fsys.FS, newname, data, cas.StoredFilePerms); err != nil {
		return err
	}

	return &os.LinkError{Op: "rename", Old: oldname, New: newname, Err: fs.ErrPermission}
}

// chmodPanicFS panics on every Chmod.
type chmodPanicFS struct {
	vfs.FS
}

func (fsys *chmodPanicFS) Chmod(name string, _ os.FileMode) error {
	panic("chmod " + name)
}

func TestContent_Link(t *testing.T) {
	t.Parallel()

	l := logger.CreateLogger()

	t.Run("create new link", func(t *testing.T) {
		t.Parallel()

		v := venvtest.New()
		require.NoError(t, v.FS.MkdirAll("/store", 0755))
		require.NoError(t, v.FS.MkdirAll("/target", 0755))

		store := cas.NewStore("/store")

		content := cas.NewContent(store)
		testHash := testHashValue
		testData := []byte("test content")

		// First store some content
		err := content.Store(l, v, testHash, testData, cas.StoredFilePerms)
		require.NoError(t, err)

		// Then create a link to it
		targetPath := filepath.Join("/target", "test.txt")

		_, err = content.Link(l, v, testHash, targetPath, 0o644)
		require.NoError(t, err)

		// Verify link was created and contains correct content
		linkedData, err := vfs.ReadFile(v.FS, targetPath)
		require.NoError(t, err)
		assert.Equal(t, testData, linkedData)
	})

	t.Run("create hard link on real filesystem", func(t *testing.T) {
		t.Parallel()

		v := venvtest.NewOSWithEmptyEnv()

		storeDir := t.TempDir()
		targetDir := t.TempDir()
		store := cas.NewStore(storeDir)

		content := cas.NewContent(store)
		testHash := testHashValue
		testData := []byte("test content")

		err := content.Store(l, v, testHash, testData, cas.StoredFilePerms)
		require.NoError(t, err)

		targetPath := filepath.Join(targetDir, "test.txt")
		_, err = content.Link(l, v, testHash, targetPath, 0o644)
		require.NoError(t, err)

		// Verify hard link by comparing inodes
		sourcePath := filepath.Join(storeDir, testHash[:2], testHash)
		sourceInfo, err := os.Stat(sourcePath)
		require.NoError(t, err)
		targetInfo, err := os.Stat(targetPath)
		require.NoError(t, err)
		assert.True(t, os.SameFile(sourceInfo, targetInfo), "expected hard link (same inode)")
	})

	// A generated file whose destination directory was deleted between runs still
	// hardlinks, rather than quietly falling back to a copy.
	t.Run("create hard link under missing directory on real filesystem", func(t *testing.T) {
		t.Parallel()

		v := venvtest.NewOSWithEmptyEnv()

		storeDir := t.TempDir()
		targetDir := t.TempDir()
		store := cas.NewStore(storeDir)

		content := cas.NewContent(store)
		testHash := testHashValue
		testData := []byte("test content")

		err := content.Store(l, v, testHash, testData, cas.StoredFilePerms)
		require.NoError(t, err)

		targetPath := filepath.Join(targetDir, "generated", "nested", "test.txt")
		_, err = content.Link(l, v, testHash, targetPath, 0o644)
		require.NoError(t, err)

		sourcePath := filepath.Join(storeDir, testHash[:2], testHash)
		sourceInfo, err := os.Stat(sourcePath)
		require.NoError(t, err)
		targetInfo, err := os.Stat(targetPath)
		require.NoError(t, err)
		assert.True(t, os.SameFile(sourceInfo, targetInfo), "expected hard link (same inode)")
	})

	t.Run("force copy creates independent inode on real filesystem", func(t *testing.T) {
		t.Parallel()

		v := venvtest.NewOSWithEmptyEnv()

		storeDir := t.TempDir()
		targetDir := t.TempDir()
		store := cas.NewStore(storeDir)

		content := cas.NewContent(store)
		testHash := testHashValue
		testData := []byte("test content")

		err := content.Store(l, v, testHash, testData, cas.StoredFilePerms)
		require.NoError(t, err)

		targetPath := filepath.Join(targetDir, "test.txt")
		_, err = content.Link(l, v, testHash, targetPath, 0o644, cas.WithLinkForceCopy())
		require.NoError(t, err)

		sourcePath := filepath.Join(storeDir, testHash[:2], testHash)
		sourceInfo, err := os.Stat(sourcePath)
		require.NoError(t, err)
		targetInfo, err := os.Stat(targetPath)
		require.NoError(t, err)
		assert.False(
			t,
			os.SameFile(sourceInfo, targetInfo),
			"expected independent inode (copy, not hard link)",
		)
		assert.Equal(t, os.FileMode(0o644), targetInfo.Mode().Perm(),
			"force copy must preserve original git perms exactly")

		copied, err := os.ReadFile(targetPath)
		require.NoError(t, err)
		assert.Equal(t, testData, copied)

		// The destination must be writable so callers can mutate it without
		// touching the shared store.
		require.NoError(t, os.WriteFile(targetPath, []byte("mutated"), 0644))

		stored, err := os.ReadFile(sourcePath)
		require.NoError(t, err)
		assert.Equal(t, testData, stored, "store blob must not change when target is mutated")
	})

	t.Run("default path strips write bit from non-executable", func(t *testing.T) {
		t.Parallel()

		v := venvtest.NewOSWithEmptyEnv()

		storeDir := t.TempDir()
		targetDir := t.TempDir()
		store := cas.NewStore(storeDir)

		content := cas.NewContent(store)
		testHash := testHashValue
		testData := []byte("test content")

		require.NoError(t, content.Store(l, v, testHash, testData, cas.StoredFilePerms))

		targetPath := filepath.Join(targetDir, "test.txt")
		_, err := content.Link(l, v, testHash, targetPath, 0o644)
		require.NoError(t, err)

		info, err := os.Stat(targetPath)
		require.NoError(t, err)
		assert.Equal(t, os.FileMode(0o444), info.Mode().Perm(),
			"default path must clear write bits (0o644 -> 0o444)")
	})

	t.Run(
		"default path hardlinks executable when store carries matching perms",
		func(t *testing.T) {
			t.Parallel()

			v := venvtest.NewOSWithEmptyEnv()

			storeDir := t.TempDir()
			targetDir := t.TempDir()
			store := cas.NewStore(storeDir)

			content := cas.NewContent(store)
			testHash := testHashValue
			testData := []byte("#!/bin/sh\necho hi\n")

			require.NoError(t, content.Store(l, v, testHash, testData, cas.StoredFilePerms))

			// Mirror the store-side chmod that the git-clone path applies: stored
			// blobs carry their original git mode with write bits cleared, so
			// executables sit at 0o555 in the store.
			sourcePath := filepath.Join(storeDir, testHash[:2], testHash)
			require.NoError(t, os.Chmod(sourcePath, 0o555))

			targetPath := filepath.Join(targetDir, "run.sh")
			_, err := content.Link(l, v, testHash, targetPath, 0o755)
			require.NoError(t, err)

			info, err := os.Stat(targetPath)
			require.NoError(t, err)
			assert.Equal(t, os.FileMode(0o555), info.Mode().Perm(),
				"executable entry must keep exec bits and lose only write (0o755 -> 0o555)")

			sourceInfo, err := os.Stat(sourcePath)
			require.NoError(t, err)
			assert.True(t, os.SameFile(sourceInfo, info),
				"executable entry should hardlink when the stored blob already carries 0o555")
		},
	)

	t.Run("default path falls back to copy on perm collision", func(t *testing.T) {
		t.Parallel()

		v := venvtest.NewOSWithEmptyEnv()

		storeDir := t.TempDir()
		targetDir := t.TempDir()
		store := cas.NewStore(storeDir)

		content := cas.NewContent(store)
		testHash := testHashValue
		testData := []byte("test content")

		require.NoError(t, content.Store(l, v, testHash, testData, cas.StoredFilePerms))

		// The blob landed in the store at 0o444 (treated as non-exec). A second
		// tree referencing the same content under mode 100755 wants 0o555.
		// Link must produce a fresh inode at 0o555 rather than hardlinking
		// the 0o444 blob.
		targetPath := filepath.Join(targetDir, "run.sh")
		_, err := content.Link(l, v, testHash, targetPath, 0o755)
		require.NoError(t, err)

		info, err := os.Stat(targetPath)
		require.NoError(t, err)
		assert.Equal(t, os.FileMode(0o555), info.Mode().Perm())

		sourcePath := filepath.Join(storeDir, testHash[:2], testHash)
		sourceInfo, err := os.Stat(sourcePath)
		require.NoError(t, err)
		assert.False(t, os.SameFile(sourceInfo, info),
			"perm mismatch must materialize as an independent inode")
	})

	t.Run("force copy preserves executable bits", func(t *testing.T) {
		t.Parallel()

		v := venvtest.NewOSWithEmptyEnv()

		storeDir := t.TempDir()
		targetDir := t.TempDir()
		store := cas.NewStore(storeDir)

		content := cas.NewContent(store)
		testHash := testHashValue
		testData := []byte("#!/bin/sh\necho hi\n")

		require.NoError(t, content.Store(l, v, testHash, testData, cas.StoredFilePerms))

		targetPath := filepath.Join(targetDir, "run.sh")
		_, err := content.Link(l, v, testHash, targetPath, 0o755, cas.WithLinkForceCopy())
		require.NoError(t, err)

		info, err := os.Stat(targetPath)
		require.NoError(t, err)
		assert.Equal(t, os.FileMode(0o755), info.Mode().Perm(),
			"force copy must reproduce git mode exactly (0o755)")
	})

	t.Run("link to existing file overwrites stale content", func(t *testing.T) {
		t.Parallel()

		v := venvtest.New()
		require.NoError(t, v.FS.MkdirAll("/store", 0755))
		require.NoError(t, v.FS.MkdirAll("/target", 0755))

		store := cas.NewStore("/store")

		content := cas.NewContent(store)
		testHash := testHashValue
		testData := []byte("test content")

		// Store content
		err := content.Store(l, v, testHash, testData, cas.StoredFilePerms)
		require.NoError(t, err)

		// Pre-populate the target with stale bytes. A previous failed
		// run could leave the working tree in this state; Link must
		// publish the CAS content rather than silently keep the stale
		// file.
		targetPath := filepath.Join("/target", "test.txt")
		err = vfs.WriteFile(v.FS, targetPath, []byte("existing content"), 0644)
		require.NoError(t, err)

		_, err = content.Link(l, v, testHash, targetPath, 0o644)
		require.NoError(t, err)

		got, err := vfs.ReadFile(v.FS, targetPath)
		require.NoError(t, err)
		assert.Equal(t, testData, got)
	})

	t.Run("default path replaces an occupied target with a hardlink", func(t *testing.T) {
		t.Parallel()

		v := venvtest.NewOSWithEmptyEnv()

		storeDir := t.TempDir()
		targetDir := t.TempDir()
		store := cas.NewStore(storeDir)

		content := cas.NewContent(store)
		testHash := testHashValue
		testData := []byte("test content")

		require.NoError(t, content.Store(l, v, testHash, testData, cas.StoredFilePerms))

		// The state a rerun finds: the previous materialization is still
		// there, read-only, on a name no hard link can be made over.
		targetPath := filepath.Join(targetDir, "test.txt")
		require.NoError(t, os.WriteFile(targetPath, []byte("stale"), 0o444))

		_, linkErr := content.Link(l, v, testHash, targetPath, 0o644)
		require.NoError(t, linkErr)

		got, err := os.ReadFile(targetPath)
		require.NoError(t, err)
		assert.Equal(t, testData, got)

		sourceInfo, err := os.Stat(filepath.Join(storeDir, testHash[:2], testHash))
		require.NoError(t, err)

		info, err := os.Stat(targetPath)
		require.NoError(t, err)
		assert.True(t, os.SameFile(sourceInfo, info),
			"an occupied target must still end up sharing the stored blob")
	})

	t.Run("stored perms win over a narrower request when accepted", func(t *testing.T) {
		t.Parallel()

		v := venvtest.NewOSWithEmptyEnv()

		storeDir := t.TempDir()
		targetDir := t.TempDir()
		store := cas.NewStore(storeDir)

		content := cas.NewContent(store)
		testHash := testHashValue

		require.NoError(
			t,
			content.Store(l, v, testHash, []byte("test content"), cas.StoredFilePerms),
		)

		sourceInfo, err := os.Stat(filepath.Join(storeDir, testHash[:2], testHash))
		require.NoError(t, err)

		targetPath := filepath.Join(targetDir, "test.txt")
		_, linkErr := content.Link(l, v, testHash, targetPath, 0o600, cas.WithLinkStoredPerm())
		require.NoError(t, linkErr)

		info, err := os.Stat(targetPath)
		require.NoError(t, err)
		assert.True(t, os.SameFile(sourceInfo, info),
			"sharing the stored blob matters more than the exact mode")
		assert.Equal(t, os.FileMode(0o444), info.Mode().Perm())
	})

	t.Run("a narrower request copies when stored perms are not accepted", func(t *testing.T) {
		t.Parallel()

		v := venvtest.NewOSWithEmptyEnv()

		storeDir := t.TempDir()
		targetDir := t.TempDir()
		store := cas.NewStore(storeDir)

		content := cas.NewContent(store)
		testHash := testHashValue

		require.NoError(
			t,
			content.Store(l, v, testHash, []byte("test content"), cas.StoredFilePerms),
		)

		sourceInfo, err := os.Stat(filepath.Join(storeDir, testHash[:2], testHash))
		require.NoError(t, err)

		targetPath := filepath.Join(targetDir, "test.txt")
		_, linkErr := content.Link(l, v, testHash, targetPath, 0o600)
		require.NoError(t, linkErr)

		info, err := os.Stat(targetPath)
		require.NoError(t, err)
		assert.False(t, os.SameFile(sourceInfo, info))
		assert.Equal(t, os.FileMode(0o400), info.Mode().Perm(),
			"a caller holding out for the narrow mode gets an inode of its own")
	})

	t.Run("copy path recovers from a stale read-only temp file", func(t *testing.T) {
		t.Parallel()

		v := venvtest.NewOSWithEmptyEnv()

		storeDir := t.TempDir()
		targetDir := t.TempDir()
		store := cas.NewStore(storeDir)

		content := cas.NewContent(store)
		testHash := testHashValue
		testData := []byte("ref: refs/heads/master\n")

		require.NoError(t, content.Store(l, v, testHash, testData, cas.StoredFilePerms))

		// Mismatched store perms route Link through the copy path.
		require.NoError(t, os.Chmod(filepath.Join(storeDir, testHash[:2], testHash), 0o644))

		targetPath := filepath.Join(targetDir, "HEAD")

		// An interrupted run can leave a read-only temp file behind. Opening
		// it for writing fails with EACCES, so Link must not reuse it.
		require.NoError(t, os.WriteFile(targetPath+".tmp", []byte("partial"), 0o444))

		_, err := content.Link(l, v, testHash, targetPath, 0o644)
		require.NoError(t, err)

		got, err := os.ReadFile(targetPath)
		require.NoError(t, err)
		assert.Equal(t, testData, got)
	})
}

func TestContent_EnsureWithWait(t *testing.T) {
	t.Parallel()

	l := logger.CreateLogger()

	t.Run("content already exists", func(t *testing.T) {
		t.Parallel()

		v := venvtest.New()
		require.NoError(t, v.FS.MkdirAll("/store", 0755))

		store := cas.NewStore("/store")

		content := cas.NewContent(store)
		testHash := testHashValue
		testData := []byte("test content")

		// Store content first
		err := content.Store(l, v, testHash, testData, cas.StoredFilePerms)
		require.NoError(t, err)

		// EnsureWithWait should not need to write again
		err = content.EnsureWithWait(l, v, testHash, []byte("different content"), cas.StoredFilePerms)
		require.NoError(t, err)

		// Verify original content remains
		partitionDir := filepath.Join(store.Path(), testHash[:2])
		storedPath := filepath.Join(partitionDir, testHash)
		storedData, err := vfs.ReadFile(v.FS, storedPath)
		require.NoError(t, err)
		assert.Equal(t, testData, storedData)
	})

	t.Run("content doesn't exist", func(t *testing.T) {
		t.Parallel()

		v := venvtest.New()
		require.NoError(t, v.FS.MkdirAll("/store", 0755))

		store := cas.NewStore("/store")

		content := cas.NewContent(store)
		testHash := "newcontent123456"
		testData := []byte("new test content")

		// EnsureWithWait should store the content
		err := content.EnsureWithWait(l, v, testHash, testData, cas.StoredFilePerms)
		require.NoError(t, err)

		// Verify content was stored
		partitionDir := filepath.Join(store.Path(), testHash[:2])
		storedPath := filepath.Join(partitionDir, testHash)
		storedData, err := vfs.ReadFile(v.FS, storedPath)
		require.NoError(t, err)
		assert.Equal(t, testData, storedData)
	})

	t.Run("concurrent writes - optimization", func(t *testing.T) {
		t.Parallel()

		v := venvtest.New()
		require.NoError(t, v.FS.MkdirAll("/store", 0755))

		store := cas.NewStore("/store")

		content := cas.NewContent(store)
		testHash := "concurrent123456"

		// Channel to coordinate the test
		process1Started := make(chan struct{})
		process1Done := make(chan struct{})
		process2Done := make(chan struct{})

		// Process 1: acquires lock first
		go func() {
			defer close(process1Done)

			err := content.EnsureWithWait(l, v, testHash, []byte("process 1 data"), cas.StoredFilePerms)
			assert.NoError(t, err)

			close(process1Started)
		}()

		// Process 2: should wait for process 1 and not duplicate work
		go func() {
			defer close(process2Done)

			// Wait for process 1 to start
			<-process1Started

			err := content.EnsureWithWait(l, v, testHash, []byte("process 2 data"), cas.StoredFilePerms)
			assert.NoError(t, err)
		}()

		// Wait for both to complete
		<-process1Done
		<-process2Done

		// Verify only one content exists (from process 1)
		partitionDir := filepath.Join(store.Path(), testHash[:2])
		storedPath := filepath.Join(partitionDir, testHash)
		storedData, err := vfs.ReadFile(v.FS, storedPath)
		require.NoError(t, err)
		assert.Equal(t, []byte("process 1 data"), storedData)
	})
}
