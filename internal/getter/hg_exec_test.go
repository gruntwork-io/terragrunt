//go:build exec

// Real-hg parity tests for HgGetter.
//
// They spawn the hg binary against a repository on disk. The default build
// pins the command lines through the in-memory exec in hg_test.go.

package getter_test

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/getter"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/gruntwork-io/terragrunt/test/helpers"
	"github.com/gruntwork-io/terragrunt/test/helpers/venvtest"
	"github.com/stretchr/testify/require"
)

func TestExecHgGetterClonesThenUpdatesToRev(t *testing.T) {
	t.Parallel()

	v, tmp := hgExecVenv(t)
	repo := filepath.Join(tmp, "repo")
	first := hgCommitRevisions(t, v, repo, "# v1", "# v2")
	src := "hg::" + getter.FileURL(repo).String()
	dst := filepath.Join(tmp, "module")
	g := getter.NewHgGetter(v)

	require.NoError(t, download(t, g, src, dst, getter.ModeDir))
	assertFileContent(t, v.FS, filepath.Join(dst, "main.tf"), "# v2")

	require.NoError(t, download(t, g, src+"?rev="+first, dst, getter.ModeDir))
	assertFileContent(t, v.FS, filepath.Join(dst, "main.tf"), "# v1")
}

func TestExecHgGetterDownloadsSingleFile(t *testing.T) {
	t.Parallel()

	v, tmp := hgExecVenv(t)
	repo := filepath.Join(tmp, "repo")
	hgCommitRevisions(t, v, repo, "# v1")

	dst := filepath.Join(tmp, "out", "main.tf")

	require.NoError(t, download(t, getter.NewHgGetter(v),
		"hg::"+getter.FileURL(filepath.Join(repo, "main.tf")).String(), dst, getter.ModeFile))
	assertFileContent(t, v.FS, dst, "# v1")
}

// hgExecVenv returns an OS venv for running hg, and a temporary directory the
// venv also uses for its own temporary files. It skips the test when hg is
// not installed.
func hgExecVenv(t *testing.T) (*venv.Venv, string) {
	t.Helper()

	tmp := helpers.TmpDirWOSymlinks(t)
	v := venvtest.NewOSWithEmptyEnv().WithTempDir(func() string { return tmp })

	if _, err := v.Exec.LookPath("hg"); err != nil {
		t.Skip("hg binary not installed on this host")
	}

	return v, tmp
}

// hgCommitRevisions creates an hg repository at dir and commits one revision
// of main.tf for each content, in order. It returns the node of the first
// revision.
func hgCommitRevisions(t *testing.T, v *venv.Venv, dir string, contents ...string) string {
	t.Helper()

	hg := func(args ...string) string {
		cmd := v.Exec.Command(t.Context(), "hg", args...)
		cmd.SetDir(dir)

		out, err := cmd.CombinedOutput()
		require.NoErrorf(t, err, "hg %v: %s", args, out)

		return strings.TrimSpace(string(out))
	}

	require.NoError(t, v.FS.MkdirAll(dir, 0o755))
	hg("init")

	var first string

	for _, content := range contents {
		require.NoError(t, vfs.WriteFile(v.FS, filepath.Join(dir, "main.tf"), []byte(content), 0o644))
		hg("--config", "ui.username=test <test@example.com>", "commit", "-A", "-m", content)

		if first == "" {
			first = hg("log", "-r", "tip", "-T", "{node}")
		}
	}

	return first
}
