package helpers

import (
	"bytes"
	"errors"
	"io/fs"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	cryptossh "golang.org/x/crypto/ssh"
)

const (
	thTHTTPURL = "http://127.0.0.1:1/repo.git"
	thTSSHURL  = "ssh://git@127.0.0.1:2/terragrunt.git"
)

func TestSubstituteTree(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()

	thTWriteFixture(t, filepath.Join(dir, "a", "main.tf"),
		`source = "git::__MIRROR_URL__//x?ref=__MIRROR_SHA__" # __MIRROR_SSH_URL__`)
	thTWriteFixture(t, filepath.Join(dir, "b", "terragrunt.hcl"), `__MIRROR_URL__`)
	thTWriteFixture(t, filepath.Join(dir, "c.tofu"), `__MIRROR_SHA__`)
	thTWriteFixture(t, filepath.Join(dir, "README.md"), `__MIRROR_URL__`)

	// A file without placeholders is left untouched, not rewritten.
	unchanged := filepath.Join(dir, "unchanged.hcl")
	thTWriteFixture(t, unchanged, "inputs = {}\n")

	old := time.Date(2020, time.January, 1, 0, 0, 0, 0, time.UTC)
	require.NoError(t, os.Chtimes(unchanged, old, old))

	// An empty SSH URL leaves its placeholder in place.
	require.NoError(t, substituteTree(dir, thTHTTPURL, "", "abc123"))

	assert.Equal(t, `source = "git::`+thTHTTPURL+`//x?ref=abc123" # __MIRROR_SSH_URL__`,
		thTReadFixture(t, filepath.Join(dir, "a", "main.tf")))
	assert.Equal(t, thTHTTPURL, thTReadFixture(t, filepath.Join(dir, "b", "terragrunt.hcl")))
	assert.Equal(t, "abc123", thTReadFixture(t, filepath.Join(dir, "c.tofu")))
	assert.Equal(t, `__MIRROR_URL__`, thTReadFixture(t, filepath.Join(dir, "README.md")))

	info, err := os.Stat(unchanged)
	require.NoError(t, err)
	assert.True(t, info.ModTime().Equal(old), "unchanged file was rewritten")

	require.ErrorIs(t, substituteTree(filepath.Join(dir, "missing"), thTHTTPURL, "", ""), fs.ErrNotExist)
}

func TestIsFixtureSubstFile(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		ext      string
		expected bool
	}{
		{ext: ".hcl", expected: true},
		{ext: ".tf", expected: true},
		{ext: ".tofu", expected: true},
		{ext: ".json"},
		{ext: ""},
	}

	for _, tc := range testCases {
		t.Run("ext "+tc.ext, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tc.expected, isFixtureSubstFile(tc.ext))
		})
	}
}

func TestWalkFixturesRooted(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	scanDir := filepath.Join(root, "test", "fixtures", "a")

	thTWriteFixture(t, filepath.Join(scanDir, "main.tf"), "__MIRROR_URL__ __MIRROR_SSH_URL__ __MIRROR_SHA__")
	thTWriteFixture(t, filepath.Join(scanDir, "notes.txt"), "__MIRROR_URL__")
	thTWriteFixture(t, filepath.Join(scanDir, ".terraform", "providers.tf"), "")
	thTWriteFixture(t, filepath.Join(scanDir, "nested", ".terragrunt-cache", "cached.tf"), "")
	thTWriteFixture(t, filepath.Join(scanDir, "terraform.tfstate"), "{}")
	thTWriteFixture(t, filepath.Join(scanDir, "terraform.tfstate.backup"), "{}")
	thTWriteFixture(t, filepath.Join(scanDir, "terragrunt-debug.tfvars.json"), "{}")
	thTSymlinkIfPossible(t, "main.tf", filepath.Join(scanDir, "link.tf"))

	got := map[string]string{}

	err := walkFixturesRooted(root, scanDir, thTHTTPURL, thTSSHURL, func(rel string, data []byte) error {
		got[rel] = string(data)

		return nil
	})
	require.NoError(t, err)

	// Paths are relative to root, state, debug, cache and linked files are skipped, and the SHA placeholder stays.
	assert.Equal(t, map[string]string{
		"test/fixtures/a/main.tf":   thTHTTPURL + " " + thTSSHURL + " __MIRROR_SHA__",
		"test/fixtures/a/notes.txt": "__MIRROR_URL__",
	}, got)

	errStop := errors.New("stop")

	err = walkFixturesRooted(root, scanDir, "", "", func(string, []byte) error { return errStop })
	require.ErrorIs(t, err, errStop)

	err = walkFixturesRooted(root, filepath.Join(root, "missing"), "", "", func(string, []byte) error { return nil })
	require.ErrorIs(t, err, fs.ErrNotExist)
}

func TestCommitDirs(t *testing.T) {
	t.Parallel()

	// No directories means no commit, so the server is never touched.
	require.NoError(t, commitDirs(t.Context(), nil, "unused", nil, ""))

	fixturesDir := filepath.Join(t.TempDir(), "test", "fixtures")

	err := commitDirs(t.Context(), nil, fixturesDir, []string{"test/fixtures/missing"}, "")
	require.ErrorIs(t, err, fs.ErrNotExist)
}

func TestMirrorRefsInTree(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()

	thTWriteFixture(t, filepath.Join(dir, "terragrunt.hcl"),
		`source = "git::__MIRROR_URL__//test/fixtures/a?ref=v1"`+"\n"+`source = "../relative"`)
	thTWriteFixture(t, filepath.Join(dir, "unit", "main.tf"), `source = "git::__MIRROR_SSH_URL__//test/fixtures/b/"`)
	thTSymlinkIfPossible(t, "terragrunt.hcl", filepath.Join(dir, "link.hcl"))

	refs, err := mirrorRefsInTree(dir)
	require.NoError(t, err)

	// Relative sources resolve inside the copy, so only mirror references count and the link is read once.
	assert.ElementsMatch(t, []string{"test/fixtures/a", "test/fixtures/b"}, refs)

	_, err = mirrorRefsInTree(filepath.Join(dir, "missing"))
	require.ErrorIs(t, err, fs.ErrNotExist)
}

func TestMirrorRefsInSubtree(t *testing.T) {
	t.Parallel()

	fixturesDir := thTFakeFixtures(t)

	refs, err := mirrorRefsInSubtree(fixturesDir, filepath.Join(fixturesDir, "a"))
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"test/fixtures/b", "test/fixtures/c"}, refs)

	refs, err = mirrorRefsInSubtree(fixturesDir, filepath.Join(fixturesDir, "b"))
	require.NoError(t, err)
	assert.Equal(t, []string{"test/fixtures/c"}, refs)

	_, err = mirrorRefsInSubtree(fixturesDir, filepath.Join(fixturesDir, "missing"))
	require.ErrorIs(t, err, fs.ErrNotExist)
}

func TestMirrorExpandClosure(t *testing.T) {
	t.Parallel()

	fixturesDir := thTFakeFixtures(t)

	// Duplicate seeds are walked once, missing seeds dropped, and nested seeds folded into their parent.
	dirs, err := mirrorExpandClosure(fixturesDir, []string{
		"test/fixtures/a",
		"test/fixtures/a",
		"test/fixtures/a/sub",
		"test/fixtures/missing",
	})
	require.NoError(t, err)
	assert.Equal(t, []string{"test/fixtures/a", "test/fixtures/b", "test/fixtures/c"}, dirs)
}

func TestMirrorRefsInTreeUnreadableFile(t *testing.T) {
	t.Parallel()

	fixturesDir := thTUnreadableFixtures(t)

	_, err := mirrorRefsInTree(filepath.Join(fixturesDir, "u"))
	require.ErrorIs(t, err, fs.ErrPermission)
}

func TestMirrorRefsInSubtreeUnreadableFile(t *testing.T) {
	t.Parallel()

	fixturesDir := thTUnreadableFixtures(t)

	_, err := mirrorRefsInSubtree(fixturesDir, filepath.Join(fixturesDir, "u"))
	require.ErrorIs(t, err, fs.ErrPermission)
}

func TestMirrorExpandClosureUnreadableFile(t *testing.T) {
	t.Parallel()

	fixturesDir := thTUnreadableFixtures(t)

	_, err := mirrorExpandClosure(fixturesDir, []string{"test/fixtures/u"})
	require.ErrorIs(t, err, fs.ErrPermission)
}

func TestWalkFixturesRootedUnreadableFile(t *testing.T) {
	t.Parallel()

	fixturesDir := thTUnreadableFixtures(t)
	root := filepath.Dir(filepath.Dir(fixturesDir))

	err := walkFixturesRooted(root, filepath.Join(fixturesDir, "u"), "", "", func(string, []byte) error { return nil })
	require.ErrorIs(t, err, fs.ErrPermission)
}

func TestSubstituteTreeUnreadableFile(t *testing.T) {
	t.Parallel()

	fixturesDir := thTUnreadableFixtures(t)

	require.ErrorIs(t, substituteTree(filepath.Join(fixturesDir, "u"), thTHTTPURL, "", ""), fs.ErrPermission)
}

func TestRunGit(t *testing.T) {
	t.Parallel()

	thTRequireBinaries(t, "git")

	dir := t.TempDir()

	require.NoError(t, runGit(dir, "init"))
	assert.DirExists(t, filepath.Join(dir, ".git"))

	err := runGit(dir, "thT-not-a-git-command")
	require.Error(t, err)
	require.ErrorContains(t, err, "thT-not-a-git-command")
	require.ErrorContains(t, err, "is not a git command")
}

func TestCopyFixturesToDisk(t *testing.T) {
	t.Parallel()

	fixturesDir, err := locateFixturesDir()
	require.NoError(t, err)

	workDir := t.TempDir()

	require.NoError(t, copyFixturesToDisk(fixturesDir, []string{"test/fixtures/download/hello-world"}, workDir, thTHTTPURL, thTSSHURL))

	contents := thTReadFixture(t, filepath.Join(workDir, "test", "fixtures", "download", "hello-world", "main.tf"))
	assert.Contains(t, contents, "git::"+thTHTTPURL+"//test/fixtures/download/hello-world-no-remote")
	assert.FileExists(t, filepath.Join(workDir, "test", "fixtures", "download", "hello-world", "hello", "main.tf"))

	// A work dir that is a regular file cannot hold the fixture tree.
	notDir := filepath.Join(t.TempDir(), "work")
	thTWriteFixture(t, notDir, "")

	require.Error(t, copyFixturesToDisk(fixturesDir, []string{"test/fixtures/download/hello-world"}, notDir, "", ""))

	err = copyFixturesToDisk(fixturesDir, []string{"test/fixtures/thT-missing"}, t.TempDir(), "", "")
	require.ErrorIs(t, err, fs.ErrNotExist)
}

func TestPopulateBareRepoInitBareFailure(t *testing.T) {
	t.Parallel()

	thTRequireBinaries(t, "git")

	fixturesDir, err := locateFixturesDir()
	require.NoError(t, err)

	// git cannot create a bare repository over an existing regular file.
	bareFile := filepath.Join(t.TempDir(), "bare.git")
	thTWriteFixture(t, bareFile, "")

	err = populateBareRepo(fixturesDir, nil, bareFile, thTHTTPURL, thTSSHURL)
	require.Error(t, err)
	require.ErrorContains(t, err, "init --bare")
}

func TestStartSSHMirrorPopulateFailure(t *testing.T) {
	t.Parallel()

	thTRequireBinaries(t, "ssh", "git-upload-pack", "git")

	fixturesDir, err := locateFixturesDir()
	require.NoError(t, err)

	m, err := startSSHMirror(fixturesDir, []string{"test/fixtures/thT-missing"}, thTHTTPURL)
	require.Error(t, err)
	assert.Nil(t, m)
	require.ErrorContains(t, err, "populate bare repo")
	require.ErrorIs(t, err, fs.ErrNotExist)
}

func TestHandleGitSSHSession(t *testing.T) {
	t.Parallel()

	thTRequireBinaries(t, "ssh", "git-upload-pack", "git")

	fixturesDir, err := locateFixturesDir()
	require.NoError(t, err)

	m, err := startSSHMirror(fixturesDir, nil, thTHTTPURL)
	require.NoError(t, err)

	t.Cleanup(func() {
		_ = m.server.Close()
		_ = m.ln.Close()
		_ = os.RemoveAll(m.bareDir)
	})

	client := thTDialSSH(t, m)

	testCases := []struct {
		name       string
		command    string
		wantStderr string
		removeRepo bool
	}{
		{
			name:       "no command",
			wantStderr: "no command provided",
		},
		{
			name:       "unsupported command",
			command:    "ls -la",
			wantStderr: `unsupported command "ls"`,
		},
		{
			name:       "git command fails",
			command:    "git-upload-pack '/terragrunt.git'",
			wantStderr: "git-upload-pack failed",
			removeRepo: true,
		},
	}

	// The cases share one server and the last one removes its repository, so they run in order.
	for _, tc := range testCases {
		if tc.removeRepo {
			require.NoError(t, os.RemoveAll(m.bareDir))
		}

		stderr, err := thTRunSSH(t, client, tc.command)

		exitErr, ok := errors.AsType[*cryptossh.ExitError](err)
		require.True(t, ok, "%s: expected an exit error, got %v", tc.name, err)
		assert.Equal(t, 1, exitErr.ExitStatus(), tc.name)
		assert.Contains(t, stderr, tc.wantStderr, tc.name)
	}
}

// thTFakeFixtures builds test/fixtures/{a,b,c}: a uses b by path and c by mirror, b uses c by template-url.
func thTFakeFixtures(t *testing.T) string {
	t.Helper()

	fixturesDir := filepath.Join(t.TempDir(), "test", "fixtures")

	thTWriteFixture(t, filepath.Join(fixturesDir, "a", "main.tf"),
		`source = "../b"`+"\n"+`source = "git::__MIRROR_URL__//test/fixtures/c"`)
	thTWriteFixture(t, filepath.Join(fixturesDir, "a", "sub", "variables.tf"), "")
	thTWriteFixture(t, filepath.Join(fixturesDir, "b", "boilerplate.yml"), "dependencies:\n  - template-url: ../c\n")
	thTWriteFixture(t, filepath.Join(fixturesDir, "c", "main.tf"), "")
	thTSymlinkIfPossible(t, "main.tf", filepath.Join(fixturesDir, "a", "link.tf"))

	return fixturesDir
}

// thTUnreadableFixtures builds fixture u with an unreadable file, skipping where permissions do not block reads.
func thTUnreadableFixtures(t *testing.T) string {
	t.Helper()

	if runtime.GOOS == "windows" {
		t.Skip("Windows ignores the read permission bits")
	}

	if os.Geteuid() == 0 {
		t.Skip("root can read any file")
	}

	fixturesDir := filepath.Join(t.TempDir(), "test", "fixtures")
	secret := filepath.Join(fixturesDir, "u", "secret.tf")

	thTWriteFixture(t, secret, "__MIRROR_URL__")
	require.NoError(t, os.Chmod(secret, 0))

	return fixturesDir
}

// thTRequireBinaries skips the test unless every named binary is on PATH.
func thTRequireBinaries(t *testing.T, names ...string) {
	t.Helper()

	for _, name := range names {
		if _, err := exec.LookPath(name); err != nil {
			t.Skipf("%s is not on PATH", name)
		}
	}
}

// thTDialSSH connects to the mirror with the client key it accepts.
func thTDialSSH(t *testing.T, m *sshMirror) *cryptossh.Client {
	t.Helper()

	signer, err := cryptossh.ParsePrivateKey(m.keyPEM)
	require.NoError(t, err)

	addr := m.ln.Addr().String()

	var dialer net.Dialer

	conn, err := dialer.DialContext(t.Context(), "tcp", addr)
	require.NoError(t, err)

	sshConn, chans, reqs, err := cryptossh.NewClientConn(conn, addr, &cryptossh.ClientConfig{
		User:            "git",
		Auth:            []cryptossh.AuthMethod{cryptossh.PublicKeys(signer)},
		HostKeyCallback: cryptossh.InsecureIgnoreHostKey(),
	})
	require.NoError(t, err)

	client := cryptossh.NewClient(sshConn, chans, reqs)

	t.Cleanup(func() { _ = client.Close() })

	return client
}

// thTRunSSH runs command in a new session, or a shell if command is empty, and returns the server's stderr.
func thTRunSSH(t *testing.T, client *cryptossh.Client, command string) (string, error) {
	t.Helper()

	session, err := client.NewSession()
	require.NoError(t, err)

	defer session.Close()

	var stderr bytes.Buffer

	session.Stderr = &stderr

	if command == "" {
		require.NoError(t, session.Shell())

		err = session.Wait()
	} else {
		err = session.Run(command)
	}

	return stderr.String(), err
}

// thTSymlinkIfPossible links link to target, logging instead of failing where symlinks are not allowed.
func thTSymlinkIfPossible(t *testing.T, target, link string) {
	t.Helper()

	if err := os.Symlink(target, link); err != nil {
		t.Logf("symlinks unavailable, skipping the link case: %v", err)
	}
}

// thTWriteFixture writes content to path, creating its parent directories.
func thTWriteFixture(t *testing.T, path, content string) {
	t.Helper()

	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
}

// thTReadFixture returns the contents of path.
func thTReadFixture(t *testing.T, path string) string {
	t.Helper()

	data, err := os.ReadFile(path)
	require.NoError(t, err)

	return string(data)
}
