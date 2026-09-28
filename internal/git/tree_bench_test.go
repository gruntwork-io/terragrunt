package git_test

import (
	"bytes"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gruntwork-io/terragrunt/internal/git"
)

// lsTreeOutput builds realistic `git ls-tree -r` output with n entries, using
// the nesting depth and name lengths typical of an infrastructure monorepo.
func lsTreeOutput(n int) []byte {
	var buf bytes.Buffer

	envs := []string{"dev", "stage", "prod"}
	regions := []string{"us-east-1", "us-west-2", "eu-central-1"}
	units := []string{"vpc", "eks", "rds", "s3-bucket", "iam-roles", "route53"}
	files := []string{"terragrunt.hcl", "main.tf", "variables.tf", "outputs.tf", "README.md"}

	for i := range n {
		fmt.Fprintf(&buf, "100644 blob %040x\tinfrastructure-live/%s/%s/%s-%d/%s\n",
			i*2654435761,
			envs[i%len(envs)],
			regions[i/len(envs)%len(regions)],
			units[i%len(units)],
			i,
			files[i%len(files)],
		)
	}

	return buf.Bytes()
}

func TestParseTreeTabDelimited(t *testing.T) {
	t.Parallel()

	tree, err := git.ParseTree(lsTreeOutput(3), "repo")
	require.NoError(t, err)

	entries := tree.Entries()
	require.Len(t, entries, 3)

	assert.Equal(t, "100644", entries[0].Mode)
	assert.Equal(t, git.EntryTypeBlob, entries[0].Type)
	assert.Equal(t, "infrastructure-live/dev/us-east-1/vpc-0/terragrunt.hcl", entries[0].Path)
}

// TestParseTreeEntryPreservesRepeatedSpaces pins the fix for paths whose names
// contain consecutive spaces. The previous strings.Fields parser split on
// whitespace runs and rejoined with a single space, silently renaming them.
func TestParseTreeEntryPreservesRepeatedSpaces(t *testing.T) {
	t.Parallel()

	tc := []struct {
		name string
		line string
		want string
	}{
		{
			name: "tab delimited with repeated spaces",
			line: "100644 blob a1b2c3d4\tmy  spaced  file.tf",
			want: "my  spaced  file.tf",
		},
		{
			name: "space delimited with single spaces",
			line: "100644 blob a1b2c3d4 path with spaces.txt",
			want: "path with spaces.txt",
		},
		{
			name: "path containing a tab",
			line: "100644 blob a1b2c3d4\tweird\tname.tf",
			want: "weird\tname.tf",
		},
	}

	for _, tt := range tc {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			entry, err := git.ParseTreeEntry(tt.line)
			require.NoError(t, err)
			assert.Equal(t, tt.want, entry.Path)
		})
	}
}

func TestParseTreeEntryRejectsMalformed(t *testing.T) {
	t.Parallel()

	tc := []struct {
		name string
		line string
	}{
		{name: "no delimiters", line: "garbage"},
		{name: "missing hash and path", line: "100644 blob"},
		{name: "missing path", line: "100644 blob abc123"},
		{name: "empty path", line: "100644 blob abc123\t"},
	}

	for _, tt := range tc {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			_, err := git.ParseTreeEntry(tt.line)
			require.ErrorIs(t, err, git.ErrParseTree)
		})
	}
}

func BenchmarkParseTree(b *testing.B) {
	for _, n := range []int{1_000, 10_000, 100_000} {
		output := lsTreeOutput(n)

		b.Run(fmt.Sprintf("entries_%d", n), func(b *testing.B) {
			b.SetBytes(int64(len(output)))
			b.ReportAllocs()

			for b.Loop() {
				tree, err := git.ParseTree(output, ".")
				require.NoError(b, err)
				require.Len(b, tree.Entries(), n)
			}
		})
	}
}
