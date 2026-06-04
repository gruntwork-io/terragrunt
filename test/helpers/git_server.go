package helpers

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	gliderssh "github.com/gliderlabs/ssh"
	gogit "github.com/go-git/go-git/v6"
	"github.com/go-git/go-git/v6/plumbing/object"
	"github.com/gruntwork-io/terragrunt/internal/git"
	"github.com/stretchr/testify/require"
)

const (
	// MirrorURLPlaceholder is the token fixtures embed where they would
	// otherwise hold `https://github.com/gruntwork-io/terragrunt.git`.
	// [GitServer.RenderFixture] rewrites it to the server's HTTP URL.
	MirrorURLPlaceholder = "__MIRROR_URL__"

	// MirrorSSHURLPlaceholder is the token fixtures embed where they
	// would otherwise hold an SSH GitHub URL. It is rewritten to the
	// server's SSH URL (`ssh://git@127.0.0.1:PORT/terragrunt.git`).
	MirrorSSHURLPlaceholder = "__MIRROR_SSH_URL__"

	// MirrorSHAPlaceholder is the token fixtures embed where they pin a
	// commit SHA. It is rewritten to the server's HEAD commit hash.
	MirrorSHAPlaceholder = "__MIRROR_SHA__"
)

// TerragruntMirrorTags lists the git tag names integration test
// fixtures pin against. A server creates all of them at HEAD; tests
// don't depend on the historical content of any of these refs, only on
// the ability to clone the named ref.
var TerragruntMirrorTags = []string{
	"v0.53.8",
	"v0.67.4",
	"v0.78.4",
	"v0.79.0",
	"v0.83.2",
	"v0.84.1",
	"v0.85.0",
	"v0.93.2",
	"v0.99.1",
}

// TerragruntMirrorBranches lists non-default branches a server creates
// at HEAD. Tests that exercise URL-parser tolerance for slash-bearing
// ref names (e.g., source-map's slash-in-ref behavior) rely on these.
var TerragruntMirrorBranches = []string{
	"fixture/test-fixtures",
}

// GitServer is a dedicated, per-test git server pair (HTTP, plus
// optional SSH) that serves fixtures from the working tree's
// `test/fixtures/` directory as if they were the upstream
// `gruntwork-io/terragrunt` repository, so tests avoid cloning the real
// repo from GitHub.
//
// Create one with [NewGitServer], which registers shutdown via
// t.Cleanup. Populate it with the fixtures a test clones: most tests
// just call [GitServer.RenderFixture], which serves whatever the
// rendered fixture references; tests that build a source URL by hand
// declare their content with [GitServer.AddFixtures]. Call
// [GitServer.RequireSSH] to add the SSH endpoint.
//
// A GitServer is owned by one test and is not safe to share across
// tests. Because [GitServer.RequireSSH] calls t.Setenv, a test that
// uses SSH must not call t.Parallel.
type GitServer struct {
	// URL is the HTTP endpoint, of form
	// `http://127.0.0.1:PORT/terragrunt.git`.
	URL string
	// SSHURL is the SSH endpoint, set once [GitServer.RequireSSH]
	// succeeds, else empty.
	SSHURL string

	t           *testing.T
	fixturesDir string
	// headSHA is the current HEAD commit, substituted for
	// [MirrorSHAPlaceholder].
	headSHA string

	httpServer  *git.Server
	sshServer   *gliderssh.Server
	sshListener net.Listener
	bareDir     string

	// committed is the set of repo-relative fixture directories already
	// served, so AddFixtures only commits what is missing.
	committed map[string]bool
	// sshKeyPEM is the OpenSSH-format private key the SSH endpoint
	// accepts; RequireSSH writes it under t.TempDir().
	sshKeyPEM []byte

	// mu guards committed and the lazy SSH fields.
	mu sync.Mutex
	// sshLive reports whether RequireSSH has built the SSH endpoint, so
	// later AddFixtures calls also refresh the SSH bare repo.
	sshLive bool
}

// NewGitServer starts an HTTP git server backed by an empty in-memory
// repository and registers its shutdown with t.Cleanup, so the server
// is torn down when the test finishes. The returned server serves no
// fixtures yet; add them with [GitServer.RenderFixture] or
// [GitServer.AddFixtures].
func NewGitServer(t *testing.T) *GitServer {
	t.Helper()

	srv, err := git.NewServer()
	require.NoError(t, err, "new git server")

	base, err := srv.Start(context.Background())
	require.NoError(t, err, "start git server")

	fixturesDir, err := locateFixturesDir()
	require.NoError(t, err, "locate fixtures dir")

	// Append `/terragrunt.git` so the HTTP URL is symmetric with the SSH
	// URL. The underlying [git.Server] uses a single-repo loader that
	// ignores the request path, so any path resolves to the same storer.
	s := &GitServer{
		URL:         base + "/terragrunt.git",
		t:           t,
		fixturesDir: fixturesDir,
		httpServer:  srv,
		committed:   map[string]bool{},
	}

	// Seed an empty initial commit and create every tag and branch at
	// it, so the standard refs always resolve even before (or without)
	// any fixtures. A clone of `?ref=vX` then succeeds and a missing
	// subpath surfaces as "working dir not found" rather than a
	// confusing "ref not found", and source-map tests that redirect to a
	// ref without rendering a fixture still find the ref.
	require.NoError(t, s.initRefs(), "initialize git server refs")

	t.Cleanup(s.shutdown)

	return s
}

// initRefs creates an empty initial commit and points every tag and
// branch at it. It runs before the server is shared, so it needs no
// lock.
func (s *GitServer) initRefs() error {
	w, err := s.httpServer.Repo().Worktree()
	if err != nil {
		return fmt.Errorf("worktree: %w", err)
	}

	sig := &object.Signature{Name: "Test", Email: "test@test.com", When: time.Now()}

	if _, err := w.Commit("seed empty repository", &gogit.CommitOptions{
		Author:            sig,
		Committer:         sig,
		AllowEmptyCommits: true,
	}); err != nil {
		return fmt.Errorf("seed commit: %w", err)
	}

	head, err := s.httpServer.Head()
	if err != nil {
		return fmt.Errorf("resolve head: %w", err)
	}

	s.headSHA = head

	return s.retagLocked()
}

// shutdown releases the server's HTTP and SSH endpoints and removes the
// SSH bare repo. It is best-effort: teardown errors have nowhere
// meaningful to go.
func (s *GitServer) shutdown() {
	if s.httpServer != nil {
		_ = s.httpServer.Close()
	}

	if s.sshServer != nil {
		_ = s.sshServer.Close()
	}

	if s.sshListener != nil {
		_ = s.sshListener.Close()
	}

	if s.bareDir != "" {
		_ = os.RemoveAll(s.bareDir)
	}
}

// SourceURL returns a go-getter source string of the form
// `git::<server-url>//<subpath>?ref=<ref>`. Pass an empty ref to omit
// the query. SourceURL only builds the string; call [GitServer.AddFixtures]
// for subpath so the server actually serves it.
func (s *GitServer) SourceURL(subpath, ref string) string {
	src := "git::" + s.URL + "//" + strings.TrimPrefix(subpath, "/")
	if ref != "" {
		src += "?ref=" + ref
	}

	return src
}

// RenderFixture copies the fixture at fixturePath into a fresh temp dir,
// makes the server serve every fixture the copy references via a
// `__MIRROR_URL__//` / `__MIRROR_SSH_URL__//` source (and their
// transitive references), and rewrites the copy's mirror placeholders to
// point at this server. It returns the copy's root directory.
//
// If the fixture uses [MirrorSSHURLPlaceholder], call
// [GitServer.RequireSSH] first so SSHURL is known when the copy is
// rewritten.
func (s *GitServer) RenderFixture(fixturePath string, includeInCopy ...string) string {
	s.t.Helper()

	dir := CopyEnvironment(s.t, fixturePath, includeInCopy...)

	refs, err := mirrorRefsInTree(dir)
	require.NoError(s.t, err, "scan %s for git server references", fixturePath)

	s.AddFixtures(refs...)

	require.NoError(s.t, substituteTree(dir, s.URL, s.SSHURL, s.headSHA), "render placeholders in %s", dir)

	return dir
}

// AddFixtures makes the server serve the given repo-relative fixture
// subpaths (e.g. "test/fixtures/scaffold/scaffold-module") and
// everything they reference, transitively. Call it for fixtures a test
// clones through a hand-built source URL; [GitServer.RenderFixture] adds
// the fixtures it renders automatically. It is idempotent and may be
// called before or after [GitServer.RequireSSH].
func (s *GitServer) AddFixtures(subpaths ...string) {
	s.t.Helper()

	require.NoError(s.t, s.add(subpaths), "add fixtures %v", subpaths)
}

// add commits any of subpaths' reference closure the server does not yet
// serve, as a new commit on top of HEAD, then re-points the tags and
// branches at the new HEAD (so a `scaffold` source resolved to the
// latest tag finds the content) and refreshes the SSH bare repo when SSH
// is live.
func (s *GitServer) add(subpaths []string) error {
	cleaned := make([]string, 0, len(subpaths))
	for _, sp := range subpaths {
		cleaned = append(cleaned, strings.Trim(filepath.ToSlash(sp), "/"))
	}

	dirs, err := mirrorExpandClosure(s.fixturesDir, cleaned)
	if err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	var added []string

	for _, dir := range dirs {
		if !s.committed[dir] {
			added = append(added, dir)
		}
	}

	if len(added) == 0 {
		return nil
	}

	if err := commitDirs(s.httpServer, s.fixturesDir, added, s.URL); err != nil {
		return err
	}

	for _, dir := range added {
		s.committed[dir] = true
	}

	head, err := s.httpServer.Head()
	if err != nil {
		return fmt.Errorf("resolve head: %w", err)
	}

	s.headSHA = head

	if err := s.retagLocked(); err != nil {
		return err
	}

	if s.sshLive {
		if err := populateBareRepo(s.fixturesDir, s.committedDirsLocked(), s.bareDir, s.URL, s.SSHURL); err != nil {
			return fmt.Errorf("refresh ssh bare repo: %w", err)
		}
	}

	return nil
}

// RequireSSH adds an SSH endpoint over the content committed so far and
// points the calling test at it. It skips the test when the SSH endpoint
// can't be built (typically because `ssh` or `git-upload-pack` is not on
// PATH). On success it writes the server's private key under t.TempDir()
// and points `GIT_SSH_COMMAND` at it via t.Setenv. Because t.Setenv
// panics in parallel tests, a test that calls RequireSSH must not call
// t.Parallel.
//
// RequireSSH may be called before the fixtures are added; AddFixtures and
// RenderFixture refresh the SSH bare repo afterwards. This ordering is
// what lets a fixture that embeds [MirrorSSHURLPlaceholder] be rendered
// after the SSH URL is known.
func (s *GitServer) RequireSSH() {
	s.t.Helper()

	if IsWindows() {
		s.t.Skip("the in-process git-over-SSH server relies on Unix-only setup (GIT_SSH_COMMAND, /dev/null) unavailable on the Windows runner")
	}

	s.mu.Lock()

	if s.sshServer == nil {
		ssh, err := startSSHMirror(s.fixturesDir, s.committedDirsLocked(), s.URL)
		if err != nil {
			s.mu.Unlock()
			s.t.Skipf("ssh git server unavailable: %v", err)

			return
		}

		s.SSHURL = ssh.url
		s.sshKeyPEM = ssh.keyPEM
		s.sshServer = ssh.server
		s.sshListener = ssh.ln
		s.bareDir = ssh.bareDir
		s.sshLive = true
	}

	keyPEM := s.sshKeyPEM
	s.mu.Unlock()

	keyPath := filepath.Join(s.t.TempDir(), "id_ed25519")
	require.NoError(s.t, os.WriteFile(keyPath, keyPEM, sshKeyFilePerm), "write ssh key")

	s.t.Setenv("GIT_SSH_COMMAND", fmt.Sprintf(
		"ssh -i %s -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null -o IdentitiesOnly=yes",
		keyPath,
	))
}

// retagLocked moves every tag and branch to the current HEAD. The caller
// must hold s.mu.
func (s *GitServer) retagLocked() error {
	repo := s.httpServer.Repo()

	for _, tag := range TerragruntMirrorTags {
		if err := repo.DeleteTag(tag); err != nil && !errors.Is(err, gogit.ErrTagNotFound) {
			return fmt.Errorf("delete tag %s: %w", tag, err)
		}

		if err := s.httpServer.Tag(tag); err != nil {
			return fmt.Errorf("tag %s: %w", tag, err)
		}
	}

	for _, branch := range TerragruntMirrorBranches {
		if err := s.httpServer.Branch(branch); err != nil {
			return fmt.Errorf("branch %s: %w", branch, err)
		}
	}

	return nil
}

// committedDirsLocked returns the served fixture directories in sorted
// order. The caller must hold s.mu.
func (s *GitServer) committedDirsLocked() []string {
	dirs := make([]string, 0, len(s.committed))
	for dir := range s.committed {
		dirs = append(dirs, dir)
	}

	sort.Strings(dirs)

	return dirs
}

// substituteMirrorPlaceholders rewrites [MirrorURLPlaceholder],
// [MirrorSSHURLPlaceholder], and [MirrorSHAPlaceholder] in data. Empty
// replacement values are skipped so the literal placeholder remains:
// replacing `__MIRROR_SSH_URL__` with `""` before RequireSSH would
// produce a malformed `git::` URL, whereas leaving the placeholder
// yields a clear error if the fixture is actually used.
func substituteMirrorPlaceholders(data []byte, httpURL, sshURL, headSHA string) []byte {
	if httpURL != "" {
		data = bytes.ReplaceAll(data, []byte(MirrorURLPlaceholder), []byte(httpURL))
	}

	if sshURL != "" {
		data = bytes.ReplaceAll(data, []byte(MirrorSSHURLPlaceholder), []byte(sshURL))
	}

	if headSHA != "" {
		data = bytes.ReplaceAll(data, []byte(MirrorSHAPlaceholder), []byte(headSHA))
	}

	return data
}

// substituteTree rewrites the mirror placeholders in every `*.hcl`,
// `*.tf`, and `*.tofu` file under dir, writing back only files that
// actually changed.
func substituteTree(dir, httpURL, sshURL, headSHA string) error {
	return filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if d.IsDir() || !isFixtureSubstFile(filepath.Ext(d.Name())) {
			return nil
		}

		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}

		out := substituteMirrorPlaceholders(data, httpURL, sshURL, headSHA)
		if bytes.Equal(out, data) {
			return nil
		}

		return os.WriteFile(path, out, readWritePermissions)
	})
}

// isFixtureSubstFile reports whether ext denotes a file whose contents
// should be searched for mirror placeholders.
func isFixtureSubstFile(ext string) bool {
	switch ext {
	case ".hcl", ".tf", ".tofu":
		return true
	}

	return false
}

// walkFixturesRooted walks scanDir applying the standard skip rules
// (`.terraform`, `.terragrunt-cache`, symlinks, `terraform.tfstate*`,
// `terragrunt-debug.tfvars.json`) and, for each remaining file, calls fn
// with the path relative to root (slash-separated) and the file's bytes
// after [substituteMirrorPlaceholders] has been applied using httpURL and
// sshURL. scanDir must be root or a directory beneath it; reporting paths
// relative to root lets a scoped walk of one fixture subtree still
// produce its full `test/fixtures/...` repo path.
func walkFixturesRooted(root, scanDir, httpURL, sshURL string, fn func(rel string, data []byte) error) error {
	return filepath.WalkDir(scanDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if d.IsDir() {
			if d.Name() == ".terraform" || d.Name() == ".terragrunt-cache" {
				return filepath.SkipDir
			}

			return nil
		}

		// Skip symlinks (e.g., regression benchmark fixtures point a
		// dependency-group dir at a template dir). They have no bearing
		// on the git-server tests and following them would require
		// duplicating the target into the server.
		if d.Type()&fs.ModeSymlink != 0 {
			return nil
		}

		name := d.Name()
		if name == "terragrunt-debug.tfvars.json" || strings.HasPrefix(name, "terraform.tfstate") {
			return nil
		}

		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}

		data, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("read %s: %w", path, err)
		}

		if isFixtureSubstFile(filepath.Ext(name)) {
			data = substituteMirrorPlaceholders(data, httpURL, sshURL, "")
		}

		return fn(filepath.ToSlash(rel), data)
	})
}

func locateFixturesDir() (string, error) {
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		return "", errors.New("locate helper file via runtime.Caller")
	}

	return filepath.Join(filepath.Dir(thisFile), "..", "fixtures"), nil
}

// commitDirs writes the files of each repo-relative fixture directory in
// dirs into srv's worktree at their full `test/fixtures/<rel>` path,
// substituting [MirrorURLPlaceholder] with mirrorURL, then bulk-stages
// and creates a single commit on top of the current HEAD. Passing an
// empty dirs is a no-op.
//
// Bulk-staging via [AddOptions.All] is required because per-file `Add()`
// calls `Status()` on the entire worktree each time, which degrades to
// O(n²); serving only the fixtures a test needs keeps the count small.
func commitDirs(srv *git.Server, fixturesDir string, dirs []string, mirrorURL string) error {
	if len(dirs) == 0 {
		return nil
	}

	repoRoot := filepath.Dir(filepath.Dir(fixturesDir))

	repo := srv.Repo()

	w, err := repo.Worktree()
	if err != nil {
		return fmt.Errorf("worktree: %w", err)
	}

	for _, dir := range dirs {
		walkErr := walkFixturesRooted(repoRoot, filepath.Join(repoRoot, dir), mirrorURL, "", func(rel string, data []byte) error {
			f, err := w.Filesystem.Create(rel)
			if err != nil {
				return fmt.Errorf("create %s: %w", rel, err)
			}

			if _, err := f.Write(data); err != nil {
				return fmt.Errorf("write %s: %w", rel, err)
			}

			if err := f.Close(); err != nil {
				return fmt.Errorf("close %s: %w", rel, err)
			}

			return nil
		})
		if walkErr != nil {
			return walkErr
		}
	}

	if err := w.AddWithOptions(&gogit.AddOptions{All: true}); err != nil {
		return fmt.Errorf("add all: %w", err)
	}

	sig := &object.Signature{Name: "Test", Email: "test@test.com", When: time.Now()}

	if _, err := w.Commit("seed test/fixtures from working tree", &gogit.CommitOptions{
		Author:    sig,
		Committer: sig,
	}); err != nil {
		return fmt.Errorf("commit: %w", err)
	}

	return nil
}
