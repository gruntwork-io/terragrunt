package cas_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/cas"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStore(t *testing.T) {
	t.Parallel()

	t.Run("custom path", func(t *testing.T) {
		t.Parallel()
		tempDir := t.TempDir()
		customPath := filepath.Join(tempDir, "custom-store")

		store, err := cas.NewStore(customPath)
		require.NoError(t, err)
		assert.Equal(t, customPath, store.Path())
	})

	t.Run("default path", func(t *testing.T) {
		t.Parallel()
		home, err := os.UserHomeDir()
		require.NoError(t, err)

		expectedPath := filepath.Join(home, ".cas-store")

		store, err := cas.NewStore("")
		require.NoError(t, err)
		assert.Equal(t, expectedPath, store.Path())

		_, err = os.Stat(expectedPath)
		assert.NoError(t, err, "Store directory should exist")
	})
}

func TestStore_HasContent(t *testing.T) {
	t.Parallel()
	tempDir := t.TempDir()
	store, err := cas.NewStore(tempDir)
	require.NoError(t, err)

	// Create a fake content file
	testHash := "abcdef123456"
	// Create partition directory
	partitionDir := filepath.Join(store.Path(), testHash[:2])
	err = os.MkdirAll(partitionDir, 0755)
	require.NoError(t, err, "Failed to create partition directory")

	testPath := filepath.Join(partitionDir, testHash)
	err = os.WriteFile(testPath, []byte("test"), 0644)
	require.NoError(t, err, "Failed to create test file")

	tests := []struct {
		name string
		hash string
		want bool
	}{
		{
			name: "existing content",
			hash: testHash,
			want: true,
		},
		{
			name: "non-existing content",
			hash: "nonexistent",
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, store.HasContent(tt.hash))
		})
	}
}
