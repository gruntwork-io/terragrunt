package cas_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/cas"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testHashValue = "abcdef123456"

func TestContent_Store(t *testing.T) {
	t.Parallel()

	l := logger.CreateLogger()

	t.Run("store new content", func(t *testing.T) {
		t.Parallel()

		memFs := vfs.NewMemMapFS()
		require.NoError(t, memFs.MkdirAll("/store", 0755))
		store := cas.NewStore("/store").WithFS(memFs)

		content := cas.NewContent(store)
		testHash := testHashValue
		testData := []byte("test content")

		err := content.Store(l, testHash, testData)
		require.NoError(t, err)

		// Verify content was stored
		partitionDir := filepath.Join(store.Path(), testHash[:2])
		storedPath := filepath.Join(partitionDir, testHash)
		storedData, err := vfs.ReadFile(memFs, storedPath)
		require.NoError(t, err)
		assert.Equal(t, testData, storedData)
	})

	t.Run("ensure existing content", func(t *testing.T) {
		t.Parallel()

		memFs := vfs.NewMemMapFS()
		require.NoError(t, memFs.MkdirAll("/store", 0755))
		store := cas.NewStore("/store").WithFS(memFs)

		content := cas.NewContent(store)
		testHash := testHashValue
		testData := []byte("test content")
		differentData := []byte("different content")

		// Store content twice
		err := content.Ensure(l, testHash, testData)
		require.NoError(t, err)
		err = content.Ensure(l, testHash, differentData)
		require.NoError(t, err)

		// Verify original content remains
		partitionDir := filepath.Join(store.Path(), testHash[:2])
		storedPath := filepath.Join(partitionDir, testHash)
		storedData, err := vfs.ReadFile(memFs, storedPath)
		require.NoError(t, err)
		assert.Equal(t, testData, storedData)
	})

	t.Run("overwrite existing content", func(t *testing.T) {
		t.Parallel()

		memFs := vfs.NewMemMapFS()
		require.NoError(t, memFs.MkdirAll("/store", 0755))
		store := cas.NewStore("/store").WithFS(memFs)

		content := cas.NewContent(store)
		testHash := testHashValue
		testData := []byte("test content")
		differentData := []byte("different content")

		// Store content twice
		err := content.Store(l, testHash, testData)
		require.NoError(t, err)
		err = content.Store(l, testHash, differentData)
		require.NoError(t, err)

		// Verify content was overwritten
		partitionDir := filepath.Join(store.Path(), testHash[:2])
		storedPath := filepath.Join(partitionDir, testHash)
		storedData, err := vfs.ReadFile(memFs, storedPath)
		require.NoError(t, err)
		assert.Equal(t, differentData, storedData)
	})
}

func TestContent_Link(t *testing.T) {
	t.Parallel()

	l := logger.CreateLogger()

	t.Run("create new link", func(t *testing.T) {
		t.Parallel()

		memFs := vfs.NewMemMapFS()
		require.NoError(t, memFs.MkdirAll("/store", 0755))
		require.NoError(t, memFs.MkdirAll("/target", 0755))
		store := cas.NewStore("/store").WithFS(memFs)

		content := cas.NewContent(store)
		testHash := testHashValue
		testData := []byte("test content")

		// First store some content
		err := content.Store(l, testHash, testData)
		require.NoError(t, err)

		// Then create a link to it
		targetPath := filepath.Join("/target", "test.txt")

		err = content.Link(t.Context(), testHash, targetPath, 0o644)
		require.NoError(t, err)

		// Verify link was created and contains correct content
		linkedData, err := vfs.ReadFile(memFs, targetPath)
		require.NoError(t, err)
		assert.Equal(t, testData, linkedData)
	})

	t.Run("create hard link on real filesystem", func(t *testing.T) {
		t.Parallel()

		osFs := vfs.NewOSFS()
		storeDir := t.TempDir()
		targetDir := t.TempDir()
		store := cas.NewStore(storeDir).WithFS(osFs)

		content := cas.NewContent(store)
		testHash := testHashValue
		testData := []byte("test content")

		err := content.Store(l, testHash, testData)
		require.NoError(t, err)

		targetPath := filepath.Join(targetDir, "test.txt")
		err = content.Link(t.Context(), testHash, targetPath, 0o644)
		require.NoError(t, err)

		// Verify hard link by comparing inodes
		sourcePath := filepath.Join(storeDir, testHash[:2], testHash)
		sourceInfo, err := os.Stat(sourcePath)
		require.NoError(t, err)
		targetInfo, err := os.Stat(targetPath)
		require.NoError(t, err)
		assert.True(t, os.SameFile(sourceInfo, targetInfo), "expected hard link (same inode)")
	})

	t.Run("force copy creates independent inode on real filesystem", func(t *testing.T) {
		t.Parallel()

		osFs := vfs.NewOSFS()
		storeDir := t.TempDir()
		targetDir := t.TempDir()
		store := cas.NewStore(storeDir).WithFS(osFs)

		content := cas.NewContent(store)
		testHash := testHashValue
		testData := []byte("test content")

		err := content.Store(l, testHash, testData)
		require.NoError(t, err)

		targetPath := filepath.Join(targetDir, "test.txt")
		err = content.Link(t.Context(), testHash, targetPath, 0o644, cas.WithLinkForceCopy())
		require.NoError(t, err)

		sourcePath := filepath.Join(storeDir, testHash[:2], testHash)
		sourceInfo, err := os.Stat(sourcePath)
		require.NoError(t, err)
		targetInfo, err := os.Stat(targetPath)
		require.NoError(t, err)
		assert.False(t, os.SameFile(sourceInfo, targetInfo), "expected independent inode (copy, not hard link)")
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

		osFs := vfs.NewOSFS()
		storeDir := t.TempDir()
		targetDir := t.TempDir()
		store := cas.NewStore(storeDir).WithFS(osFs)

		content := cas.NewContent(store)
		testHash := testHashValue
		testData := []byte("test content")

		require.NoError(t, content.Store(l, testHash, testData))

		targetPath := filepath.Join(targetDir, "test.txt")
		require.NoError(t, content.Link(t.Context(), testHash, targetPath, 0o644))

		info, err := os.Stat(targetPath)
		require.NoError(t, err)
		assert.Equal(t, os.FileMode(0o444), info.Mode().Perm(),
			"default path must clear write bits (0o644 -> 0o444)")
	})

	t.Run("default path hardlinks executable when store carries matching perms", func(t *testing.T) {
		t.Parallel()

		osFs := vfs.NewOSFS()
		storeDir := t.TempDir()
		targetDir := t.TempDir()
		store := cas.NewStore(storeDir).WithFS(osFs)

		content := cas.NewContent(store)
		testHash := testHashValue
		testData := []byte("#!/bin/sh\necho hi\n")

		require.NoError(t, content.Store(l, testHash, testData))

		// Mirror the store-side chmod that the git-clone path applies: stored
		// blobs carry their original git mode with write bits cleared, so
		// executables sit at 0o555 in the store.
		sourcePath := filepath.Join(storeDir, testHash[:2], testHash)
		require.NoError(t, os.Chmod(sourcePath, 0o555))

		targetPath := filepath.Join(targetDir, "run.sh")
		require.NoError(t, content.Link(t.Context(), testHash, targetPath, 0o755))

		info, err := os.Stat(targetPath)
		require.NoError(t, err)
		assert.Equal(t, os.FileMode(0o555), info.Mode().Perm(),
			"executable entry must keep exec bits and lose only write (0o755 -> 0o555)")

		sourceInfo, err := os.Stat(sourcePath)
		require.NoError(t, err)
		assert.True(t, os.SameFile(sourceInfo, info),
			"executable entry should hardlink when the stored blob already carries 0o555")
	})

	t.Run("default path falls back to copy on perm collision", func(t *testing.T) {
		t.Parallel()

		osFs := vfs.NewOSFS()
		storeDir := t.TempDir()
		targetDir := t.TempDir()
		store := cas.NewStore(storeDir).WithFS(osFs)

		content := cas.NewContent(store)
		testHash := testHashValue
		testData := []byte("test content")

		require.NoError(t, content.Store(l, testHash, testData))

		// The blob landed in the store at 0o444 (treated as non-exec). A second
		// tree referencing the same content under mode 100755 wants 0o555.
		// Link must produce a fresh inode at 0o555 rather than hardlinking
		// the 0o444 blob.
		targetPath := filepath.Join(targetDir, "run.sh")
		require.NoError(t, content.Link(t.Context(), testHash, targetPath, 0o755))

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

		osFs := vfs.NewOSFS()
		storeDir := t.TempDir()
		targetDir := t.TempDir()
		store := cas.NewStore(storeDir).WithFS(osFs)

		content := cas.NewContent(store)
		testHash := testHashValue
		testData := []byte("#!/bin/sh\necho hi\n")

		require.NoError(t, content.Store(l, testHash, testData))

		targetPath := filepath.Join(targetDir, "run.sh")
		require.NoError(t, content.Link(t.Context(), testHash, targetPath, 0o755, cas.WithLinkForceCopy()))

		info, err := os.Stat(targetPath)
		require.NoError(t, err)
		assert.Equal(t, os.FileMode(0o755), info.Mode().Perm(),
			"force copy must reproduce git mode exactly (0o755)")
	})

	t.Run("link to existing file", func(t *testing.T) {
		t.Parallel()

		memFs := vfs.NewMemMapFS()
		require.NoError(t, memFs.MkdirAll("/store", 0755))
		require.NoError(t, memFs.MkdirAll("/target", 0755))
		store := cas.NewStore("/store").WithFS(memFs)

		content := cas.NewContent(store)
		testHash := testHashValue
		testData := []byte("test content")

		// Store content
		err := content.Store(l, testHash, testData)
		require.NoError(t, err)

		// Create target file
		targetPath := filepath.Join("/target", "test.txt")
		err = vfs.WriteFile(memFs, targetPath, []byte("existing content"), 0644)
		require.NoError(t, err)

		// Try to create link
		err = content.Link(t.Context(), testHash, targetPath, 0o644)
		require.NoError(t, err)

		// Verify original content remains
		existingData, err := vfs.ReadFile(memFs, targetPath)
		require.NoError(t, err)
		assert.Equal(t, []byte("existing content"), existingData)
	})
}

func TestContent_EnsureWithWait(t *testing.T) {
	t.Parallel()

	l := logger.CreateLogger()

	t.Run("content already exists", func(t *testing.T) {
		t.Parallel()

		memFs := vfs.NewMemMapFS()
		require.NoError(t, memFs.MkdirAll("/store", 0755))
		store := cas.NewStore("/store").WithFS(memFs)

		content := cas.NewContent(store)
		testHash := testHashValue
		testData := []byte("test content")

		// Store content first
		err := content.Store(l, testHash, testData)
		require.NoError(t, err)

		// EnsureWithWait should not need to write again
		err = content.EnsureWithWait(l, testHash, []byte("different content"))
		require.NoError(t, err)

		// Verify original content remains
		partitionDir := filepath.Join(store.Path(), testHash[:2])
		storedPath := filepath.Join(partitionDir, testHash)
		storedData, err := vfs.ReadFile(memFs, storedPath)
		require.NoError(t, err)
		assert.Equal(t, testData, storedData)
	})

	t.Run("content doesn't exist", func(t *testing.T) {
		t.Parallel()

		memFs := vfs.NewMemMapFS()
		require.NoError(t, memFs.MkdirAll("/store", 0755))
		store := cas.NewStore("/store").WithFS(memFs)

		content := cas.NewContent(store)
		testHash := "newcontent123456"
		testData := []byte("new test content")

		// EnsureWithWait should store the content
		err := content.EnsureWithWait(l, testHash, testData)
		require.NoError(t, err)

		// Verify content was stored
		partitionDir := filepath.Join(store.Path(), testHash[:2])
		storedPath := filepath.Join(partitionDir, testHash)
		storedData, err := vfs.ReadFile(memFs, storedPath)
		require.NoError(t, err)
		assert.Equal(t, testData, storedData)
	})

	t.Run("concurrent writes - optimization", func(t *testing.T) {
		t.Parallel()

		memFs := vfs.NewMemMapFS()
		require.NoError(t, memFs.MkdirAll("/store", 0755))
		store := cas.NewStore("/store").WithFS(memFs)

		content := cas.NewContent(store)
		testHash := "concurrent123456"

		// Channel to coordinate the test
		process1Started := make(chan struct{})
		process1Done := make(chan struct{})
		process2Done := make(chan struct{})

		// Process 1: acquires lock first
		go func() {
			defer close(process1Done)

			err := content.EnsureWithWait(l, testHash, []byte("process 1 data"))
			assert.NoError(t, err)

			close(process1Started)
		}()

		// Process 2: should wait for process 1 and not duplicate work
		go func() {
			defer close(process2Done)

			// Wait for process 1 to start
			<-process1Started

			err := content.EnsureWithWait(l, testHash, []byte("process 2 data"))
			assert.NoError(t, err)
		}()

		// Wait for both to complete
		<-process1Done
		<-process2Done

		// Verify only one content exists (from process 1)
		partitionDir := filepath.Join(store.Path(), testHash[:2])
		storedPath := filepath.Join(partitionDir, testHash)
		storedData, err := vfs.ReadFile(memFs, storedPath)
		require.NoError(t, err)
		assert.Equal(t, []byte("process 1 data"), storedData)
	})
}
