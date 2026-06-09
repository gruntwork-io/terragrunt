package helpers

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/pem"
	"fmt"
	"io/fs"
	"net"
	"os"
	"os/exec"
	"path/filepath"

	gliderssh "github.com/gliderlabs/ssh"
	cryptossh "golang.org/x/crypto/ssh"
)

const (
	sshKeyFilePerm    fs.FileMode = 0o600
	sshDirPerm        fs.FileMode = 0o755
	sshOutputFilePerm fs.FileMode = 0o644
)

// sshMirror is a running localhost SSH git server together with the
// resources that must be released when the owning test finishes: the
// [gliderssh.Server], its listener, and the on-disk bare repo.
// [GitServer.shutdown] releases them.
type sshMirror struct {
	server *gliderssh.Server
	ln     net.Listener
	// url is the SSH endpoint, of form
	// `ssh://git@127.0.0.1:PORT/terragrunt.git`.
	url string
	// bareDir is the temporary bare repo the server serves; removed
	// on shutdown.
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
// URL (`ssh://git@HOST:PORT/terragrunt.git`), the OpenSSH-format private
// key bytes that consumer tests materialize on disk via
// [GitServer.RequireSSH], and the server/listener/bare-repo
// handles its owner releases via [GitServer.shutdown].
//
// Returns an error if `ssh` or `git-upload-pack` is not on PATH;
// callers should treat that as "skip SSH tests" rather than fatal.
func startSSHMirror(fixturesDir string, dirs []string, mirrorHTTPURL string) (*sshMirror, error) {
	if _, err := exec.LookPath("ssh"); err != nil {
		return nil, fmt.Errorf("ssh client not available: %w", err)
	}

	if _, err := exec.LookPath("git-upload-pack"); err != nil {
		return nil, fmt.Errorf("git-upload-pack not available: %w", err)
	}

	// Bind first so the port (and therefore the SSH URL) is known
	// before populating the bare repo. Fixtures that embed
	// __MIRROR_SSH_URL__ need the substitution to happen at bake time.
	var lc net.ListenConfig

	ln, err := lc.Listen(context.Background(), "tcp", "127.0.0.1:0")
	if err != nil {
		return nil, fmt.Errorf("listen: %w", err)
	}

	tcpAddr, ok := ln.Addr().(*net.TCPAddr)
	if !ok {
		_ = ln.Close()

		return nil, fmt.Errorf("listener returned non-TCP addr %T", ln.Addr())
	}

	sshURL := fmt.Sprintf("ssh://git@127.0.0.1:%d/terragrunt.git", tcpAddr.Port)

	clientPriv, clientPub, err := generateSSHKey()
	if err != nil {
		_ = ln.Close()

		return nil, fmt.Errorf("generate client key: %w", err)
	}

	hostSigner, err := generateHostSigner()
	if err != nil {
		_ = ln.Close()

		return nil, fmt.Errorf("host signer: %w", err)
	}

	bareDir, err := os.MkdirTemp("", "terragrunt-ssh-bare-*.git")
	if err != nil {
		_ = ln.Close()

		return nil, fmt.Errorf("mkdir bare: %w", err)
	}

	if err := populateBareRepo(fixturesDir, dirs, bareDir, mirrorHTTPURL, sshURL); err != nil {
		_ = ln.Close()
		_ = os.RemoveAll(bareDir)

		return nil, fmt.Errorf("populate bare repo: %w", err)
	}

	server := &gliderssh.Server{
		Handler: func(s gliderssh.Session) {
			handleGitSSHSession(s, bareDir)
		},
		PublicKeyHandler: func(_ gliderssh.Context, key gliderssh.PublicKey) bool {
			return gliderssh.KeysEqual(key, clientPub)
		},
	}
	server.AddHostKey(hostSigner)

	go func() { _ = server.Serve(ln) }()

	return &sshMirror{
		url:     sshURL,
		keyPEM:  clientPriv,
		server:  server,
		ln:      ln,
		bareDir: bareDir,
	}, nil
}

// handleGitSSHSession services a single SSH `exec` request by running
// `git-upload-pack` (or `git-receive-pack`) against the bare repo and
// piping stdin/stdout/stderr through the SSH channel.
func handleGitSSHSession(s gliderssh.Session, bareDir string) {
	cmd := s.Command()
	if len(cmd) == 0 {
		_, _ = fmt.Fprintln(s.Stderr(), "no command provided")
		_ = s.Exit(1)

		return
	}

	switch cmd[0] {
	case "git-upload-pack", "git-receive-pack":
	default:
		_, _ = fmt.Fprintf(s.Stderr(), "unsupported command %q\n", cmd[0])
		_ = s.Exit(1)

		return
	}

	// Ignore the path the client requested. The server has a single
	// bare repo and tests only need the upload-pack stream to succeed.
	execCmd := exec.CommandContext(s.Context(), cmd[0], bareDir)
	execCmd.Stdin = s
	execCmd.Stdout = s
	execCmd.Stderr = s.Stderr()

	if err := execCmd.Run(); err != nil {
		_, _ = fmt.Fprintf(s.Stderr(), "%s failed: %v\n", cmd[0], err)
		_ = s.Exit(1)

		return
	}

	_ = s.Exit(0)
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
func populateBareRepo(fixturesDir string, dirs []string, bareDir, httpURL, sshURL string) error {
	workDir, err := os.MkdirTemp("", "terragrunt-ssh-work-*")
	if err != nil {
		return fmt.Errorf("mkdir work: %w", err)
	}

	defer func() { _ = os.RemoveAll(workDir) }()

	initCmds := [][]string{
		{"init", "-b", "main"},
		{"config", "user.email", "test@test.com"},
		{"config", "user.name", "Test"},
		{"config", "commit.gpgSign", "false"},
		{"config", "tag.gpgSign", "false"},
	}
	for _, args := range initCmds {
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
		// `-a -m` matches the annotated tags created on the HTTP
		// mirror via [git.Server.Tag], so both mirrors expose the
		// same tag object type.
		if err := runGit(workDir, "tag", "-a", "-m", tag, tag); err != nil {
			return err
		}
	}

	for _, branch := range TerragruntMirrorBranches {
		if err := runGit(workDir, "branch", branch); err != nil {
			return err
		}
	}

	if err := runGit("", "init", "--bare", "-b", "main", bareDir); err != nil {
		return err
	}

	if err := runGit(workDir, "push", "--mirror", bareDir); err != nil {
		return err
	}

	return nil
}

func runGit(dir string, args ...string) error {
	cmd := exec.CommandContext(context.Background(), "git", args...)
	if dir != "" {
		cmd.Dir = dir
	}

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
		err := walkFixturesRooted(repoRoot, filepath.Join(repoRoot, dir), httpURL, sshURL, func(rel string, data []byte) error {
			dst := filepath.Join(workDir, filepath.FromSlash(rel))
			if err := os.MkdirAll(filepath.Dir(dst), sshDirPerm); err != nil {
				return err
			}

			return os.WriteFile(dst, data, sshOutputFilePerm)
		})
		if err != nil {
			return err
		}
	}

	return nil
}
