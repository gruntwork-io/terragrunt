package cln_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/cln"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStore(t *testing.T) {
	t.Parallel()

	t.Run("custom path", func(t *testing.T) {
		t.Parallel()
		tempDir := t.TempDir()
		customPath := filepath.Join(tempDir, "custom-store")

		store, err := cln.NewStore(customPath)
		require.NoError(t, err)
		assert.Equal(t, customPath, store.Path())
	})

	t.Run("default path", func(t *testing.T) {
		t.Parallel()
		home, err := os.UserHomeDir()
		require.NoError(t, err)

		expectedPath := filepath.Join(home, ".cln-store")

		store, err := cln.NewStore("")
		require.NoError(t, err)
		assert.Equal(t, expectedPath, store.Path())

		_, err = os.Stat(expectedPath)
		assert.NoError(t, err, "Store directory should exist")
	})
}

func TestStore_HasContent(t *testing.T) {
	t.Parallel()
	tempDir := t.TempDir()
	store, err := cln.NewStore(tempDir)
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
