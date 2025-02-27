package cas_test

import (
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
