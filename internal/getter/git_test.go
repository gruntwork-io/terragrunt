package getter_test

import (
	"context"
	"encoding/base64"
	"errors"
	"io/fs"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/getter"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/internal/vexec"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/gruntwork-io/terragrunt/test/helpers/venvtest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// errDenied stands in for an exec that refuses to spawn.
var errDenied = errors.New("spawning refused")

// commandLog records every command a getter runs through a mem exec, and
// answers each with the reply the test registered for its argv prefix.
type commandLog struct {
	replies map[string]vexec.Result
	onRun   func(inv *vexec.Invocation)
	calls   []vexec.Invocation
	trusted []bool
	mu      sync.Mutex
}

// handler returns the [vexec.Handler] that records into c.
func (c *commandLog) handler() vexec.Handler {
	return func(ctx context.Context, inv vexec.Invocation) vexec.Result {
		return c.record(ctx, &inv)
	}
}

func (c *commandLog) record(ctx context.Context, inv *vexec.Invocation) vexec.Result {
	c.mu.Lock()
	c.calls = append(c.calls, *inv)
	c.trusted = append(c.trusted, vexec.IsTrustedCommand(ctx))
	c.mu.Unlock()

	if c.onRun != nil {
		c.onRun(inv)
	}

	line := strings.Join(append([]string{inv.Name}, inv.Args...), " ")
	for prefix, res := range c.replies {
		if strings.HasPrefix(line, prefix) {
			return res
		}
	}

	return vexec.Result{}
}

// argv returns every recorded command as one space-joined line.
func (c *commandLog) argv() []string {
	c.mu.Lock()
	defer c.mu.Unlock()

	out := make([]string, 0, len(c.calls))
	for _, inv := range c.calls {
		out = append(out, strings.Join(append([]string{inv.Name}, inv.Args...), " "))
	}

	return out
}

// envOf returns the value of the variable called name in the environment of
// the first recorded command whose line starts with prefix.
func (c *commandLog) envOf(t *testing.T, prefix, name string) string {
	t.Helper()

	c.mu.Lock()
	defer c.mu.Unlock()

	for _, inv := range c.calls {
		if !strings.HasPrefix(strings.Join(append([]string{inv.Name}, inv.Args...), " "), prefix) {
			continue
		}

		for _, kv := range inv.Env {
			if k, v, ok := strings.Cut(kv, "="); ok && k == name {
				return v
			}
		}

		return ""
	}

	require.Failf(t, "command not run", "no command starts with %q", prefix)

	return ""
}

// gitVenv returns an in-memory venv whose exec answers through log.
func gitVenv(log *commandLog) *venv.Venv {
	return venvtest.New().WithHandler(log.handler())
}

// download fetches src into dst with a client whose only getter is g.
func download(t *testing.T, g getter.Getter, src, dst string, mode getter.Mode) error {
	t.Helper()

	client := &getter.Client{Getters: []getter.Getter{g}}

	_, err := client.Get(t.Context(), &getter.Request{Src: src, Dst: dst, GetMode: mode})

	return err
}

func assertFileContent(t *testing.T, fsys vfs.FS, path, want string) {
	t.Helper()

	got, err := vfs.ReadFile(fsys, path)
	require.NoError(t, err)
	assert.Equal(t, want, string(got))
}

func TestGitGetterClonesRefAtDepth(t *testing.T) {
	t.Parallel()

	log := &commandLog{}
	dst := venvtest.Root("/work/module")

	err := download(t, getter.NewGitGetter(gitVenv(log)),
		"git::https://example.com/org/repo.git?ref=v1.2.3&depth=1", dst, getter.ModeDir)
	require.NoError(t, err)

	assert.Equal(t, []string{
		"git clone --depth 1 --single-branch -- https://example.com/org/repo.git " + dst,
		"git checkout v1.2.3",
		"git submodule update --init --recursive --depth 1",
	}, log.argv())

	assert.NotContains(t, log.trusted, false, "every git command must run as Terragrunt's own")
	assert.Equal(t, "ssh", log.envOf(t, "git clone", getter.EnvGitSSHCommand))
}

func TestGitGetterRefreshesExistingCheckout(t *testing.T) {
	t.Parallel()

	log := &commandLog{}
	v := gitVenv(log)
	dst := venvtest.Root("/work/module")

	require.NoError(t, v.FS.MkdirAll(filepath.Join(dst, ".git"), 0o755))
	require.NoError(t, v.FS.MkdirAll(filepath.Join(dst, ".GIT"), 0o755))

	err := download(t, getter.NewGitGetter(v),
		"git::https://example.com/org/repo.git?ref=main&depth=2", dst, getter.ModeDir)
	require.NoError(t, err)

	assert.Equal(t, []string{
		"git init",
		"git remote add origin -- https://example.com/org/repo.git",
		"git fetch --tags -- origin",
		"git -c maintenance.auto=false -c gc.auto=0 fetch -- origin main",
		"git reset --hard FETCH_HEAD",
		"git checkout main",
		"git pull origin --depth 2 --ff-only -- main",
		"git checkout main",
		"git submodule update --init --recursive --depth 2",
	}, log.argv())

	for _, name := range []string{".git", ".GIT"} {
		_, statErr := v.FS.Stat(filepath.Join(dst, name))
		require.ErrorIs(t, statErr, fs.ErrNotExist, "the old %s must be removed before the refresh", name)
	}
}

func TestGitGetterRefreshWithoutRefFollowsRemoteHead(t *testing.T) {
	t.Parallel()

	log := &commandLog{replies: map[string]vexec.Result{
		"git ls-remote --symref": {Stdout: []byte("ref: refs/heads/trunk\tHEAD\nabc123\tHEAD\n")},
	}}
	v := gitVenv(log)
	dst := venvtest.Root("/work/module")

	require.NoError(t, v.FS.MkdirAll(dst, 0o755))

	require.NoError(t, download(t, getter.NewGitGetter(v), "git::https://example.com/org/repo.git", dst, getter.ModeDir))

	assert.Contains(t, log.argv(), "git ls-remote --symref -- https://example.com/org/repo.git HEAD")
	assert.Contains(t, log.argv(), "git pull origin --ff-only -- trunk")
}

func TestGitGetterHandsSSHKeyToSSH(t *testing.T) {
	t.Parallel()

	key := []byte("-----BEGIN OPENSSH PRIVATE KEY-----\nplaceholder\n")

	var (
		keyPath string
		keySeen []byte
		keyMode fs.FileMode
	)

	log := &commandLog{replies: map[string]vexec.Result{
		"git version": {Stdout: []byte("git version 2.39.3 (Apple Git-146)\n")},
	}}
	v := gitVenv(log)

	log.onRun = func(inv *vexec.Invocation) {
		if inv.Args[0] != "clone" {
			return
		}

		for _, kv := range inv.Env {
			if cmd, ok := strings.CutPrefix(kv, getter.EnvGitSSHCommand+"="); ok {
				keyPath = strings.TrimPrefix(cmd, "ssh -i ")
			}
		}

		keySeen, _ = vfs.ReadFile(v.FS, keyPath)

		if info, err := v.FS.Stat(keyPath); err == nil {
			keyMode = info.Mode().Perm()
		}
	}

	src := "git::ssh://git@example.com/org/repo.git?sshkey=" + base64.StdEncoding.EncodeToString(key)

	require.NoError(t, download(t, getter.NewGitGetter(v), src, venvtest.Root("/work/module"), getter.ModeDir))

	assert.Equal(t, "git version", log.argv()[0], "the git version must be checked before the key is used")
	assert.Equal(t, key, keySeen, "ssh must be handed the decoded key")
	assert.Equal(t, fs.FileMode(0o600), keyMode)
	assert.NotContains(t, log.argv()[1], "sshkey", "the key must not reach git's command line")

	_, err := v.FS.Stat(keyPath)
	require.ErrorIs(t, err, fs.ErrNotExist, "the key file must be removed after the download")
}

func TestGitGetterRefusesSSHKeyOnOldGit(t *testing.T) {
	t.Parallel()

	log := &commandLog{replies: map[string]vexec.Result{
		"git version": {Stdout: []byte("git version 2.2.1\n")},
	}}

	src := "git::ssh://git@example.com/org/repo.git?sshkey=" + base64.StdEncoding.EncodeToString([]byte("key"))

	err := download(t, getter.NewGitGetter(gitVenv(log)), src, venvtest.Root("/work/module"), getter.ModeDir)
	require.ErrorIs(t, err, getter.ErrGitTooOldForSSHKey)
	assert.Equal(t, []string{"git version"}, log.argv(), "nothing may be cloned with a git too old for the key")
}

func TestGitGetterRefusesDashRef(t *testing.T) {
	t.Parallel()

	log := &commandLog{}

	err := download(t, getter.NewGitGetter(gitVenv(log)),
		"git::https://example.com/org/repo.git?ref=--upload-pack=touch", venvtest.Root("/work/module"), getter.ModeDir)
	require.ErrorIs(t, err, getter.ErrInvalidGitSource)
	assert.Empty(t, log.argv())
}

func TestGitGetterRefusesPortOutOfRange(t *testing.T) {
	t.Parallel()

	log := &commandLog{}

	err := download(t, getter.NewGitGetter(gitVenv(log)),
		"git::ssh://git@example.com:65536/org/repo.git", venvtest.Root("/work/module"), getter.ModeDir)
	require.ErrorIs(t, err, getter.ErrInvalidGitSource)
	assert.Empty(t, log.argv())
}

func TestGitGetterFailsWhenExecRefuses(t *testing.T) {
	t.Parallel()

	v := venvtest.New().WithHandler(func(context.Context, vexec.Invocation) vexec.Result {
		return vexec.Result{Err: errDenied}
	})

	err := download(t, getter.NewGitGetter(v),
		"git::https://example.com/org/repo.git", venvtest.Root("/work/module"), getter.ModeDir)
	require.ErrorIs(t, err, errDenied)
}

func TestGitGetterErrorKeepsEndOfLongStderr(t *testing.T) {
	t.Parallel()

	const reason = "fatal: repository 'https://example.com/org/repo.git/' not found"

	stderr := strings.Repeat("remote: counting objects\n", 10_000) + reason + "\n"
	log := &commandLog{replies: map[string]vexec.Result{"git clone": {Stderr: []byte(stderr), ExitCode: 128}}}

	err := download(t, getter.NewGitGetter(gitVenv(log)),
		"git::https://example.com/org/repo.git", venvtest.Root("/work/module"), getter.ModeDir)
	require.Error(t, err)

	assert.Contains(t, err.Error(), reason)
	assert.Less(t, len(err.Error()), 5000)
}

func TestGitGetterKeepsCredentialsOutOfErrors(t *testing.T) {
	t.Parallel()

	const token = "s3cr3t-token"

	tests := []struct {
		name   string
		src    string
		stderr string
	}{
		{
			name:   "userinfo",
			src:    "git::https://user:" + token + "@example.com/org/repo.git",
			stderr: "fatal: unable to access 'https://user:" + token + "@example.com/org/repo.git/'\n",
		},
		{
			name:   "query",
			src:    "git::https://example.com/org/repo.git?private_token=" + token,
			stderr: "fatal: unable to access 'https://example.com/org/repo.git?private_token=" + token + "/'\n",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			log := &commandLog{replies: map[string]vexec.Result{
				"git clone": {Stderr: []byte(tc.stderr), ExitCode: 128},
			}}

			err := download(t, getter.NewGitGetter(gitVenv(log)), tc.src, venvtest.Root("/work/module"), getter.ModeDir)
			require.Error(t, err)
			assert.NotContains(t, err.Error(), token)
		})
	}
}

func TestGitGetterGetsOneFile(t *testing.T) {
	t.Parallel()

	log := &commandLog{}
	v := gitVenv(log)

	log.onRun = func(inv *vexec.Invocation) {
		if inv.Args[0] == "clone" {
			checkout := inv.Args[len(inv.Args)-1]
			require.NoError(t, vfs.WriteFile(v.FS, filepath.Join(checkout, "main.tf"), []byte("# module"), 0o644))
		}
	}

	dst := venvtest.Root("/work/main.tf")

	require.NoError(t, download(t, getter.NewGitGetter(v),
		"git::https://example.com/org/repo.git/main.tf?ref=v1", dst, getter.ModeFile))

	got, err := vfs.ReadFile(v.FS, dst)
	require.NoError(t, err)
	assert.Equal(t, "# module", string(got))
	assert.True(t, slices.ContainsFunc(log.argv(), func(line string) bool {
		return strings.HasPrefix(line, "git clone -- https://example.com/org/repo.git ")
	}), "the repository holding the file must be cloned, got %v", log.argv())
}

func TestGitGetterRefusesSymlinkedFile(t *testing.T) {
	t.Parallel()

	if venvtest.New().Platform.GOOS == "windows" {
		t.Skip("creating a symlink on Windows needs a privilege the test runner may not hold")
	}

	log := &commandLog{}
	tmp := t.TempDir()
	v := venvtest.New().WithFS(vfs.NewOSFS()).WithTempDir(func() string { return tmp }).WithHandler(log.handler())

	log.onRun = func(inv *vexec.Invocation) {
		if inv.Args[0] == "clone" {
			checkout := inv.Args[len(inv.Args)-1]
			require.NoError(t, v.FS.MkdirAll(checkout, 0o755))
			require.NoError(t, vfs.Symlink(v.FS, "../../outside", filepath.Join(checkout, "main.tf")))
		}
	}

	err := download(t, getter.NewGitGetter(v),
		"git::https://example.com/org/repo.git/main.tf", filepath.Join(tmp, "out", "main.tf"), getter.ModeFile)
	require.ErrorIs(t, err, getter.ErrSourceFileNotRegular)
}
