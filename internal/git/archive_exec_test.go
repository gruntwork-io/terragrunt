//go:build exec && !windows

// Real-git tests for archive materialization. Everything here spawns the actual
// git binary, so it is gated behind the exec tag alongside the other real-git
// tests; the default build pins the extraction contract through
// [git.ExtractArchive] in archive_test.go. Constrained to !windows because the
// unit matrix has only ever run these on ubuntu/macos.

package git_test

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/git"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/test/helpers"
	"github.com/gruntwork-io/terragrunt/test/helpers/venvtest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExecArchiveTreeCoversWholeTreeFromSubdirectory(t *testing.T) {
	t.Parallel()

	repo := newRepoWithFiles(t, map[string]string{
		"root.hcl":                  "locals {}\n",
		"live/unit/terragrunt.hcl":  "inputs = {}\n",
		"other/unit/terragrunt.hcl": "inputs = {}\n",
	})

	v := venv.OSVenv()

	runner, err := git.NewGitRunner(v)
	require.NoError(t, err)

	// A run started from a subdirectory still needs every unit in the tree, and
	// `git archive` covers only the directory it runs in.
	runner = runner.WithWorkDir(filepath.Join(repo, "live", "unit"))

	var archive bytes.Buffer

	require.NoError(t, runner.ArchiveTree(t.Context(), v, "HEAD", &archive))

	dest := helpers.TmpDirWOSymlinks(t)

	require.NoError(t, git.ExtractArchive(
		t.Context(),
		venvtest.NewWithOSFS(),
		bytes.NewReader(archive.Bytes()),
		dest,
		extractWriters,
	))

	for _, path := range []string{
		"root.hcl",
		filepath.Join("live", "unit", "terragrunt.hcl"),
		filepath.Join("other", "unit", "terragrunt.hcl"),
	} {
		assert.FileExists(t, filepath.Join(dest, path))
	}
}

func TestExecHasArchiveAlteringAttributes(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		files map[string]string
		name  string
		want  bool
	}{
		{
			name:  "no attributes",
			files: map[string]string{"terragrunt.hcl": "inputs = {}\n"},
			want:  false,
		},
		{
			name: "attributes that leave the archive alone",
			files: map[string]string{
				".gitattributes": "*.hcl text eol=lf\n",
				"terragrunt.hcl": "inputs = {}\n",
			},
			want: false,
		},
		{
			name: "export-ignore drops paths from the archive",
			files: map[string]string{
				".gitattributes": "vendor/ export-ignore\n",
				"terragrunt.hcl": "inputs = {}\n",
			},
			want: true,
		},
		{
			name: "a filter leaves content unresolved in the archive",
			files: map[string]string{
				"data/.gitattributes": "*.bin filter=lfs diff=lfs merge=lfs -text\n",
				"terragrunt.hcl":      "inputs = {}\n",
			},
			want: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			v := venv.OSVenv()

			runner, err := git.NewGitRunner(v)
			require.NoError(t, err)

			runner = runner.WithWorkDir(newRepoWithFiles(t, tc.files))

			got, err := runner.HasArchiveAlteringAttributes(t.Context(), v, "HEAD")
			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
}

// newRepoWithFiles commits files to a new repository and returns its path.
func newRepoWithFiles(t *testing.T, files map[string]string) string {
	t.Helper()

	ctx := t.Context()

	dir := helpers.TmpDirWOSymlinks(t)

	runner, err := git.NewGitRunner(venv.OSVenv())
	require.NoError(t, err)

	runner = runner.WithWorkDir(dir)

	require.NoError(t, runner.Init(ctx))
	require.NoError(t, runner.ConfigSet(ctx, "user.email", "test@example.com"))
	require.NoError(t, runner.ConfigSet(ctx, "user.name", "Terragrunt Test"))

	for name, content := range files {
		path := filepath.Join(dir, filepath.FromSlash(name))

		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
		require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
	}

	require.NoError(t, runner.Add(ctx, "."))
	require.NoError(t, runner.Commit(ctx, "initial commit"))

	return dir
}
