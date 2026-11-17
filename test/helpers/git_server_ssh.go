package helpers

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	gliderssh "github.com/gliderlabs/ssh"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	cryptossh "golang.org/x/crypto/ssh"

	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
)

const (
	sshKeyFilePerm    fs.FileMode = 0o600
	sshDirPerm        fs.FileMode = 0o755
	sshOutputFilePerm fs.FileMode = 0o644
)

// gitRepoEnvVars are the variables that point git at a repository or alter its object store: the entries git
// itself clears for a local repository, minus the config injection ones.
var gitRepoEnvVars = []string{
	"GIT_ALTERNATE_OBJECT_DIRECTORIES", "GIT_CONFIG", "GIT_DIR", "GIT_WORK_TREE", "GIT_IMPLICIT_WORK_TREE",
	"GIT_GRAFT_FILE", "GIT_INDEX_FILE", "GIT_NO_REPLACE_OBJECTS", "GIT_REPLACE_REF_BASE", "GIT_PREFIX",
	"GIT_SHALLOW_FILE", "GIT_COMMON_DIR", "GIT_OBJECT_DIRECTORY",
}

// sshMirror is a running localhost SSH git server. The server stops and
// its bare repo is removed when the test that started it finishes.
type sshMirror struct {
	// url is the SSH endpoint, of form
	// `ssh://git@127.0.0.1:PORT/terragrunt.git`.
	url string
	// bareDir is the bare repo the server serves.
	bareDir string
	// keyPEM is the OpenSSH-format private key the server accepts.
	keyPEM []byte
}

// startSSHMirror brings up a localhost SSH git server backed by an
// on-disk bare repo populated with the given repo-relative fixture
// directories (the same content the HTTP mirror serves). The server
// execs `git-upload-pack` (or `git-receive-pack`) against the bare repo
// on each session, ignoring the path the client requested because there
// is only one repo to serve. The returned [sshMirror] carries the SSH
// URL (`ssh://git@HOST:PORT/terragrunt.git`) and the OpenSSH-format
// private key bytes that consumer tests materialize on disk via
// [GitServer.RequireSSH]. The server shuts down when t finishes.
//
// Fails t when `ssh` or `git-upload-pack` is not on PATH.
func startSSHMirror(t *testing.T, fixturesDir string, dirs []string, mirrorHTTPURL string) *sshMirror {
	t.Helper()

	for _, bin := range []string{"ssh", "git-upload-pack"} {
		_, err := exec.LookPath(bin)
		require.NoError(t, err, "ssh git server needs %s on PATH", bin)
	}

	clientPriv, clientPub, err := generateSSHKey()
	require.NoError(t, err, "generate client key")

	hostSigner, err := generateHostSigner()
	require.NoError(t, err, "generate host signer")

	bareDir := t.TempDir()

	l := logger.CreateLogger()

	server := &gliderssh.Server{
		Handler: func(s gliderssh.Session) {
			if err := handleGitSSHSession(s, bareDir); err != nil {
				l.Debugf("SSH git session: %v", err)
			}
		},
		PublicKeyHandler: func(_ gliderssh.Context, key gliderssh.PublicKey) bool {
			return gliderssh.KeysEqual(key, clientPub)
		},
	}
	server.AddHostKey(hostSigner)

	// Bind first so the port (and therefore the SSH URL) is known
	// before populating the bare repo. Fixtures that embed
	// __MIRROR_SSH_URL__ need the substitution to happen at bake time.
	var lc net.ListenConfig

	ln, err := lc.Listen(t.Context(), "tcp", "127.0.0.1:0")
	require.NoError(t, err, "listen")

	serveErrCh := make(chan error, 1)

	go func() {
		serveErrCh <- server.Serve(ln)
	}()

	t.Cleanup(func() {
		assert.NoError(t, server.Close(), "close ssh server")

		if err := ln.Close(); !errors.Is(err, net.ErrClosed) {
			assert.NoError(t, err, "close ssh listener")
		}

		if err := <-serveErrCh; !errors.Is(err, gliderssh.ErrServerClosed) && !errors.Is(err, net.ErrClosed) {
			assert.NoError(t, err, "serve ssh")
		}
	})

	sshURL := fmt.Sprintf("ssh://git@%s/terragrunt.git", ln.Addr())

	require.NoError(t, populateBareRepo(fixturesDir, dirs, bareDir, mirrorHTTPURL, sshURL), "populate bare repo")

	return &sshMirror{
		url:     sshURL,
		keyPEM:  clientPriv,
		bareDir: bareDir,
	}
}

// handleGitSSHSession services a single SSH `exec` request by running
// `git-upload-pack` (or `git-receive-pack`) against the bare repo and
// piping stdin/stdout/stderr through the SSH channel. It returns the errors
// from reporting to the client and from ending the session.
func handleGitSSHSession(s gliderssh.Session, bareDir string) error {
	cmd := s.Command()
	if len(cmd) == 0 {
		return failGitSSHSession(s, "no command provided\n")
	}

	switch cmd[0] {
	case "git-upload-pack", "git-receive-pack":
	default:
		return failGitSSHSession(s, fmt.Sprintf("unsupported command %q\n", cmd[0]))
	}

	// Ignore the path the client requested. The server has a single
	// bare repo and tests only need the upload-pack stream to succeed.
	execCmd := exec.CommandContext(s.Context(), cmd[0], bareDir)
	execCmd.Stdin = s
	execCmd.Stdout = s
	execCmd.Stderr = s.Stderr()

	if err := execCmd.Run(); err != nil {
		return failGitSSHSession(s, fmt.Sprintf("%s failed: %v\n", cmd[0], err))
	}

	return s.Exit(0)
}

// failGitSSHSession writes msg to the session's stderr and ends the session
// with exit status 1.
func failGitSSHSession(s gliderssh.Session, msg string) error {
	_, writeErr := io.WriteString(s.Stderr(), msg)

	return errors.Join(writeErr, s.Exit(1))
}

// generateSSHKey returns an ed25519 keypair: the private key encoded
// in OpenSSH format (suitable for writing to a file the `ssh` client
// can read via `ssh -i`), and the matching `crypto/ssh.PublicKey` for
// the server's authorization check.
func generateSSHKey() ([]byte, cryptossh.PublicKey, error) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, nil, err
	}

	pemBlock, err := cryptossh.MarshalPrivateKey(priv, "")
	if err != nil {
		return nil, nil, err
	}

	sshPub, err := cryptossh.NewPublicKey(pub)
	if err != nil {
		return nil, nil, err
	}

	return pem.EncodeToMemory(pemBlock), sshPub, nil
}

// generateHostSigner returns a fresh ed25519 signer for the server's
// host key. The client disables host-key verification, so the host
// signer just needs to exist.
func generateHostSigner() (cryptossh.Signer, error) {
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, err
	}

	return cryptossh.NewSignerFromKey(priv)
}

// populateBareRepo writes the given repo-relative fixture directories
// into a working repo on disk, commits and tags it (using
// [TerragruntMirrorTags] and [TerragruntMirrorBranches]), then runs
// `git push --mirror` into the empty bare repo at bareDir. The work goes
// through the real `git` binary, not go-git, because the SSH server
// execs the real `git-upload-pack`, which only reads on-disk
// repositories.
//
// __MIRROR_URL__ and __MIRROR_SSH_URL__ in `*.hcl`, `*.tf`, and `*.tofu`
// files are substituted at copy time so a clone of one fixture that
// references another stays inside the local mirror.
func populateBareRepo(fixturesDir string, dirs []string, bareDir, httpURL, sshURL string) (retErr error) {
	workDir, err := os.MkdirTemp("", "terragrunt-ssh-work-*")
	if err != nil {
		return fmt.Errorf("mkdir work: %w", err)
	}

	defer func() {
		if err := os.RemoveAll(workDir); err != nil {
			retErr = errors.Join(retErr, fmt.Errorf("remove work repo: %w", err))
		}
	}()

	// Identity and signing are pinned, and the per-user ignore and attributes files, which git reads even
	// with config files suppressed, are pointed at nothing.
	settings := []struct{ key, value string }{
		{key: "user.email", value: "test@test.com"},
		{key: "user.name", value: "Test"},
		{key: "commit.gpgSign", value: "false"},
		{key: "tag.gpgSign", value: "false"},
		{key: "core.excludesFile", value: os.DevNull},
		{key: "core.attributesFile", value: os.DevNull},
	}

	// An empty template keeps the host's template hooks out of the work repo.
	cmds := [][]string{{"init", "-b", "main", "--template="}}
	for _, setting := range settings {
		cmds = append(cmds, []string{"config", setting.key, setting.value})
	}

	for _, args := range cmds {
		if err := runGit(workDir, args...); err != nil {
			return err
		}
	}

	if err := copyFixturesToDisk(fixturesDir, dirs, workDir, httpURL, sshURL); err != nil {
		return fmt.Errorf("copy fixtures: %w", err)
	}

	if err := runGit(workDir, "add", "."); err != nil {
		return err
	}

	// --allow-empty: RequireSSH may build the bare repo before any
	// fixtures are added (the SSH URL must be known so it can be
	// substituted into a fixture rendered afterwards, which then
	// refreshes this repo with content).
	if err := runGit(workDir, "commit", "--allow-empty", "-m", "seed test/fixtures"); err != nil {
		return err
	}

	for _, tag := range TerragruntMirrorTags {
		// Lightweight tags match [git.Server.Tag], keeping both mirrors symmetric.
		if err := runGit(workDir, "tag", tag); err != nil {
			return err
		}
	}

	for _, branch := range TerragruntMirrorBranches {
		if err := runGit(workDir, "branch", branch); err != nil {
			return err
		}
	}

	if err := runGit("", "init", "--bare", "-b", "main", "--template=", bareDir); err != nil {
		return err
	}

	if err := runGit(workDir, "push", "--mirror", bareDir); err != nil {
		return err
	}

	return nil
}

// runGit runs git with the host's global and system config files and system attributes suppressed and any
// inherited repository selection dropped, so the mirror cannot vary with developer settings or with the
// repository a caller runs in. GIT_CONFIG_COUNT and GIT_CONFIG_PARAMETERS are inherited on purpose, so tests
// can inject config into the work repo steps. They never reach the bare repo: git drops both before it starts
// the receive-pack for a local push, so the bare repo's own hooks stay in charge.
func runGit(dir string, args ...string) error {
	cmd := exec.CommandContext(context.Background(), "git", args...)
	if dir != "" {
		cmd.Dir = dir
	}

	cmd.Env = slices.DeleteFunc(os.Environ(), func(kv string) bool {
		name, _, _ := strings.Cut(kv, "=")

		return slices.Contains(gitRepoEnvVars, name)
	})
	cmd.Env = append(cmd.Env, "GIT_CONFIG_GLOBAL="+os.DevNull, "GIT_CONFIG_NOSYSTEM=1", "GIT_ATTR_NOSYSTEM=1")

	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git %v: %w: %s", args, err, out)
	}

	return nil
}

// copyFixturesToDisk writes the given repo-relative fixture directories
// into workDir at their full `test/fixtures/...` path, reusing
// [walkFixturesRooted] so the skip rules and placeholder substitution
// stay in lockstep with the HTTP path.
func copyFixturesToDisk(fixturesDir string, dirs []string, workDir, httpURL, sshURL string) error {
	repoRoot := filepath.Dir(filepath.Dir(fixturesDir))

	for _, dir := range dirs {
		err := walkFixturesRooted(
			repoRoot,
			filepath.Join(repoRoot, dir),
			httpURL,
			sshURL,
			func(rel string, data []byte) error {
				dst := filepath.Join(workDir, filepath.FromSlash(rel))
				if err := os.MkdirAll(filepath.Dir(dst), sshDirPerm); err != nil {
					return err
				}

				return os.WriteFile(dst, data, sshOutputFilePerm)
			},
		)
		if err != nil {
			return err
		}
	}

	return nil
}
