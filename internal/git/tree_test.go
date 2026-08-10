package git_test

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/git"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseTree_RoundTripsTree(t *testing.T) {
	t.Parallel()

	output := []byte(
		"100644 blob aaaabeefcafefacedeadbeefcafefacedeadbeef\tREADME.md\n" +
			"160000 commit bbbbbeefcafefacedeadbeefcafefacedeadbeef\tmodules/child repo\n",
	)

	tree, err := git.ParseTree(output, "modules")
	require.NoError(t, err)

	assert.Equal(t, "modules", tree.Path())
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
			Path: "modules/child repo",
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

	t.Run("oversized entry", func(t *testing.T) {
		t.Parallel()

		_, err := git.ParseTree([]byte(strings.Repeat("100644 blob hash path ", 4000)), ".")
		require.Error(t, err)

		var wrappedErr *git.WrappedError
		require.ErrorAs(t, err, &wrappedErr)
	})
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
