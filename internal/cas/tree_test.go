package cas_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/cas"
	"github.com/gruntwork-io/terragrunt/internal/git"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseTreeEntry(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		input   string
		want    git.TreeEntry
		wantErr bool
	}{
		{
			name:  "regular file",
			input: "100644 blob a1b2c3d4 README.md",
			want: git.TreeEntry{
				Mode: "100644",
				Type: "blob",
				Hash: "a1b2c3d4",
				Path: "README.md",
			},
		},
		{
			name:  "executable file",
			input: "100755 blob e5f6g7h8 scripts/test.sh",
			want: git.TreeEntry{
				Mode: "100755",
				Type: "blob",
				Hash: "e5f6g7h8",
				Path: "scripts/test.sh",
			},
		},
		{
			name:  "directory",
			input: "040000 tree i9j0k1l2 src",
			want: git.TreeEntry{
				Mode: "040000",
				Type: "tree",
				Hash: "i9j0k1l2",
				Path: "src",
			},
		},
		{
			name:  "path with spaces",
			input: "100644 blob m3n4o5p6 path with spaces.txt",
			want: git.TreeEntry{
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

			got, err := git.ParseTreeEntry(tt.input)
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
		path     string
		wantPath string
		input    []byte
		wantLen  int
		wantErr  bool
	}{
		{
			name: "multiple entries",
			input: []byte(`100644 blob a1b2c3d4 README.md
100755 blob e5f6g7h8 scripts/test.sh
040000 tree i9j0k1l2 src`),
			path:     "test-repo",
			wantLen:  3,
			wantPath: "test-repo",
		},
		{
			name:     "empty input",
			input:    []byte(""),
			path:     "empty-repo",
			wantLen:  0,
			wantPath: "empty-repo",
		},
		{
			name: "invalid entry",
			input: []byte(`100644 blob a1b2c3d4 README.md
invalid format`),
			path:    "invalid-repo",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := git.ParseTree(tt.input, tt.path)
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

	tests := []struct {
		name       string
		setupStore func(t *testing.T) (*cas.Store, string)
		treeData   []byte
		wantFiles  []struct {
			path    string
			hash    string
			content []byte
			isDir   bool
		}
		wantErr bool
	}{
		{
			name: "basic tree with files and directories",
			setupStore: func(t *testing.T) (*cas.Store, string) {
				t.Helper()

				storeDir := t.TempDir()
				store := cas.NewStore(storeDir)
				content := cas.NewContent(store)

				// Create test content
				testData := []byte("test content")
				testHash := "a1b2c3d4"
				err := content.Store(nil, testHash, testData)
				require.NoError(t, err)

				// Create and store the src directory tree data
				srcTreeData := `100644 blob a1b2c3d4 README.md`
				srcTreeHash := "i9j0k1l2"
				err = content.Store(nil, srcTreeHash, []byte(srcTreeData))
				require.NoError(t, err)

				return store, testHash
			},
			treeData: []byte(`100644 blob a1b2c3d4 README.md
100755 blob a1b2c3d4 scripts/test.sh
040000 tree i9j0k1l2 src`),
			wantFiles: []struct {
				path    string
				hash    string
				content []byte
				isDir   bool
			}{
				{
					path:    "README.md",
					content: []byte("test content"),
					isDir:   false,
					hash:    "a1b2c3d4",
				},
				{
					path:    "scripts/test.sh",
					content: []byte("test content"),
					isDir:   false,
					hash:    "a1b2c3d4",
				},
				{
					path:  "src",
					isDir: true,
				},
				{
					path:    "src/README.md",
					content: []byte("test content"),
					isDir:   false,
					hash:    "a1b2c3d4",
				},
			},
		},
		{
			name: "empty tree",
			setupStore: func(t *testing.T) (*cas.Store, string) {
				t.Helper()

				storeDir := t.TempDir()
				store := cas.NewStore(storeDir)
				return store, ""
			},
			treeData: []byte(""),
			wantFiles: []struct {
				path    string
				hash    string
				content []byte
				isDir   bool
			}{},
		},
		{
			name: "tree with missing content",
			setupStore: func(t *testing.T) (*cas.Store, string) {
				t.Helper()

				storeDir := t.TempDir()
				store := cas.NewStore(storeDir)
				return store, ""
			},
			treeData: []byte(`100644 blob missing123 README.md`),
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// Setup store
			store, _ := tt.setupStore(t)

			// Parse the tree
			tree, err := git.ParseTree(tt.treeData, "test-repo")
			require.NoError(t, err)

			// Create target directory
			targetDir := t.TempDir()

			// Link the tree
			err = cas.LinkTree(t.Context(), store, tree, targetDir)
			if tt.wantErr {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)

			// Verify all expected files and directories
			for _, want := range tt.wantFiles {
				path := filepath.Join(targetDir, want.path)

				// Check if file/directory exists
				info, err := os.Stat(path)
				require.NoError(t, err)
				assert.Equal(t, want.isDir, info.IsDir())

				if !want.isDir {
					// Check file content
					data, err := os.ReadFile(path)
					require.NoError(t, err)
					assert.Equal(t, want.content, data)

					dataStat, err := os.Stat(path)
					require.NoError(t, err)

					// Verify hard link by comparing content.
					// We don't compare inode numbers because the test might be running on Windows.
					storePath := filepath.Join(store.Path(), want.hash[:2], want.hash)
					storeStat, err := os.Stat(storePath)
					require.NoError(t, err)

					assert.True(t, os.SameFile(dataStat, storeStat))
				}
			}
		})
	}
}
