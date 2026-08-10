package git_test

import (
	"bufio"
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/git"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestParseTree_RoundTripsCanonicalOutput pins the round trip for canonical ls-tree output only.
func TestParseTree_RoundTripsCanonicalOutput(t *testing.T) {
	t.Parallel()

	output := []byte(
		"100644 blob aaaabeefcafefacedeadbeefcafefacedeadbeef\tREADME.md\n" +
			"160000 commit bbbbbeefcafefacedeadbeefcafefacedeadbeef\tchild repo\n" +
			"040000 tree ccccbeefcafefacedeadbeefcafefacedeadbeef\tmodules\n",
	)

	tree, err := git.ParseTree(output, ".")
	require.NoError(t, err)

	assert.Equal(t, ".", tree.Path())
	assert.Equal(t, output, tree.Data())
	assert.Equal(t, []git.TreeEntry{
		{
			Mode: "100644",
			Type: git.EntryTypeBlob,
			Hash: "aaaabeefcafefacedeadbeefcafefacedeadbeef",
			Path: "README.md",
		},
		{
			Mode: "160000",
			Type: git.EntryTypeCommit,
			Hash: "bbbbbeefcafefacedeadbeefcafefacedeadbeef",
			Path: "child repo",
		},
		{
			Mode: "040000",
			Type: git.EntryTypeTree,
			Hash: "ccccbeefcafefacedeadbeefcafefacedeadbeef",
			Path: "modules",
		},
	}, tree.Entries())

	var rendered bytes.Buffer
	require.NoError(t, tree.Write(&rendered))
	assert.Equal(t, output, rendered.Bytes())
}

func TestParseTree_RejectsInvalidOutput(t *testing.T) {
	t.Parallel()

	t.Run("invalid entry", func(t *testing.T) {
		t.Parallel()

		_, err := git.ParseTree([]byte("100644 blob\n"), ".")
		require.ErrorIs(t, err, git.ErrParseTree)
	})

	t.Run("entry longer than the scanner buffer", func(t *testing.T) {
		t.Parallel()

		entry := "100644 blob aaaabeefcafefacedeadbeefcafefacedeadbeef\t" + strings.Repeat("a", bufio.MaxScanTokenSize)

		_, err := git.ParseTree([]byte(entry), ".")
		require.ErrorIs(t, err, bufio.ErrTooLong)

		var wrappedErr *git.WrappedError
		require.ErrorAs(t, err, &wrappedErr)
	})
}

func TestParseTree_SkipsBlankLines(t *testing.T) {
	t.Parallel()

	tree, err := git.ParseTree(
		[]byte("\n100644 blob aaaabeefcafefacedeadbeefcafefacedeadbeef\tREADME.md\n\n"),
		".",
	)
	require.NoError(t, err)
	assert.Len(t, tree.Entries(), 1)
	assert.Equal(t, "README.md", tree.Entries()[0].Path)
}

func TestTree_WritePropagatesWriterError(t *testing.T) {
	t.Parallel()

	tree, err := git.ParseTree(
		[]byte("100644 blob aaaabeefcafefacedeadbeefcafefacedeadbeef\tREADME.md\n"),
		".",
	)
	require.NoError(t, err)

	wantErr := errors.New("write failed")
	err = tree.Write(&failingWriter{err: wantErr})
	require.ErrorIs(t, err, wantErr)
}

type failingWriter struct {
	err error
}

func (w *failingWriter) Write([]byte) (int, error) {
	return 0, w.err
}
