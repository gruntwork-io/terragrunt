package git_test

import (
	"bufio"
	"strings"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/git"
	"github.com/gruntwork-io/terragrunt/internal/vexec"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseDiff(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		want   *git.Diffs
		name   string
		output string
	}{
		{
			name: "classifies paths",
			output: "A\tadded.txt\n" +
				"D\tremoved file.txt\n" +
				"M\tchanged.txt\n" +
				"T\tignored.txt\n",
			want: &git.Diffs{
				Added:   []string{"added.txt"},
				Removed: []string{"removed file.txt"},
				Changed: []string{"changed.txt"},
			},
		},
		{
			name:   "empty output",
			output: "",
			want: &git.Diffs{
				Added:   []string{},
				Removed: []string{},
				Changed: []string{},
			},
		},
		{
			name:   "blank lines",
			output: "\n\nM\tchanged.txt\n\n",
			want: &git.Diffs{
				Added:   []string{},
				Removed: []string{},
				Changed: []string{"changed.txt"},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, err := git.ParseDiff([]byte(tc.output))
			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestParseDiff_RejectsInvalidOutput(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		wantErr error
		name    string
		output  string
	}{
		{
			name:    "missing path",
			output:  "M\n",
			wantErr: git.ErrParseDiff,
		},
		{
			name:    "oversized line",
			output:  strings.Repeat("M path ", 10000),
			wantErr: bufio.ErrTooLong,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			_, err := git.ParseDiff([]byte(tc.output))
			require.Error(t, err)

			var wrappedErr *git.WrappedError
			require.ErrorAs(t, err, &wrappedErr)

			if tc.wantErr != nil {
				assert.ErrorIs(t, err, tc.wantErr)
			}
		})
	}
}

func TestGitRunner_Diff(t *testing.T) {
	t.Parallel()

	t.Run("parses command output", func(t *testing.T) {
		t.Parallel()

		runner := newMemRunner(t, staticResult(vexec.Result{
			Stdout: []byte("A\tadded.txt\nM\tchanged.txt\n"),
		})).WithWorkDir("/repo")

		diffs, err := runner.Diff(t.Context(), "main", "feature")
		require.NoError(t, err)
		assert.Equal(t, []string{"added.txt"}, diffs.Added)
		assert.Equal(t, []string{"changed.txt"}, diffs.Changed)
		assert.Empty(t, diffs.Removed)
	})

	t.Run("command failure", func(t *testing.T) {
		t.Parallel()

		runner := newMemRunner(t, staticResult(vexec.Result{ExitCode: 128})).WithWorkDir("/repo")

		_, err := runner.Diff(t.Context(), "main", "feature")
		require.ErrorIs(t, err, git.ErrCommandSpawn)
	})

	t.Run("parse failure", func(t *testing.T) {
		t.Parallel()

		runner := newMemRunner(t, staticResult(vexec.Result{
			Stdout: []byte("M\n"),
		})).WithWorkDir("/repo")

		_, err := runner.Diff(t.Context(), "main", "feature")
		require.ErrorIs(t, err, git.ErrParseDiff)
	})

	t.Run("missing workdir", func(t *testing.T) {
		t.Parallel()

		runner := newMemRunner(t, staticResult(vexec.Result{}))

		_, err := runner.Diff(t.Context(), "main", "feature")
		require.ErrorIs(t, err, git.ErrNoWorkDir)
	})
}
