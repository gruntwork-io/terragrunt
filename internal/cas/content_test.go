package cas_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/cas"
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
		store := cas.NewStore(t.TempDir())

		content := cas.NewContent(store)
		testHash := testHashValue
		testData := []byte("test content")

		err := content.Store(&l, testHash, testData)
		require.NoError(t, err)

		// Verify content was stored
		partitionDir := filepath.Join(store.Path(), testHash[:2])
		storedPath := filepath.Join(partitionDir, testHash)
		storedData, err := os.ReadFile(storedPath)
		require.NoError(t, err)
		assert.Equal(t, testData, storedData)
	})

	t.Run("ensure existing content", func(t *testing.T) {
		t.Parallel()

		store := cas.NewStore(t.TempDir())

		content := cas.NewContent(store)
		testHash := testHashValue
		testData := []byte("test content")
		differentData := []byte("different content")

		// Store content twice
		err := content.Ensure(&l, testHash, testData)
		require.NoError(t, err)
		err = content.Ensure(&l, testHash, differentData)
		require.NoError(t, err)

		// Verify original content remains
		partitionDir := filepath.Join(store.Path(), testHash[:2])
		storedPath := filepath.Join(partitionDir, testHash)
		storedData, err := os.ReadFile(storedPath)
		require.NoError(t, err)
		assert.Equal(t, testData, storedData)
	})

	t.Run("overwrite existing content", func(t *testing.T) {
		t.Parallel()

		store := cas.NewStore(t.TempDir())

		content := cas.NewContent(store)
		testHash := testHashValue
		testData := []byte("test content")
		differentData := []byte("different content")

		// Store content twice
		err := content.Store(&l, testHash, testData)
		require.NoError(t, err)
		err = content.Store(&l, testHash, differentData)
		require.NoError(t, err)

		// Verify original content remains
		partitionDir := filepath.Join(store.Path(), testHash[:2])
		storedPath := filepath.Join(partitionDir, testHash)
		storedData, err := os.ReadFile(storedPath)
		require.NoError(t, err)
		assert.Equal(t, differentData, storedData)
	})
}

func TestContent_Link(t *testing.T) {
	t.Parallel()

	l := logger.CreateLogger()

	t.Run("create new link", func(t *testing.T) {
		t.Parallel()
		storeDir := t.TempDir()
		store := cas.NewStore(storeDir)

		content := cas.NewContent(store)
		testHash := testHashValue
		testData := []byte("test content")

		// First store some content
		err := content.Store(&l, testHash, testData)
		require.NoError(t, err)

		// Then create a link to it
		targetDir := t.TempDir()
		targetPath := filepath.Join(targetDir, "subdir", "test.txt")
		err = content.Link(testHash, targetPath)
		require.NoError(t, err)

		// Verify link was created and contains correct content
		linkedData, err := os.ReadFile(targetPath)
		require.NoError(t, err)
		assert.Equal(t, testData, linkedData)

		// Verify it's a hard link by checking inode numbers
		partitionDir := filepath.Join(store.Path(), testHash[:2])
		sourceInfo, err := os.Stat(filepath.Join(partitionDir, testHash))
		require.NoError(t, err)
		targetInfo, err := os.Stat(targetPath)
		require.NoError(t, err)
		assert.Equal(t, sourceInfo.Sys(), targetInfo.Sys())
	})

	t.Run("link to existing file", func(t *testing.T) {
		t.Parallel()
		store := cas.NewStore(t.TempDir())

		content := cas.NewContent(store)
		testHash := testHashValue
		testData := []byte("test content")

		// Store content
		err := content.Store(&l, testHash, testData)
		require.NoError(t, err)

		// Create target file
		targetDir := t.TempDir()
		targetPath := filepath.Join(targetDir, "test.txt")
		err = os.WriteFile(targetPath, []byte("existing content"), 0644)
		require.NoError(t, err)

		// Try to create link
		err = content.Link(testHash, targetPath)
		require.NoError(t, err)

		// Verify original content remains
		existingData, err := os.ReadFile(targetPath)
		require.NoError(t, err)
		assert.Equal(t, []byte("existing content"), existingData)
	})
}
