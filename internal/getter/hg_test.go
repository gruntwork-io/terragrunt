package getter_test

import (
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/gruntwork-io/terragrunt/internal/getter"
	"github.com/gruntwork-io/terragrunt/internal/vexec"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/gruntwork-io/terragrunt/test/helpers/venvtest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHgGetterClonesAndUpdatesToRev(t *testing.T) {
	t.Parallel()

	log := &commandLog{}
	dst := venvtest.Root("/work/module")

	require.NoError(t, download(t, getter.NewHgGetter(gitVenv(log)),
		"hg::https://example.com/org/repo?rev=v1", dst, getter.ModeDir))

	assert.Equal(t, []string{
		"hg clone -U -- https://example.com/org/repo " + dst,
		"hg pull",
		"hg update -- v1",
	}, log.argv())
	assert.NotContains(t, log.trusted, false, "every hg command must run as Terragrunt's own")
}

func TestHgGetterPullsIntoExistingCheckout(t *testing.T) {
	t.Parallel()

	log := &commandLog{}
	v := gitVenv(log)
	dst := venvtest.Root("/work/module")

	require.NoError(t, v.FS.MkdirAll(dst, 0o755))

	require.NoError(t, download(t, getter.NewHgGetter(v), "hg::https://example.com/org/repo", dst, getter.ModeDir))

	assert.Equal(t, []string{"hg pull", "hg update"}, log.argv())
}

func TestHgGetterFailsWhenExecRefuses(t *testing.T) {
	t.Parallel()

	log := &commandLog{replies: map[string]vexec.Result{"hg": {Err: errDenied}}}

	err := download(t, getter.NewHgGetter(gitVenv(log)),
		"hg::https://example.com/org/repo", venvtest.Root("/work/module"), getter.ModeDir)
	require.ErrorIs(t, err, errDenied)
}

func TestHgGetterErrorKeepsEndOfLongOutput(t *testing.T) {
	t.Parallel()

	const reason = "abort: repository example.com/org/repo not found"

	output := strings.Repeat("é", 100_000) + "\n" + reason + "\n"
	log := &commandLog{replies: map[string]vexec.Result{"hg clone": {Stdout: []byte(output), ExitCode: 255}}}

	err := download(t, getter.NewHgGetter(gitVenv(log)),
		"hg::https://example.com/org/repo", venvtest.Root("/work/module"), getter.ModeDir)
	require.Error(t, err)

	msg := err.Error()
	assert.Contains(t, msg, "running hg")
	assert.Contains(t, msg, reason)
	assert.True(t, utf8.ValidString(msg), "the message must not split a character")
	assert.Less(t, utf8.RuneCountInString(msg), 5000)
}

func TestHgGetterClonesFileSourceWithRootedPath(t *testing.T) {
	t.Parallel()

	log := &commandLog{}
	src := venvtest.Root("/repos/module")
	dst := venvtest.Root("/work/module")

	require.NoError(t, download(t, getter.NewHgGetter(gitVenv(log)),
		"hg::"+getter.FileURL(src).String(), dst, getter.ModeDir))

	want := "hg clone -U -- file:///" + strings.TrimPrefix(filepath.ToSlash(src), "/") + " " + dst
	assert.Equal(t, want, log.argv()[0])
}

func TestHgGetterGetFileKeepsRemoteHostOnWindows(t *testing.T) {
	t.Parallel()

	log := &commandLog{}
	v := gitVenv(log).WithGOOS("windows")
	dst := venvtest.Root("/work/main.tf")

	log.onRun = func(inv *vexec.Invocation) {
		if inv.Args[0] != "clone" {
			return
		}

		checkout := inv.Args[len(inv.Args)-1]
		require.NoError(t, v.FS.MkdirAll(checkout, 0o755))
		require.NoError(t, vfs.WriteFile(v.FS, filepath.Join(checkout, "main.tf"), []byte("# vpc"), 0o644))
	}

	require.NoError(t, download(t, getter.NewHgGetter(v),
		"hg::https://example.com/org/repo/main.tf", dst, getter.ModeFile))

	assert.True(t, strings.HasPrefix(log.argv()[0], "hg clone -U -- https://example.com/org/repo "),
		"clone must fetch from the source host: %s", log.argv()[0])

	assertFileContent(t, v.FS, dst, "# vpc")
}
