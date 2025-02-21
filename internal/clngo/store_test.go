package clngo_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/clngo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStore(t *testing.T) {
	t.Parallel()

	t.Run("custom path", func(t *testing.T) {
		t.Parallel()
		tempDir := t.TempDir()
		customPath := filepath.Join(tempDir, "custom-store")

		store, err := clngo.NewStore(customPath)
		require.NoError(t, err)
		assert.Equal(t, customPath, store.Path())
	})

	t.Run("default path", func(t *testing.T) {
		t.Parallel()
		store, err := clngo.NewStore("")
		require.NoError(t, err)

		home, err := os.UserHomeDir()
		require.NoError(t, err)

		expected := filepath.Join(home, ".cache", ".cln-store")
		t.Cleanup(func() {
			err := os.RemoveAll(expected)
			assert.NoError(t, err, "Failed to cleanup store directory")
		})

		assert.Equal(t, expected, store.Path())

		// Verify directory was created
		_, err = os.Stat(expected)
		assert.NoError(t, err, "Store directory should exist")
	})
}

func TestStore_HasContent(t *testing.T) {
	t.Parallel()
	tempDir := t.TempDir()
	store, err := clngo.NewStore(tempDir)
	require.NoError(t, err)

	// Create a fake content file
	testHash := "abcdef123456"
	testPath := filepath.Join(store.Path(), testHash)
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
