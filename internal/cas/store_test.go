package cas_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

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

		store := cas.NewStore(customPath)
		assert.Equal(t, customPath, store.Path())
	})
}

func TestStore_NeedsWrite(t *testing.T) {
	t.Parallel()
	tempDir := t.TempDir()
	store := cas.NewStore(tempDir)

	// Create a fake content file
	testHash := "abcdef123456"
	// Create partition directory
	partitionDir := filepath.Join(store.Path(), testHash[:2])
	err := os.MkdirAll(partitionDir, 0755)
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
			want: false,
		},
		{
			name: "non-existing content",
			hash: "nonexistent",
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, store.NeedsWrite(tt.hash, time.Now()))
		})
	}
}
