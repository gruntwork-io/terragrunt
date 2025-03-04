package cas_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/cas"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseTreeEntry(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		input   string
		want    cas.TreeEntry
		wantErr bool
	}{
		{
			name:  "regular file",
			input: "100644 blob a1b2c3d4 README.md",
			want: cas.TreeEntry{
				Mode: "100644",
				Type: "blob",
				Hash: "a1b2c3d4",
				Path: "README.md",
			},
		},
		{
			name:  "executable file",
			input: "100755 blob e5f6g7h8 scripts/test.sh",
			want: cas.TreeEntry{
				Mode: "100755",
				Type: "blob",
				Hash: "e5f6g7h8",
				Path: "scripts/test.sh",
			},
		},
		{
			name:  "directory",
			input: "040000 tree i9j0k1l2 src",
			want: cas.TreeEntry{
				Mode: "040000",
				Type: "tree",
				Hash: "i9j0k1l2",
				Path: "src",
			},
		},
		{
			name:  "path with spaces",
			input: "100644 blob m3n4o5p6 path with spaces.txt",
			want: cas.TreeEntry{
				Mode: "100644",
				Type: "blob",
				Hash: "m3n4o5p6",
				Path: "path with spaces.txt",
			},
		},
		{
			name:    "invalid format",
			input:   "invalid format",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := cas.ParseTreeEntry(tt.input)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestParseTree(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		input    string
		path     string
		wantLen  int
		wantPath string
		wantErr  bool
	}{
		{
			name: "multiple entries",
			input: `100644 blob a1b2c3d4 README.md
100755 blob e5f6g7h8 scripts/test.sh
040000 tree i9j0k1l2 src`,
			path:     "test-repo",
			wantLen:  3,
			wantPath: "test-repo",
		},
		{
			name:     "empty input",
			input:    "",
			path:     "empty-repo",
			wantLen:  0,
			wantPath: "empty-repo",
		},
		{
			name: "invalid entry",
			input: `100644 blob a1b2c3d4 README.md
invalid format`,
			path:    "invalid-repo",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := cas.ParseTree(tt.input, tt.path)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Len(t, got.Entries(), tt.wantLen)
			assert.Equal(t, tt.wantPath, got.Path())
		})
	}
}

func TestLinkTree(t *testing.T) {
	t.Parallel()

	// Create a temporary store directory
	storeDir := t.TempDir()
	store := cas.NewStore(storeDir)
	content := cas.NewContent(store)

	// Create test content
	testData := []byte("test content")
	testHash := "a1b2c3d4" // Using a fixed hash for testing
	err := content.Store(nil, testHash, testData)
	require.NoError(t, err)

	// Create and store the src directory tree data
	srcTreeData := `100644 blob a1b2c3d4 README.md`
	srcTreeHash := "i9j0k1l2"
	err = content.Store(nil, srcTreeHash, []byte(srcTreeData))
	require.NoError(t, err)

	// Create a test tree with both files and directories
	treeData := `100644 blob a1b2c3d4 README.md
100755 blob a1b2c3d4 scripts/test.sh
040000 tree i9j0k1l2 src`
	tree, err := cas.ParseTree(treeData, "test-repo")
	require.NoError(t, err)

	// Create target directory
	targetDir := t.TempDir()

	// Link the tree
	err = tree.LinkTree(context.Background(), store, targetDir)
	require.NoError(t, err)

	// Verify the structure was created correctly
	readmePath := filepath.Join(targetDir, "README.md")
	scriptPath := filepath.Join(targetDir, "scripts", "test.sh")
	srcPath := filepath.Join(targetDir, "src")
	srcReadmePath := filepath.Join(targetDir, "src", "README.md")

	// Check files exist and have correct content
	readmeData, err := os.ReadFile(readmePath)
	require.NoError(t, err)
	assert.Equal(t, testData, readmeData)

	scriptData, err := os.ReadFile(scriptPath)
	require.NoError(t, err)
	assert.Equal(t, testData, scriptData)

	// Check directory exists
	srcInfo, err := os.Stat(srcPath)
	require.NoError(t, err)
	assert.True(t, srcInfo.IsDir())

	// Check file in src directory exists and has correct content
	srcReadmeData, err := os.ReadFile(srcReadmePath)
	require.NoError(t, err)
	assert.Equal(t, testData, srcReadmeData)

	// Verify hard links were created
	storePath := filepath.Join(store.Path(), testHash[:2], testHash)
	storeInfo, err := os.Stat(storePath)
	require.NoError(t, err)

	readmeInfo, err := os.Stat(readmePath)
	require.NoError(t, err)
	assert.Equal(t, storeInfo.Sys(), readmeInfo.Sys())

	scriptInfo, err := os.Stat(scriptPath)
	require.NoError(t, err)
	assert.Equal(t, storeInfo.Sys(), scriptInfo.Sys())

	srcReadmeInfo, err := os.Stat(srcReadmePath)
	require.NoError(t, err)
	assert.Equal(t, storeInfo.Sys(), srcReadmeInfo.Sys())
}
